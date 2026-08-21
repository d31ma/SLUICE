// Package lifecycle starts and stops the packaged desktop process topology.
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

// Session represents a healthy authenticated local API and all processes
// behind it.
type Session struct {
	tachyon     managedProcess
	agent       managedProcess
	closeErr    error
	endpoint    string
	projectRoot string
	closeOnce   sync.Once
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
		deps.probeHealth == nil || deps.choosePort == nil {
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
	previous := descriptorCapability(paths.sessionFile)
	agent, err := coordinator.deps.startAgent(startup, coordinator.agentRequest(paths))
	if err != nil {
		return nil, fmt.Errorf("start Sluice agent: %w", err)
	}
	session := &Session{agent: agent}
	if err := coordinator.startAPI(startup, session, paths, token, previous); err != nil {
		return nil, errors.Join(err, session.cleanupAfterFailure())
	}
	return session, nil
}

func (coordinator *Coordinator) startAPI(
	ctx context.Context,
	session *Session,
	paths privatePaths,
	token string,
	previousCapability string,
) error {
	_, err := coordinator.deps.waitDescriptor(ctx, descriptorWaitRequest{
		Path:               paths.sessionFile,
		PreviousCapability: previousCapability,
		Process:            session.agent,
	})
	if err != nil {
		return fmt.Errorf("wait for Sluice agent: %w", err)
	}
	project, err := materializeTachyonProject(paths.runRoot, coordinator.config.Bundle, paths.sessionFile)
	if err != nil {
		return err
	}
	session.projectRoot = project
	port, err := coordinator.deps.choosePort()
	if err != nil {
		return fmt.Errorf("choose local API port: %w", err)
	}
	tachyon, err := coordinator.deps.startTachyon(ctx, tachyonRequest{
		Bundle:         coordinator.config.Bundle,
		ProjectRoot:    project,
		SessionFile:    paths.sessionFile,
		RuntimeVersion: coordinator.config.RuntimeVersion,
		Port:           port,
	})
	if err != nil {
		return fmt.Errorf("start Tachyon local API: %w", err)
	}
	session.tachyon = tachyon
	session.endpoint = fmt.Sprintf("http://127.0.0.1:%d", port)
	if err := coordinator.deps.probeHealth(ctx, session.endpoint, token, tachyon); err != nil {
		return fmt.Errorf("prove authenticated local API health: %w", err)
	}
	return nil
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

// Endpoint returns the authenticated loopback API origin. Callers must still
// present the private bearer token through a higher-level client.
func (session *Session) Endpoint() string {
	return session.endpoint
}

// Close stops Tachyon before the agent, allowing the agent to drain and close
// its warm Fylo child. It is idempotent.
func (session *Session) Close(ctx context.Context) error {
	session.closeOnce.Do(func() {
		tachyonErr := closeProcess(ctx, session.tachyon)
		agentErr := closeProcess(ctx, session.agent)
		projectErr := removeProject(session.projectRoot)
		session.closeErr = errors.Join(tachyonErr, agentErr, projectErr)
	})
	return session.closeErr
}

func (session *Session) cleanupAfterFailure() error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	return session.Close(ctx)
}

func closeProcess(ctx context.Context, process managedProcess) error {
	if process == nil {
		return nil
	}
	return process.Close(ctx)
}
