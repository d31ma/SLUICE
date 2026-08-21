package yon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
)

const shutdownGrace = 5 * time.Second

// ErrCompatibility means the installed Tachyon release is not the pinned
// Sluice runtime.
var ErrCompatibility = errors.New("tachyon runtime is incompatible")

// RuntimeConfig pins the Tachyon binary accepted by Sluice.
type RuntimeConfig struct {
	Binary           string
	RequiredVersion  string
	AgentSessionFile string
	Environment      runtimeexec.Environment
}

// ServeConfig identifies one loopback Tachyon project instance.
type ServeConfig struct {
	ProjectRoot string
	Host        string
	Port        int
}

// Runtime validates and starts one exact Tachyon artifact.
type Runtime struct {
	executable       runtimeexec.Executable
	requiredVersion  string
	agentSessionFile string
	environment      runtimeexec.Environment
}

// Version is the installed Tachyon product version.
type Version struct {
	Product string
}

// Server is one supervised Tachyon serve process.
type Server struct {
	process *os.Process
	done    chan error
	err     error
	close   sync.Once
}

// NewRuntime validates configuration without starting Tachyon.
func NewRuntime(config RuntimeConfig) (*Runtime, error) {
	executable, err := runtimeexec.OpenExecutable(config.Binary)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(config.RequiredVersion) == "" {
		return nil, fmt.Errorf("required Tachyon version is empty")
	}
	if config.AgentSessionFile == "" || !filepath.IsAbs(config.AgentSessionFile) {
		return nil, fmt.Errorf("agent session file must be absolute")
	}
	environment := config.Environment
	if environment == nil {
		environment = tachyonEnvironment()
	}
	return &Runtime{
		executable:       executable,
		requiredVersion:  config.RequiredVersion,
		agentSessionFile: filepath.Clean(config.AgentSessionFile),
		environment:      environment,
	}, nil
}

// Probe checks the installed release before any project process starts.
func (runtime *Runtime) Probe(ctx context.Context) (Version, error) {
	output, err := runtime.executable.Output(ctx, runtime.environment, 4*1024, "--version")
	if err != nil {
		return Version{}, fmt.Errorf("probe Tachyon runtime: %w", err)
	}
	product := strings.TrimSpace(string(output))
	if product != runtime.requiredVersion {
		return Version{}, fmt.Errorf(
			"%w: runtime version %q is not the required %q",
			ErrCompatibility,
			product,
			runtime.requiredVersion,
		)
	}
	return Version{Product: product}, nil
}

// Start validates the release and launches a loopback-only Tachyon server.
func (runtime *Runtime) Start(ctx context.Context, config ServeConfig) (*Server, error) {
	if _, err := runtime.Probe(ctx); err != nil {
		return nil, err
	}
	project, err := validateServeConfig(config)
	if err != nil {
		return nil, err
	}
	arguments := []string{
		"serve", "--host", config.Host, "--port", strconv.Itoa(config.Port),
		"--no-watch", project,
	}
	environment := append(runtimeexec.Environment(nil), runtime.environment...)
	environment = append(environment, agentSessionFileEnv+"="+runtime.agentSessionFile)
	command := runtime.executable.Command(context.Background(), environment, arguments...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Tachyon runtime: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	return &Server{process: command.Process, done: done}, nil
}

func validateServeConfig(config ServeConfig) (string, error) {
	project, err := validateProjectRoot(config.ProjectRoot)
	if err != nil {
		return "", err
	}
	if err := validateLoopbackHost(config.Host); err != nil {
		return "", err
	}
	return project, validatePort(config.Port)
}

func validateProjectRoot(root string) (string, error) {
	project, err := canonicalProjectRoot(root)
	if err != nil {
		return "", err
	}
	return project, validateProjectDirectory(project)
}

func validateProjectDirectory(project string) error {
	metadata, err := os.Stat(project)
	if err != nil || !metadata.IsDir() {
		return fmt.Errorf("tachyon project root must be a directory")
	}
	return nil
}

func canonicalProjectRoot(root string) (string, error) {
	if err := validateAbsoluteProjectRoot(root); err != nil {
		return "", err
	}
	project, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("open Tachyon project root: %w", err)
	}
	return project, nil
}

func validateAbsoluteProjectRoot(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("tachyon project root must be absolute")
	}
	return nil
}

func validateLoopbackHost(host string) error {
	address := net.ParseIP(host)
	if address == nil || !address.IsLoopback() {
		return fmt.Errorf("tachyon host must be an explicit loopback address")
	}
	return nil
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("tachyon port is outside 1..65535")
	}
	return nil
}

// Close requests a clean interrupt, then force-reaps the process after the
// bounded shutdown grace period.
func (server *Server) Close(ctx context.Context) error {
	server.close.Do(func() {
		server.err = server.stop(ctx)
	})
	return server.err
}

func (server *Server) stop(ctx context.Context) error {
	if err := server.process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = server.process.Kill()
	}
	timer := time.NewTimer(shutdownGrace)
	defer timer.Stop()
	select {
	case err := <-server.done:
		if err != nil {
			return fmt.Errorf("tachyon exited unsuccessfully")
		}
		return nil
	case <-ctx.Done():
		_ = server.process.Kill()
		<-server.done
		return fmt.Errorf("wait for Tachyon shutdown: %w", ctx.Err())
	case <-timer.C:
		_ = server.process.Kill()
		<-server.done
		return fmt.Errorf("tachyon did not stop within %s", shutdownGrace)
	}
}

func tachyonEnvironment() runtimeexec.Environment {
	return runtimeexec.AllowEnvironment(
		"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "SystemRoot", "WINDIR",
		"USERPROFILE", "LOCALAPPDATA", "LANG", "LC_ALL",
	)
}
