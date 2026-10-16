package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
	"github.com/d31ma/sluice/internal/transport/ipc"
)

func TestCoordinatorStartsAndStopsThePackagedTopologyInDependencyOrder(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state")
	bundle := lifecycleBundleFixture(t)
	events := make([]string, 0, 6)
	var projectRoot string
	agent := &recordingProcess{name: "agent", events: &events}
	tachyon := &recordingProcess{name: "tachyon", events: &events}
	coordinator, err := newCoordinator(Config{
		Bundle:         bundle,
		StateRoot:      stateRoot,
		RuntimeVersion: "26.33.02",
		StartupTimeout: time.Second,
	}, dependencies{
		startAgent: func(_ context.Context, request agentRequest) (managedProcess, error) {
			events = append(events, "agent:start")
			assertAgentRequest(t, request, bundle, stateRoot)
			return agent, nil
		},
		waitDescriptor: func(_ context.Context, request descriptorWaitRequest) (ipc.Descriptor, error) {
			events = append(events, "descriptor:ready")
			if request.Path != filepath.Join(stateRoot, "run", "agent-session.json") {
				t.Fatalf("session path = %q", request.Path)
			}
			if request.Process != agent {
				t.Fatal("descriptor wait did not observe agent process")
			}
			return ipc.Descriptor{}, nil
		},
		startTachyon: func(_ context.Context, request tachyonRequest) (managedProcess, error) {
			events = append(events, "tachyon:start")
			projectRoot = request.ProjectRoot
			if request.Bundle != bundle || request.Port == 0 {
				t.Fatalf("tachyon request = %+v", request)
			}
			configuration, readErr := os.ReadFile(filepath.Join(request.ProjectRoot, ".tachyonrc"))
			if readErr != nil {
				t.Fatalf("read materialized .tachyonrc: %v", readErr)
			}
			var parsed struct {
				Interpreters map[string][]string `json:"interpreters"`
			}
			if decodeErr := json.Unmarshal(configuration, &parsed); decodeErr != nil {
				t.Fatalf("decode materialized .tachyonrc: %v", decodeErr)
			}
			arguments := parsed.Interpreters["sluice"]
			if len(arguments) != 3 || arguments[0] != bundle.YonBinary {
				t.Fatalf(".tachyonrc interpreter = %#v, want bundled Yon path", arguments)
			}
			return tachyon, nil
		},
		probeHealth: func(_ context.Context, endpoint, token string, process managedProcess) error {
			events = append(events, "health:authenticated")
			if !strings.HasPrefix(endpoint, "http://127.0.0.1:") || len(token) < 32 || process != tachyon {
				t.Fatalf("health probe = endpoint %q, token length %d, process %#v", endpoint, len(token), process)
			}
			return nil
		},
		choosePort:   func() (int, error) { return 43129, nil },
		restartDelay: boundedRestartDelay,
		waitRestart:  waitForRestart,
	})
	if err != nil {
		t.Fatalf("newCoordinator() error = %v", err)
	}

	session, err := coordinator.Start(context.Background())
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if session.Endpoint() != "http://127.0.0.1:43129" {
		t.Fatalf("Endpoint() = %q", session.Endpoint())
	}
	shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := session.Close(shutdown); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	want := []string{
		"agent:start", "descriptor:ready", "tachyon:start", "health:authenticated",
		"tachyon:close", "agent:close",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
	if _, err := os.Stat(projectRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ephemeral Tachyon project remains after shutdown: %v", err)
	}
}

func TestCoordinatorCleansUpAgentWhenTachyonCannotStart(t *testing.T) {
	events := make([]string, 0, 4)
	agent := &recordingProcess{name: "agent", events: &events}
	coordinator, err := newCoordinator(Config{
		Bundle:         lifecycleBundleFixture(t),
		StateRoot:      filepath.Join(t.TempDir(), "state"),
		RuntimeVersion: "26.33.02",
		StartupTimeout: time.Second,
	}, dependencies{
		startAgent: func(context.Context, agentRequest) (managedProcess, error) {
			events = append(events, "agent:start")
			return agent, nil
		},
		waitDescriptor: func(context.Context, descriptorWaitRequest) (ipc.Descriptor, error) {
			events = append(events, "descriptor:ready")
			return ipc.Descriptor{}, nil
		},
		startTachyon: func(context.Context, tachyonRequest) (managedProcess, error) {
			events = append(events, "tachyon:start")
			return nil, errors.New("Tachyon unavailable")
		},
		probeHealth: func(context.Context, string, string, managedProcess) error {
			return errors.New("health must not run")
		},
		choosePort:   func() (int, error) { return 43129, nil },
		restartDelay: boundedRestartDelay,
		waitRestart:  waitForRestart,
	})
	if err != nil {
		t.Fatalf("newCoordinator() error = %v", err)
	}
	if _, err := coordinator.Start(context.Background()); err == nil {
		t.Fatal("Start() error = nil")
	}
	want := []string{"agent:start", "descriptor:ready", "tachyon:start", "agent:close"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

func TestLoadOrCreateTokenIsStablePrivateAndRejectsSymlinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "api-token")
	first, err := loadOrCreateToken(path)
	if err != nil {
		t.Fatalf("loadOrCreateToken() create error = %v", err)
	}
	second, err := loadOrCreateToken(path)
	if err != nil {
		t.Fatalf("loadOrCreateToken() reload error = %v", err)
	}
	if first != second || len(first) < 32 {
		t.Fatalf("token stability/length = %t/%d", first == second, len(first))
	}
	metadata, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat token: %v", err)
	}
	if runtime.GOOS != "windows" && metadata.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 600", metadata.Mode().Perm())
	}

	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte(strings.Repeat("x", 32)), 0o600); err != nil {
		t.Fatalf("write symlink target: %v", err)
	}
	link := filepath.Join(t.TempDir(), "token-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := loadOrCreateToken(link); err == nil {
		t.Fatal("loadOrCreateToken() accepted a symlink")
	}
}

func assertAgentRequest(t *testing.T, request agentRequest, bundle runtimeexec.MacOSBundle, stateRoot string) {
	t.Helper()
	if request.AgentBinary != bundle.AgentBinary || request.FyloBinary != bundle.FyloBinary {
		t.Fatalf("agent runtime paths = %+v", request)
	}
	if request.FyloRoot != filepath.Join(stateRoot, "fylo") || request.RuntimeVersion != "26.33.02" {
		t.Fatalf("agent state request = %+v", request)
	}
	if request.TokenFile != filepath.Join(stateRoot, "run", "api-token") ||
		request.SessionFile != filepath.Join(stateRoot, "run", "agent-session.json") {
		t.Fatalf("agent private paths = %+v", request)
	}
}

type recordingProcess struct {
	events *[]string
	done   chan struct{}
	name   string
}

func (process *recordingProcess) Done() <-chan struct{} {
	if process.done == nil {
		process.done = make(chan struct{})
	}
	return process.done
}

func (*recordingProcess) Err() error { return nil }

func (process *recordingProcess) Close(context.Context) error {
	*process.events = append(*process.events, process.name+":close")
	return nil
}

func lifecycleBundleFixture(t *testing.T) runtimeexec.MacOSBundle {
	t.Helper()
	root := t.TempDir()
	project := filepath.Join(root, "tachyon")
	route := filepath.Join(project, "server", "routes", "v1", "health", "yon.sluice")
	if err := os.MkdirAll(filepath.Dir(route), 0o700); err != nil {
		t.Fatalf("create Tachyon route directory: %v", err)
	}
	if err := os.WriteFile(route, []byte("Sluice packaged health route\n"), 0o600); err != nil {
		t.Fatalf("write Tachyon route: %v", err)
	}
	return runtimeexec.MacOSBundle{
		Root:           root,
		AgentBinary:    filepath.Join(root, "sluice-agent"),
		YonBinary:      filepath.Join(root, "sluice-yon"),
		TachyonBinary:  filepath.Join(root, "ty"),
		FyloBinary:     filepath.Join(root, "fylo"),
		TachyonProject: project,
		HealthRoute:    route,
	}
}
