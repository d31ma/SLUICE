package fylo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
)

const diagnosticLimit = 64 * 1024

// Config identifies a pinned Fylo artifact and its private state root.
type Config struct { //nolint:govet // Public configuration groups identity before policy.
	Binary       string
	Root         string
	Requirements Requirements
	Environment  runtimeexec.Environment
}

// Supervisor owns one warm, exclusive Fylo machine loop.
type Supervisor struct {
	process      *machineProcess
	executable   runtimeexec.Executable
	root         string
	environment  runtimeexec.Environment
	requirements Requirements

	stateMu   sync.Mutex
	requestMu sync.Mutex
}

type machineProcess struct {
	command     *exec.Cmd
	stdin       io.WriteCloser
	writer      *bufio.Writer
	reader      *bufio.Reader
	diagnostics *boundedDiagnostics
}

// NewSupervisor validates configuration without touching the state root.
func NewSupervisor(config Config) (*Supervisor, error) {
	executable, err := runtimeexec.OpenExecutable(config.Binary)
	if err != nil {
		return nil, err
	}
	if config.Root == "" || !filepath.IsAbs(config.Root) {
		return nil, fmt.Errorf("fylo root must be an absolute path")
	}
	if config.Requirements.RuntimeVersion == "" || config.Requirements.ProtocolVersion < 1 {
		return nil, fmt.Errorf("fylo runtime and protocol requirements are required")
	}
	environment := config.Environment
	if environment == nil {
		environment = defaultEnvironment()
	}
	return &Supervisor{
		executable:   executable,
		root:         filepath.Clean(config.Root),
		requirements: config.Requirements,
		environment:  environment,
	}, nil
}

// Probe reads the installed binary's side-effect-free version report and
// rejects an unknown runtime before the state root is created.
func (supervisor *Supervisor) Probe(ctx context.Context) (Handshake, error) {
	output, err := supervisor.executable.Output(
		ctx,
		supervisor.environment,
		defaultMaxResponseBytes,
		"version", "--output", "json",
	)
	if err != nil {
		return Handshake{}, fmt.Errorf("probe Fylo runtime: %w", err)
	}
	var handshake Handshake
	decoder := json.NewDecoder(bytes.NewReader(output))
	if err := decoder.Decode(&handshake); err != nil {
		return Handshake{}, fmt.Errorf("%w: invalid version report: %w", ErrCompatibility, err)
	}
	if err := supervisor.requirements.validate(handshake); err != nil {
		return Handshake{}, err
	}
	return handshake, nil
}

// Start probes compatibility, starts an exclusive machine loop, and completes
// a live handshake before returning.
func (supervisor *Supervisor) Start(ctx context.Context) (Handshake, error) {
	supervisor.stateMu.Lock()
	defer supervisor.stateMu.Unlock()
	if supervisor.process != nil {
		return Handshake{}, fmt.Errorf("fylo supervisor is already running")
	}
	preflight, err := supervisor.Probe(ctx)
	if err != nil {
		return Handshake{}, err
	}
	if mkdirErr := os.MkdirAll(supervisor.root, 0o700); mkdirErr != nil {
		return Handshake{}, fmt.Errorf("create Fylo root: %w", mkdirErr)
	}
	canonicalRoot, err := filepath.EvalSymlinks(supervisor.root)
	if err != nil {
		return Handshake{}, fmt.Errorf("canonicalize Fylo root: %w", err)
	}
	supervisor.root = canonicalRoot
	process, err := supervisor.startProcess()
	if err != nil {
		return Handshake{}, err
	}
	supervisor.process = process
	handshake, err := supervisor.handshakeLocked(ctx)
	if err != nil {
		_ = closeProcess(ctx, process)
		supervisor.process = nil
		return Handshake{}, err
	}
	if handshake.RuntimeVersion != preflight.RuntimeVersion || handshake.Commit != preflight.Commit {
		_ = closeProcess(ctx, process)
		supervisor.process = nil
		return Handshake{}, fmt.Errorf("%w: preflight and live process identity differ", ErrCompatibility)
	}
	return handshake, nil
}

// Handshake revalidates the running process without mutating state.
func (supervisor *Supervisor) Handshake(ctx context.Context) (Handshake, error) {
	supervisor.stateMu.Lock()
	defer supervisor.stateMu.Unlock()
	if supervisor.process == nil {
		return Handshake{}, ErrNotRunning
	}
	return supervisor.handshakeLocked(ctx)
}

func (supervisor *Supervisor) handshakeLocked(ctx context.Context) (Handshake, error) {
	request := map[string]any{"op": "handshake", "requestId": "sluice-runtime-handshake"}
	reply, err := supervisor.request(ctx, request)
	if err != nil {
		return Handshake{}, err
	}
	if !reply.OK {
		if reply.Error != nil && reply.Error.Code == "EROOTLOCKED" {
			return Handshake{}, ErrRootOwned
		}
		return Handshake{}, fmt.Errorf("fylo handshake failed with a stable machine error")
	}
	if reply.Op != "handshake" || reply.ProtocolVersion != supervisor.requirements.ProtocolVersion {
		return Handshake{}, fmt.Errorf("%w: invalid handshake envelope", ErrCompatibility)
	}
	var handshake Handshake
	if err := json.Unmarshal(reply.Result, &handshake); err != nil {
		return Handshake{}, fmt.Errorf("%w: invalid handshake result", ErrCompatibility)
	}
	if err := supervisor.requirements.validate(handshake); err != nil {
		return Handshake{}, err
	}
	return handshake, nil
}

func (supervisor *Supervisor) startProcess() (*machineProcess, error) {
	arguments := []string{
		"exec", "--loop", "--root", supervisor.root, "--exclusive-root",
		"--max-request-bytes", strconv.Itoa(supervisor.requirements.MinimumMaxRequestBytes),
		"--max-response-bytes", strconv.Itoa(supervisor.requirements.MinimumMaxResponseBytes),
	}
	command := supervisor.executable.Command(context.Background(), supervisor.environment, arguments...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Fylo stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open Fylo stdout: %w", err)
	}
	diagnostics := &boundedDiagnostics{limit: diagnosticLimit}
	command.Stderr = diagnostics
	runtimeexec.ConfigureInterruptibleProcess(command)
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start Fylo runtime: %w", err)
	}
	return &machineProcess{
		command:     command,
		stdin:       stdin,
		writer:      bufio.NewWriter(stdin),
		reader:      bufio.NewReaderSize(stdout, supervisor.requirements.MinimumMaxResponseBytes+2),
		diagnostics: diagnostics,
	}, nil
}

func (supervisor *Supervisor) request(ctx context.Context, payload map[string]any) (response, error) {
	supervisor.requestMu.Lock()
	defer supervisor.requestMu.Unlock()
	process := supervisor.process
	if process == nil {
		return response{}, ErrNotRunning
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return response{}, fmt.Errorf("encode Fylo request: %w", err)
	}
	if len(encoded) > supervisor.requirements.MinimumMaxRequestBytes {
		return response{}, fmt.Errorf("fylo request exceeds negotiated frame limit")
	}
	if _, err := process.writer.Write(append(encoded, '\n')); err != nil {
		return response{}, fmt.Errorf("write Fylo request: %w", err)
	}
	if err := process.writer.Flush(); err != nil {
		return response{}, fmt.Errorf("flush Fylo request: %w", err)
	}
	type readResult struct {
		err   error
		reply response
	}
	completed := make(chan readResult, 1)
	go func() {
		reply, err := readResponse(process.reader, supervisor.requirements.MinimumMaxResponseBytes)
		completed <- readResult{reply: reply, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = process.command.Process.Kill()
		return response{}, fmt.Errorf("fylo request cancelled: %w", ctx.Err())
	case result := <-completed:
		return result.reply, result.err
	}
}

func readResponse(reader *bufio.Reader, limit int) (response, error) {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return response{}, fmt.Errorf("read Fylo response: %w", err)
	}
	if len(line)-1 > limit {
		return response{}, fmt.Errorf("fylo response exceeds negotiated frame limit")
	}
	var reply response
	if err := json.Unmarshal(line[:len(line)-1], &reply); err != nil {
		return response{}, fmt.Errorf("decode Fylo response: %w", err)
	}
	return reply, nil
}

// Close ends the machine loop by closing stdin and waits for lease release.
func (supervisor *Supervisor) Close(ctx context.Context) error {
	supervisor.stateMu.Lock()
	defer supervisor.stateMu.Unlock()
	if supervisor.process == nil {
		return nil
	}
	process := supervisor.process
	supervisor.process = nil
	return closeProcess(ctx, process)
}

func closeProcess(ctx context.Context, process *machineProcess) error {
	closeErr := process.stdin.Close()
	completed := make(chan error, 1)
	go func() { completed <- process.command.Wait() }()
	select {
	case <-ctx.Done():
		_ = process.command.Process.Kill()
		<-completed
		return fmt.Errorf("wait for Fylo shutdown: %w", ctx.Err())
	case err := <-completed:
		if err != nil {
			return fmt.Errorf("fylo exited unsuccessfully")
		}
		if closeErr != nil {
			return fmt.Errorf("close Fylo stdin: %w", closeErr)
		}
		return nil
	}
}

func defaultEnvironment() runtimeexec.Environment {
	return runtimeexec.AllowEnvironment(
		"PATH", "HOME", "TMPDIR", "TMP", "TEMP", "SystemRoot", "WINDIR",
		"USERPROFILE", "LOCALAPPDATA", "XDG_STATE_HOME", "LANG", "LC_ALL",
		"HOSTNAME", "COMPUTERNAME",
	)
}

type boundedDiagnostics struct {
	data  []byte
	mu    sync.Mutex
	limit int
}

func (diagnostics *boundedDiagnostics) Write(data []byte) (int, error) {
	diagnostics.mu.Lock()
	defer diagnostics.mu.Unlock()
	remaining := diagnostics.limit - len(diagnostics.data)
	if remaining > 0 {
		retained := min(remaining, len(data))
		diagnostics.data = append(diagnostics.data, data[:retained]...)
	}
	return len(data), nil
}
