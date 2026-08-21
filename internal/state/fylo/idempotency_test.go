package fylo_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	fylostate "github.com/d31ma/sluice/internal/state/fylo"
	"github.com/d31ma/sluice/internal/transport/localapi"
)

const qualificationAPIToken = "0123456789abcdef0123456789abcdef"

func TestInstalledFyloIdempotencyReplaysAfterRestartAndRejectsConflict(t *testing.T) {
	binary := installedFyloRuntime(t)
	root := filepath.Join(t.TempDir(), "state")
	var calls atomic.Int32
	application := http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(response, `{"rootId":"root-1"}`)
	})

	first := startedSupervisor(t, binary, root)
	firstHandler := durableHandler(t, first, application)
	created := performDurableMutation(t, firstHandler, `{"path":"/sync"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("first mutation status = %d, body = %s", created.Code, created.Body.String())
	}
	closeSupervisor(t, first)

	second := startedSupervisor(t, binary, root)
	defer closeSupervisor(t, second)
	secondHandler := durableHandler(t, second, application)
	replayed := performDurableMutation(t, secondHandler, `{"path":"/sync"}`)
	if replayed.Code != created.Code || replayed.Body.String() != created.Body.String() {
		t.Fatalf("replay = (%d, %q), want (%d, %q)", replayed.Code, replayed.Body.String(), created.Code, created.Body.String())
	}
	conflict := performDurableMutation(t, secondHandler, `{"path":"/other"}`)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"code":"idempotency_key_reused"`) {
		t.Fatalf("conflict = (%d, %q)", conflict.Code, conflict.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("application calls across restart = %d, want 1", calls.Load())
	}
}

func TestInstalledFyloPendingIdempotencyFailsClosedAfterRestart(t *testing.T) {
	binary := installedFyloRuntime(t)
	root := filepath.Join(t.TempDir(), "state")
	first := startedSupervisor(t, binary, root)
	store, err := fylostate.NewIdempotencyStore(first)
	if err != nil {
		t.Fatalf("create first store: %v", err)
	}
	claim, err := store.Begin(context.Background(), "POST\n/v1/roots\npending-request-1", "fingerprint-one")
	if err != nil {
		t.Fatalf("reserve pending claim: %v", err)
	}
	claim.Abort()
	closeSupervisor(t, first)

	second := startedSupervisor(t, binary, root)
	defer closeSupervisor(t, second)
	restartedStore, err := fylostate.NewIdempotencyStore(second)
	if err != nil {
		t.Fatalf("create restarted store: %v", err)
	}
	if _, retryErr := restartedStore.Begin(context.Background(), "POST\n/v1/roots\npending-request-1", "fingerprint-one"); retryErr == nil {
		t.Fatal("pending retry error = nil, want fail-closed recovery")
	}
	if _, retryErr := restartedStore.Begin(context.Background(), "POST\n/v1/roots\npending-request-1", "fingerprint-two"); !errors.Is(retryErr, localapi.ErrIdempotencyConflict) {
		t.Fatalf("changed pending retry error = %v, want idempotency conflict", retryErr)
	}
}

func durableHandler(t *testing.T, supervisor *fylostate.Supervisor, application http.Handler) http.Handler {
	t.Helper()
	store, err := fylostate.NewIdempotencyStore(supervisor)
	if err != nil {
		t.Fatalf("create Fylo idempotency store: %v", err)
	}
	handler, err := localapi.NewHandler(qualificationAPIToken, store, application)
	if err != nil {
		t.Fatalf("create durable local API handler: %v", err)
	}
	return handler
}

func performDurableMutation(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/roots", strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:4242"
	request.Header.Set("Authorization", "Bearer "+qualificationAPIToken)
	request.Header.Set("Idempotency-Key", "restart-request-0001")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func startedSupervisor(t *testing.T, binary, root string) *fylostate.Supervisor {
	t.Helper()
	supervisor, err := fylostate.NewSupervisor(fylostate.Config{
		Binary:       binary,
		Root:         root,
		Requirements: fylostate.SluiceRequirements(qualifiedFyloVersion),
	})
	if err != nil {
		t.Fatalf("create supervisor: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := supervisor.Start(ctx); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	return supervisor
}

func closeSupervisor(t *testing.T, supervisor *fylostate.Supervisor) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := supervisor.Close(ctx); err != nil {
		t.Fatalf("close supervisor: %v", err)
	}
}
