// Package lifecycle starts, supervises, and stops the packaged desktop process topology.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
	"github.com/d31ma/sluice/internal/transport/ipc"
)

const (
	defaultStartupTimeout = 15 * time.Second
	cleanupTimeout        = 8 * time.Second
	initialRestartBackoff = 100 * time.Millisecond
	maximumRestartBackoff = 5 * time.Second
)

// Config is the immutable packaged-runtime configuration.
type Config struct {
	Bundle         runtimeexec.MacOSBundle
	StateRoot      string
	RuntimeVersion string
	StartupTimeout time.Duration
}

// Coordinator owns startup until it returns one live Session.
type Coordinator struct {
	deps   dependencies
	config Config
}

// Session represents a supervised authenticated local API. The Session stays
// valid while failed process generations are replaced behind it.
type Session struct {
	coordinator *Coordinator
	current     *generation
	paths       privatePaths
	cancel      context.CancelFunc
	done        chan struct{}
	closeErr    error
	shutdownErr error
	lastErr     error
	endpoint    string
	closeOnce   sync.Once
	mu          sync.RWMutex
}

type generation struct {
	tachyon              managedProcess
	agent                managedProcess
	descriptorCapability string
	endpoint             string
	projectRoot          string
	resources            generationResources
}

type managedProcess interface {
	Done() <-chan struct{}
	Err() error
	Close(ctx context.Context) error
}

type dependencies struct {
	startAgent     func(context.Context, agentRequest) (managedProcess, error)
	waitDescriptor func(context.Context, descriptorWaitRequest) (ipc.Descriptor, error)
	startTachyon   func(context.Context, tachyonRequest) (managedProcess, error)
	probeHealth    func(context.Context, string, string, managedProcess) error
	choosePort     func() (int, error)
	restartDelay   func(int) time.Duration
	waitRestart    func(context.Context, time.Duration) error
}

type agentRequest struct {
	AgentBinary    string
	FyloBinary     string
	FyloRoot       string
	RuntimeVersion string
	TokenFile      string
	SessionFile    string
}

type descriptorWaitRequest struct {
	Process            managedProcess
	Path               string
	PreviousCapability string
}

type tachyonRequest struct {
	Bundle         runtimeexec.MacOSBundle
	ProjectRoot    string
	SessionFile    string
	RuntimeVersion string
	Port           int
}

type cleanupResult struct {
	tachyon error
	agent   error
	project error
	residue error
}

// New validates a production lifecycle configuration.
func New(config Config) (*Coordinator, error) {
	return newCoordinator(config, productionDependencies())
}

func newCoordinator(config Config, deps dependencies) (*Coordinator, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	if err := validateDependencies(deps); err != nil {
		return nil, err
	}
	return &Coordinator{config: normalized, deps: deps}, nil
}

func normalizeConfig(config Config) (Config, error) {
	if config.StateRoot == "" || !filepath.IsAbs(config.StateRoot) {
		return Config{}, fmt.Errorf("Sluice state root must be absolute") //nolint:staticcheck // Product name begins the operator-facing diagnostic.
	}
	if strings.TrimSpace(config.RuntimeVersion) == "" {
		return Config{}, fmt.Errorf("Sluice runtime version is empty") //nolint:staticcheck // Product name begins the operator-facing diagnostic.
	}
	if config.StartupTimeout == 0 {
		config.StartupTimeout = defaultStartupTimeout
	}
	if config.StartupTimeout < time.Second || config.StartupTimeout > time.Minute {
		return Config{}, fmt.Errorf("Sluice startup timeout must be within 1s..1m") //nolint:staticcheck // Product name begins the operator-facing diagnostic.
	}
	config.StateRoot = filepath.Clean(config.StateRoot)
	return config, nil
}

func validateDependencies(deps dependencies) error {
	if deps.startAgent == nil || deps.waitDescriptor == nil || deps.startTachyon == nil ||
		deps.probeHealth == nil || deps.choosePort == nil || deps.restartDelay == nil ||
		deps.waitRestart == nil {
		return fmt.Errorf("Sluice lifecycle dependencies are incomplete") //nolint:staticcheck // Product name begins the operator-facing diagnostic.
	}
	return nil
}

func productionDependencies() dependencies {
	return dependencies{
		startAgent:     startAgentProcess,
		waitDescriptor: waitForDescriptor,
		startTachyon:   startTachyonProcess,
		probeHealth:    awaitAuthenticatedHealth,
		choosePort:     chooseLoopbackPort,
		restartDelay:   boundedRestartDelay,
		waitRestart:    waitForRestart,
	}
}

// Start launches agent/Fylo, waits for the new private descriptor, launches
// Tachyon/Yon, and returns only after authenticated health succeeds.
func (coordinator *Coordinator) Start(ctx context.Context) (*Session, error) {
	startup, cancel := context.WithTimeout(ctx, coordinator.config.StartupTimeout)
	defer cancel()
	paths, token, err := preparePrivateState(coordinator.config.StateRoot)
	if err != nil {
		return nil, err
	}
	current, err := coordinator.startGeneration(startup, paths, token, descriptorCapability(paths.sessionFile))
	if err != nil {
		return nil, err
	}
	supervisorContext, stopSupervisor := context.WithCancel(context.Background())
	session := &Session{
		coordinator: coordinator,
		current:     current,
		paths:       paths,
		cancel:      stopSupervisor,
		done:        make(chan struct{}),
		endpoint:    current.endpoint,
	}
	go session.supervise(supervisorContext, token)
	return session, nil
}

func (coordinator *Coordinator) startGeneration(
	ctx context.Context,
	paths privatePaths,
	token string,
	previousCapability string,
) (*generation, error) {
	current := &generation{}
	agent, err := coordinator.deps.startAgent(ctx, coordinator.agentRequest(paths))
	if err != nil {
		return nil, fmt.Errorf("start Sluice agent: %w", err)
	}
	current.agent = agent
	descriptor, err := coordinator.deps.waitDescriptor(ctx, descriptorWaitRequest{
		Path:               paths.sessionFile,
		PreviousCapability: previousCapability,
		Process:            agent,
	})
	if err != nil {
		return nil, coordinator.failGeneration(current, fmt.Errorf("wait for Sluice agent: %w", err))
	}
	current.descriptorCapability = descriptor.Capability
	current.resources = captureGenerationResources(paths.sessionFile, descriptor)
	project, err := materializeTachyonProject(paths.runRoot, coordinator.config.Bundle, paths.sessionFile)
	if err != nil {
		return nil, coordinator.failGeneration(current, err)
	}
	current.projectRoot = project
	port, err := coordinator.deps.choosePort()
	if err != nil {
		return nil, coordinator.failGeneration(current, fmt.Errorf("choose local API port: %w", err))
	}
	tachyon, err := coordinator.deps.startTachyon(ctx, tachyonRequest{
		Bundle:         coordinator.config.Bundle,
		ProjectRoot:    project,
		SessionFile:    paths.sessionFile,
		RuntimeVersion: coordinator.config.RuntimeVersion,
		Port:           port,
	})
	if err != nil {
		return nil, coordinator.failGeneration(current, fmt.Errorf("start Tachyon local API: %w", err))
	}
	current.tachyon = tachyon
	current.endpoint = fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := coordinator.deps.probeHealth(ctx, current.endpoint, token, tachyon); err != nil {
		return nil, coordinator.failGeneration(
			current,
			fmt.Errorf("prove authenticated local API health: %w", err),
		)
	}
	return current, nil
}

func (coordinator *Coordinator) failGeneration(current *generation, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	return errors.Join(cause, current.cleanup(ctx).err())
}

func (coordinator *Coordinator) agentRequest(paths privatePaths) agentRequest {
	return agentRequest{
		AgentBinary:    coordinator.config.Bundle.AgentBinary,
		FyloBinary:     coordinator.config.Bundle.FyloBinary,
		FyloRoot:       filepath.Join(coordinator.config.StateRoot, "fylo"),
		RuntimeVersion: coordinator.config.RuntimeVersion,
		TokenFile:      paths.tokenFile,
		SessionFile:    paths.sessionFile,
	}
}

func (session *Session) supervise(ctx context.Context, token string) {
	defer close(session.done)
	for {
		current := session.generation()
		if current == nil {
			session.recordError(fmt.Errorf("packaged runtime supervisor lost its active generation"))
			return
		}
		select {
		case <-ctx.Done():
			session.shutdownCurrent()
			return
		case <-current.tachyon.Done():
			session.recover(ctx, token, "Tachyon", current.tachyon)
		case <-current.agent.Done():
			session.recover(ctx, token, "Sluice agent", current.agent)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (session *Session) recover(
	ctx context.Context,
	token string,
	component string,
	failed managedProcess,
) {
	current := session.takeGeneration()
	session.recordError(unexpectedExit(component, failed.Err()))
	cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), cleanupTimeout)
	cleanup := current.cleanup(cleanupContext)
	cancelCleanup()
	if err := cleanup.err(); err != nil {
		session.recordError(fmt.Errorf("clean failed process generation: %w", err))
	}
	if ctx.Err() != nil {
		return
	}
	session.restart(ctx, token, current.descriptorCapability)
}

func (session *Session) restart(ctx context.Context, token, previousCapability string) {
	for attempt := 0; ; attempt++ {
		delay := session.coordinator.deps.restartDelay(attempt)
		if err := session.coordinator.deps.waitRestart(ctx, delay); err != nil {
			return
		}
		startup, cancel := context.WithTimeout(ctx, session.coordinator.config.StartupTimeout)
		current, err := session.coordinator.startGeneration(startup, session.paths, token, previousCapability)
		cancel()
		if err == nil {
			if ctx.Err() != nil {
				cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), cleanupTimeout)
				_ = current.cleanup(cleanupContext)
				cancelCleanup()
				return
			}
			session.installGeneration(current)
			return
		}
		if ctx.Err() != nil {
			return
		}
		session.recordError(fmt.Errorf("restart packaged runtime after %s: %w", delay, err))
	}
}

func (session *Session) shutdownCurrent() {
	current := session.takeGeneration()
	if current == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	result := current.cleanup(ctx)
	session.mu.Lock()
	session.shutdownErr = result.err()
	session.mu.Unlock()
}

func (session *Session) generation() *generation {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.current
}

func (session *Session) takeGeneration() *generation {
	session.mu.Lock()
	defer session.mu.Unlock()
	current := session.current
	session.current = nil
	session.endpoint = ""
	return current
}

func (session *Session) installGeneration(current *generation) {
	session.mu.Lock()
	session.current = current
	session.endpoint = current.endpoint
	session.mu.Unlock()
}

func (session *Session) recordError(err error) {
	if err == nil {
		return
	}
	session.mu.Lock()
	session.lastErr = err
	session.mu.Unlock()
}

func unexpectedExit(component string, cause error) error {
	if cause == nil {
		return fmt.Errorf("%s exited unexpectedly without an exit error", component)
	}
	return fmt.Errorf("%s exited unexpectedly: %w", component, cause)
}

func boundedRestartDelay(attempt int) time.Duration {
	delay := initialRestartBackoff
	for range attempt {
		if delay >= maximumRestartBackoff/2 {
			return maximumRestartBackoff
		}
		delay *= 2
	}
	return delay
}

func waitForRestart(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Endpoint returns the current healthy authenticated loopback API origin. It
// is empty while the supervisor is replacing a failed generation. Callers
// must still present the private bearer token through a higher-level client.
func (session *Session) Endpoint() string {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.endpoint
}

// Err returns the latest unexpected-exit or recovery diagnostic. A non-nil
// value does not mean recovery failed permanently; Endpoint reports whether a
// healthy generation is currently installed.
func (session *Session) Err() error {
	session.mu.RLock()
	defer session.mu.RUnlock()
	return session.lastErr
}

// Close stops retry timers, then shuts Tachyon down before the agent so the
// agent can drain and close its warm Fylo child. It is idempotent.
func (session *Session) Close(ctx context.Context) error {
	session.closeOnce.Do(func() {
		session.cancel()
		select {
		case <-session.done:
			session.mu.RLock()
			session.closeErr = session.shutdownErr
			session.mu.RUnlock()
		case <-ctx.Done():
			session.closeErr = fmt.Errorf("wait for packaged runtime shutdown: %w", ctx.Err())
		}
	})
	return session.closeErr
}

func (current *generation) cleanup(ctx context.Context) cleanupResult {
	if current == nil {
		return cleanupResult{}
	}
	tachyonErr := closeProcess(ctx, current.tachyon)
	agentErr := closeProcess(ctx, current.agent)
	residueErr := current.resources.cleanup()
	projectErr := removeProject(current.projectRoot)
	return cleanupResult{
		tachyon: tachyonErr,
		agent:   agentErr,
		residue: residueErr,
		project: projectErr,
	}
}

func (result cleanupResult) err() error {
	return errors.Join(result.tachyon, result.agent, result.residue, result.project)
}

func closeProcess(ctx context.Context, process managedProcess) error {
	if process == nil {
		return nil
	}
	return process.Close(ctx)
}
