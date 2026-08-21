package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
	"github.com/d31ma/sluice/internal/transport/yon"
)

type childProcess struct {
	process   *os.Process
	done      chan struct{}
	waitErr   error
	closeErr  error
	waitMu    sync.Mutex
	closeOnce sync.Once
}

type tachyonProcess struct {
	server *yon.Server
	done   chan struct{}
}

func startAgentProcess(_ context.Context, request agentRequest) (managedProcess, error) {
	executable, err := runtimeexec.OpenExecutable(request.AgentBinary)
	if err != nil {
		return nil, err
	}
	arguments := []string{
		"--fylo-binary", request.FyloBinary,
		"--fylo-root", request.FyloRoot,
		"--fylo-version", request.RuntimeVersion,
		"--api-token-file", request.TokenFile,
		"--session-file", request.SessionFile,
	}
	command := executable.Command(context.Background(), packagedEnvironment(), arguments...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, err
	}
	process := &childProcess{process: command.Process, done: make(chan struct{})}
	go process.wait(command.Wait)
	return process, nil
}

func (process *childProcess) wait(wait func() error) {
	err := wait()
	process.waitMu.Lock()
	process.waitErr = err
	process.waitMu.Unlock()
	close(process.done)
}

func (process *childProcess) Done() <-chan struct{} {
	return process.done
}

func (process *childProcess) Err() error {
	process.waitMu.Lock()
	defer process.waitMu.Unlock()
	return process.waitErr
}

func (process *childProcess) Close(ctx context.Context) error {
	process.closeOnce.Do(func() { process.closeErr = process.stop(ctx) })
	return process.closeErr
}

func (process *childProcess) stop(ctx context.Context) error {
	if exited, err := process.exited(); exited {
		return err
	}
	process.interrupt()
	return process.awaitExit(ctx)
}

func (process *childProcess) exited() (bool, error) {
	select {
	case <-process.done:
		return true, childExitError(process.Err())
	default:
		return false, nil
	}
}

func (process *childProcess) interrupt() {
	if err := process.process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = process.process.Kill()
	}
}

func (process *childProcess) awaitExit(ctx context.Context) error {
	select {
	case <-process.done:
		return childExitError(process.Err())
	case <-ctx.Done():
		_ = process.process.Kill()
		<-process.done
		return fmt.Errorf("wait for Sluice agent shutdown: %w", ctx.Err())
	}
}

func childExitError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("Sluice agent exited unsuccessfully") //nolint:staticcheck // Product name begins the operator-facing diagnostic.
}

func startTachyonProcess(ctx context.Context, request tachyonRequest) (managedProcess, error) {
	runtime, err := yon.NewRuntime(yon.RuntimeConfig{
		Binary:           request.Bundle.TachyonBinary,
		RequiredVersion:  request.RuntimeVersion,
		AgentSessionFile: request.SessionFile,
		Environment:      packagedEnvironment(),
	})
	if err != nil {
		return nil, err
	}
	server, err := runtime.Start(ctx, yon.ServeConfig{
		ProjectRoot: request.ProjectRoot,
		Host:        "127.0.0.1",
		Port:        request.Port,
	})
	if err != nil {
		return nil, err
	}
	return &tachyonProcess{server: server, done: make(chan struct{})}, nil
}

func (process *tachyonProcess) Done() <-chan struct{} {
	return process.done
}

func (*tachyonProcess) Err() error { return nil }

func (process *tachyonProcess) Close(ctx context.Context) error {
	return process.server.Close(ctx)
}

func packagedEnvironment() runtimeexec.Environment {
	return runtimeexec.AllowEnvironment(
		"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "SystemRoot", "WINDIR",
		"USERPROFILE", "LOCALAPPDATA", "LANG", "LC_ALL",
	)
}
