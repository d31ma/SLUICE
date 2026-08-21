package agent_test

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d31ma/sluice/internal/agent"
	"github.com/d31ma/sluice/internal/transport/ipc"
	"github.com/d31ma/sluice/internal/transport/localapi"
)

const apiToken = "0123456789abcdef0123456789abcdef"

func TestServiceOwnsStateAndServesAuthenticatedHealthUntilShutdown(t *testing.T) {
	owner := &fakeOwner{}
	sessionFile := filepath.Join(t.TempDir(), "agent-session.json")
	service, err := agent.NewService(agent.Config{
		State:       owner,
		Idempotency: localapi.NewMemoryIdempotencyStore(),
		APIToken:    apiToken,
		SessionFile: sessionFile,
		Listen:      "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx) }()

	descriptor := awaitDescriptor(t, sessionFile)
	client, err := ipc.NewClient(descriptor)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, "http://sluice.local/v1/health", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+apiToken)
	response, err := client.RoundTrip(context.Background(), request)
	if err != nil {
		t.Fatalf("health round trip: %v", err)
	}
	if response.StatusCode != http.StatusOK || string(response.Body) != `{"status":"ok"}` {
		t.Fatalf("health response = %d %q", response.StatusCode, response.Body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("service shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("service did not shut down")
	}
	started, closed := owner.counts()
	if started != 1 || closed != 1 {
		t.Fatalf("state lifecycle = start %d, close %d; want 1, 1", started, closed)
	}
	if _, err := ipc.ReadDescriptor(sessionFile); !errors.Is(err, ipc.ErrUnavailable) {
		t.Fatalf("descriptor after shutdown error = %v, want ErrUnavailable", err)
	}
}

func TestServiceDoesNotPublishSessionWhenStateCannotStart(t *testing.T) {
	owner := &fakeOwner{startErr: errors.New("state unavailable")}
	sessionFile := filepath.Join(t.TempDir(), "agent-session.json")
	service, err := agent.NewService(agent.Config{
		State:       owner,
		Idempotency: localapi.NewMemoryIdempotencyStore(),
		APIToken:    apiToken,
		SessionFile: sessionFile,
		Listen:      "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	if err := service.Run(context.Background()); err == nil {
		t.Fatal("Run() error = nil, want state startup failure")
	}
	if _, err := ipc.ReadDescriptor(sessionFile); !errors.Is(err, ipc.ErrUnavailable) {
		t.Fatalf("descriptor error = %v, want ErrUnavailable", err)
	}
}

func TestRestartFailsClosedForMutationsWithoutDurableIdempotency(t *testing.T) {
	var calls int
	application := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ })
	sessionFile := filepath.Join(t.TempDir(), "agent-session.json")
	for attempt := 0; attempt < 2; attempt++ {
		service, err := agent.NewService(agent.Config{
			State:       &fakeOwner{},
			Application: application,
			Idempotency: agent.RejectingIdempotencyStore{},
			APIToken:    apiToken,
			SessionFile: sessionFile,
			Listen:      "127.0.0.1:0",
		})
		if err != nil {
			t.Fatalf("create service attempt %d: %v", attempt, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- service.Run(ctx) }()
		client, err := ipc.NewClient(awaitDescriptor(t, sessionFile))
		if err != nil {
			cancel()
			t.Fatalf("create client: %v", err)
		}
		request, err := http.NewRequest(http.MethodPost, "http://sluice.local/v1/roots", strings.NewReader(`{}`))
		if err != nil {
			cancel()
			t.Fatalf("create mutation: %v", err)
		}
		request.Header.Set("Authorization", "Bearer "+apiToken)
		request.Header.Set("Idempotency-Key", "restart-request-0001")
		response, err := client.RoundTrip(context.Background(), request)
		if err != nil {
			cancel()
			t.Fatalf("mutation round trip: %v", err)
		}
		if response.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(response.Body), `"code":"state_unavailable"`) {
			cancel()
			t.Fatalf("mutation response = %d %q", response.StatusCode, response.Body)
		}
		cancel()
		if err := <-done; err != nil {
			t.Fatalf("shutdown attempt %d: %v", attempt, err)
		}
	}
	if calls != 0 {
		t.Fatalf("unsafe mutation calls across restart = %d, want 0", calls)
	}
}

type fakeOwner struct {
	startErr error
	mu       sync.Mutex
	started  int
	closed   int
}

func (owner *fakeOwner) Start(context.Context) error {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.started++
	return owner.startErr
}

func (owner *fakeOwner) Close(context.Context) error {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.closed++
	return nil
}

func (owner *fakeOwner) counts() (int, int) {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.started, owner.closed
}

func awaitDescriptor(t *testing.T, path string) ipc.Descriptor {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		descriptor, err := ipc.ReadDescriptor(path)
		if err == nil {
			return descriptor
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("agent session descriptor was not published")
	return ipc.Descriptor{}
}
