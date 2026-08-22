package yon_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
	"github.com/d31ma/sluice/internal/transport/ipc"
	"github.com/d31ma/sluice/internal/transport/yon"
)

const qualifiedTachyonVersion = "26.33.02"

const qualifiedFyloVersion = "26.33.02"

const qualificationAPIToken = "0123456789abcdef0123456789abcdef"

func TestInstalledTachyonRunsRegisteredGoHandlerAndRestarts(t *testing.T) {
	binary := installedRuntime(t, "ty")
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	agentServer, sessionFile := startAgent(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"status":"ok"}`)
	}))
	defer closeAgent(t, agentServer)

	project := scaffoldTachyonProject(t, ctx, binary)
	handler := buildYonHandler(t, ctx)
	registerGoHandler(t, project, handler, sessionFile)
	port := availablePort(t)

	tachyon, err := yon.NewRuntime(yon.RuntimeConfig{
		Binary:           binary,
		RequiredVersion:  qualifiedTachyonVersion,
		AgentSessionFile: sessionFile,
	})
	if err != nil {
		t.Fatalf("create Tachyon runtime: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		server, err := tachyon.Start(ctx, yon.ServeConfig{
			ProjectRoot: project,
			Host:        "127.0.0.1",
			Port:        port,
		})
		if err != nil {
			t.Fatalf("start Tachyon attempt %d: %v", attempt, err)
		}
		response := awaitHealth(t, ctx, port)
		if response != `{"status":"ok"}` {
			t.Fatalf("health body attempt %d = %q", attempt, response)
		}
		if err := server.Close(ctx); err != nil {
			t.Fatalf("close Tachyon attempt %d: %v", attempt, err)
		}
	}
}

func TestInstalledTachyonVersionMismatchFailsBeforeStarting(t *testing.T) {
	binary := installedRuntime(t, "ty")
	tachyon, err := yon.NewRuntime(yon.RuntimeConfig{
		Binary:           binary,
		RequiredVersion:  "0.0.0-not-qualified",
		AgentSessionFile: filepath.Join(t.TempDir(), "agent-session.json"),
	})
	if err != nil {
		t.Fatalf("create Tachyon runtime: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	project := filepath.Join(t.TempDir(), "must-not-open")
	if _, err := tachyon.Start(ctx, yon.ServeConfig{
		ProjectRoot: project,
		Host:        "127.0.0.1",
		Port:        12345,
	}); !errors.Is(err, yon.ErrCompatibility) {
		t.Fatalf("start error = %v, want ErrCompatibility", err)
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Fatalf("mismatch touched project root; stat error: %v", err)
	}
}

func TestInstalledFourProcessChainPersistsMutationReplayAcrossAgentRestart(t *testing.T) {
	tachyonBinary := installedRuntime(t, "ty")
	fyloBinary := installedRuntime(t, "fylo")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	workspace := t.TempDir()
	sessionFile := filepath.Join(workspace, "agent-session.json")
	tokenFile := filepath.Join(workspace, "api-token")
	if err := os.WriteFile(tokenFile, []byte(qualificationAPIToken), 0o600); err != nil {
		t.Fatalf("write API token: %v", err)
	}
	agentBinary := buildGoProgram(t, ctx, "sluice-agent", "./cmd/sluice-agent")
	handlerBinary := buildYonHandler(t, ctx)
	fyloRoot := filepath.Join(workspace, "fylo")

	agentProcess := startQualifiedAgent(t, ctx, agentBinary, fyloBinary, fyloRoot, tokenFile, sessionFile)
	firstDescriptor := awaitAgentDescriptor(t, ctx, sessionFile, "")
	project := scaffoldTachyonProject(t, ctx, tachyonBinary)
	registerGoHandlerRoute(t, project, handlerBinary, sessionFile, filepath.Join("v1", "roots"))
	port := availablePort(t)
	tachyon, err := yon.NewRuntime(yon.RuntimeConfig{
		Binary:           tachyonBinary,
		RequiredVersion:  qualifiedTachyonVersion,
		AgentSessionFile: sessionFile,
	})
	if err != nil {
		t.Fatalf("create Tachyon runtime: %v", err)
	}
	server, err := tachyon.Start(ctx, yon.ServeConfig{ProjectRoot: project, Host: "127.0.0.1", Port: port})
	if err != nil {
		t.Fatalf("start Tachyon: %v", err)
	}
	defer closeQualificationServer(t, server)

	first := awaitMutation(t, ctx, port, `{"path":"/sync"}`)
	if first.status != http.StatusNotFound {
		t.Fatalf("first mutation = %d %q, want persisted application 404", first.status, first.body)
	}
	if err := agentProcess.Close(ctx); err != nil {
		t.Fatalf("stop first agent: %v", err)
	}
	agentProcess = startQualifiedAgent(t, ctx, agentBinary, fyloBinary, fyloRoot, tokenFile, sessionFile)
	defer closeQualificationAgent(t, agentProcess)
	_ = awaitAgentDescriptor(t, ctx, sessionFile, firstDescriptor.Capability)

	replayed := awaitMutation(t, ctx, port, `{"path":"/sync"}`)
	if replayed.status != first.status || replayed.body != first.body {
		t.Fatalf("replay = (%d, %q), want (%d, %q)", replayed.status, replayed.body, first.status, first.body)
	}
	conflict := awaitMutation(t, ctx, port, `{"path":"/other"}`)
	if conflict.status != http.StatusConflict || !strings.Contains(conflict.body, `"code":"idempotency_key_reused"`) {
		t.Fatalf("conflict = (%d, %q)", conflict.status, conflict.body)
	}
}

func closeQualificationAgent(t *testing.T, process *qualificationProcess) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Close(ctx); err != nil {
		t.Errorf("stop qualification agent: %v", err)
	}
}

func closeQualificationServer(t *testing.T, server *yon.Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Errorf("stop qualification Tachyon: %v", err)
	}
}

func scaffoldTachyonProject(t *testing.T, ctx context.Context, binary string) string {
	t.Helper()
	project := filepath.Join(t.TempDir(), "tachyon-project")
	executable, err := runtimeexec.OpenExecutable(binary)
	if err != nil {
		t.Fatalf("open Tachyon executable: %v", err)
	}
	command := executable.Command(
		ctx,
		runtimeexec.AllowEnvironment("PATH", "HOME", "TMPDIR", "TMP", "TEMP", "SystemRoot"),
		"init", project, "--name", "Sluice runtime qualification",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("scaffold Tachyon project: %v\n%s", err, output)
	}
	return project
}

func buildYonHandler(t *testing.T, ctx context.Context) string {
	t.Helper()
	return buildGoProgram(t, ctx, "sluice-yon", "./internal/transport/yon/cmd/sluice-yon")
}

func buildGoProgram(t *testing.T, ctx context.Context, name, packagePath string) string {
	t.Helper()
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("find Go: %v", err)
	}
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	output := filepath.Join(t.TempDir(), name)
	absolute, err := filepath.Abs(goBinary)
	if err != nil {
		t.Fatalf("resolve Go: %v", err)
	}
	executable, err := runtimeexec.OpenExecutable(absolute)
	if err != nil {
		t.Fatalf("open Go executable: %v", err)
	}
	environment := runtimeexec.AllowEnvironment(
		"PATH",
		"HOME",
		"USERPROFILE",
		"HOMEDRIVE",
		"HOMEPATH",
		"LOCALAPPDATA",
		"APPDATA",
		"TMPDIR",
		"TMP",
		"TEMP",
		"SystemRoot",
		"GOPATH",
		"GOMODCACHE",
		"GOCACHE",
		"GOROOT",
		"GOTOOLCHAIN",
	)
	environment = append(
		environment,
		"CGO_ENABLED=0",
		"GOARCH="+runtime.GOARCH,
		"GOOS="+runtime.GOOS,
	)
	command := executable.Command(
		ctx,
		environment,
		"build", "-o", output, packagePath,
	)
	command.Dir = repositoryRoot(t)
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Go Yon handler: %v\n%s", err, result)
	}
	return output
}

func registerGoHandler(t *testing.T, project, handler, sessionFile string) {
	t.Helper()
	registerGoHandlerRoute(t, project, handler, sessionFile, filepath.Join("v1", "health"))
}

func registerGoHandlerRoute(t *testing.T, project, handler, sessionFile, routePath string) {
	t.Helper()
	configuration, err := json.Marshal(map[string]any{
		"interpreters": map[string][]string{"sluice": {handler, "--session-file", sessionFile}},
	})
	if err != nil {
		t.Fatalf("encode .tachyonrc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, ".tachyonrc"), configuration, 0o600); err != nil {
		t.Fatalf("write .tachyonrc: %v", err)
	}
	route := filepath.Join(project, "server", "routes", routePath)
	if err := os.MkdirAll(route, 0o700); err != nil {
		t.Fatalf("create health route: %v", err)
	}
	if err := os.WriteFile(filepath.Join(route, "yon.sluice"), []byte("registered Go executable\n"), 0o600); err != nil {
		t.Fatalf("write route marker: %v", err)
	}
}

type qualificationProcess struct {
	command *exec.Cmd
	done    chan error
	err     error
	once    sync.Once
}

func startQualifiedAgent(
	t *testing.T,
	ctx context.Context,
	agentBinary string,
	fyloBinary string,
	fyloRoot string,
	tokenFile string,
	sessionFile string,
) *qualificationProcess {
	t.Helper()
	executable, err := runtimeexec.OpenExecutable(agentBinary)
	if err != nil {
		t.Fatalf("open agent executable: %v", err)
	}
	command := executable.Command(
		ctx,
		runtimeexec.AllowEnvironment("PATH", "HOME", "TMPDIR", "TMP", "TEMP", "SystemRoot"),
		"--fylo-binary", fyloBinary,
		"--fylo-root", fyloRoot,
		"--fylo-version", qualifiedFyloVersion,
		"--api-token-file", tokenFile,
		"--session-file", sessionFile,
	)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	runtimeexec.ConfigureInterruptibleProcess(command)
	if err := command.Start(); err != nil {
		t.Fatalf("start qualified agent: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	return &qualificationProcess{command: command, done: done}
}

func (process *qualificationProcess) Close(ctx context.Context) error {
	process.once.Do(func() {
		if err := runtimeexec.InterruptProcess(process.command.Process); err != nil && !errors.Is(err, os.ErrProcessDone) {
			_ = process.command.Process.Kill()
		}
		select {
		case process.err = <-process.done:
		case <-ctx.Done():
			_ = process.command.Process.Kill()
			<-process.done
			process.err = ctx.Err()
		}
	})
	return process.err
}

func awaitAgentDescriptor(
	t *testing.T,
	ctx context.Context,
	path string,
	previousCapability string,
) ipc.Descriptor {
	t.Helper()
	for {
		descriptor, err := ipc.ReadDescriptor(path)
		if err == nil && descriptor.Capability != previousCapability {
			return descriptor
		}
		select {
		case <-ctx.Done():
			t.Fatalf("agent descriptor did not become ready: %v", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

type mutationResult struct {
	body   string
	status int
}

func awaitMutation(t *testing.T, ctx context.Context, port int, body string) mutationResult {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/roots", port)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
		if err != nil {
			t.Fatalf("create mutation request: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+qualificationAPIToken)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "chain-request-0001")
		response, err := client.Do(request)
		if err == nil {
			result := readMutationResult(t, response)
			if result.status != http.StatusServiceUnavailable {
				return result
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("four-process mutation did not complete: %v", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func readMutationResult(t *testing.T, response *http.Response) mutationResult {
	t.Helper()
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("read mutation response: %v / %v", readErr, closeErr)
	}
	return mutationResult{status: response.StatusCode, body: string(body)}
}

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}
	return port
}

func awaitHealth(t *testing.T, ctx context.Context, port int) string {
	t.Helper()
	client := &http.Client{Timeout: time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/health", port)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("create health request: %v", err)
		}
		response, err := client.Do(request)
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if readErr != nil || closeErr != nil {
				t.Fatalf("read health response: %v / %v", readErr, closeErr)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("health status = %d, body = %s", response.StatusCode, body)
			}
			return string(body)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Tachyon did not become ready: %v", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(working, "..", "..", ".."))
}

func installedRuntime(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("SLUICE_REQUIRE_INSTALLED_RUNTIMES") == "1" {
			t.Fatalf("required installed runtime %q is unavailable: %v", name, err)
		}
		t.Skipf("installed runtime %q is unavailable", name)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("resolve %q: %v", name, err)
	}
	return absolute
}
