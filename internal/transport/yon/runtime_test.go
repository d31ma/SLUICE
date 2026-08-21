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
	"testing"
	"time"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
	"github.com/d31ma/sluice/internal/transport/yon"
)

const qualifiedTachyonVersion = "26.33.02"

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
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("find Go: %v", err)
	}
	name := "sluice-yon"
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
	command := executable.Command(
		ctx,
		runtimeexec.AllowEnvironment("PATH", "HOME", "TMPDIR", "TMP", "TEMP", "SystemRoot", "GOTOOLCHAIN"),
		"build", "-o", output, "./internal/transport/yon/cmd/sluice-yon",
	)
	command.Dir = repositoryRoot(t)
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build Go Yon handler: %v\n%s", err, result)
	}
	return output
}

func registerGoHandler(t *testing.T, project, handler, sessionFile string) {
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
	route := filepath.Join(project, "server", "routes", "v1", "health")
	if err := os.MkdirAll(route, 0o700); err != nil {
		t.Fatalf("create health route: %v", err)
	}
	if err := os.WriteFile(filepath.Join(route, "yon.sluice"), []byte("registered Go executable\n"), 0o600); err != nil {
		t.Fatalf("write route marker: %v", err)
	}
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
