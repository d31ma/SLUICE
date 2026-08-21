package ipc_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/d31ma/sluice/internal/transport/ipc"
	"github.com/d31ma/sluice/internal/transport/localapi"
)

const apiToken = "0123456789abcdef0123456789abcdef"

func TestClientPropagatesAuthenticationAndDuplicateMutation(t *testing.T) {
	var calls atomic.Int32
	application := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Header.Get("Authorization") != "Bearer "+apiToken {
			t.Fatal("authorization header did not reach the persistent handler")
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(response, `{"rootId":"root-1"}`)
	})
	handler, err := localapi.NewHandler(apiToken, localapi.NewMemoryIdempotencyStore(), application)
	if err != nil {
		t.Fatalf("create local API handler: %v", err)
	}
	server, client := startServer(t, handler)
	defer closeServer(t, server)

	first := roundTrip(t, client, http.MethodPost, "/v1/roots", `{"path":"/sync"}`, "request-00000001")
	second := roundTrip(t, client, http.MethodPost, "/v1/roots", `{"path":"/sync"}`, "request-00000001")

	if calls.Load() != 1 {
		t.Fatalf("application calls = %d, want 1", calls.Load())
	}
	if first.StatusCode != http.StatusCreated || second.StatusCode != first.StatusCode {
		t.Fatalf("statuses = %d and %d, want %d", first.StatusCode, second.StatusCode, http.StatusCreated)
	}
	if string(first.Body) != string(second.Body) {
		t.Fatalf("duplicate response = %q, want %q", second.Body, first.Body)
	}
}

func TestClientRejectsWrongSessionCapability(t *testing.T) {
	server, client := startServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("wrong session capability reached the handler")
	}))
	defer closeServer(t, server)
	descriptor := client.Descriptor()
	descriptor.Capability = strings.Repeat("A", 43)
	unauthorized, err := ipc.NewClient(descriptor)
	if err != nil {
		t.Fatalf("create unauthorized client: %v", err)
	}

	request, err := http.NewRequest(http.MethodGet, "http://sluice.local/v1/health", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if _, err := unauthorized.RoundTrip(context.Background(), request); !errors.Is(err, ipc.ErrUnauthenticated) {
		t.Fatalf("round trip error = %v, want ErrUnauthenticated", err)
	}
}

func TestClientFailsClosedOnProtocolMismatch(t *testing.T) {
	server, client := startServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer closeServer(t, server)
	descriptor := client.Descriptor()
	descriptor.ProtocolVersion++

	if _, err := ipc.NewClient(descriptor); !errors.Is(err, ipc.ErrProtocolMismatch) {
		t.Fatalf("NewClient() error = %v, want ErrProtocolMismatch", err)
	}
}

func TestClientRejectsOversizedRequestBeforeDial(t *testing.T) {
	server, client := startServer(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer closeServer(t, server)
	request, err := http.NewRequest(
		http.MethodPost,
		"http://sluice.local/v1/roots",
		strings.NewReader(strings.Repeat("x", ipc.MaxRequestBytes)),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	if _, err := client.RoundTrip(context.Background(), request); !errors.Is(err, ipc.ErrFrameTooLarge) {
		t.Fatalf("round trip error = %v, want ErrFrameTooLarge", err)
	}
}

func TestClientReportsUnavailableAgentAfterCleanShutdown(t *testing.T) {
	server, client := startServer(t, http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}))
	closeServer(t, server)
	request, err := http.NewRequest(http.MethodGet, "http://sluice.local/v1/health", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if _, err := client.RoundTrip(ctx, request); !errors.Is(err, ipc.ErrUnavailable) {
		t.Fatalf("round trip error = %v, want ErrUnavailable", err)
	}
}

func TestCloseCancelsSaturatedHandlersAndPendingConnections(t *testing.T) {
	started := make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	})
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	server, err := ipc.Listen(ipc.ServerConfig{
		Address:            "127.0.0.1:0",
		Capability:         capability,
		Handler:            handler,
		MaximumConnections: 1,
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.Serve() }()
	client, err := ipc.NewClient(server.Descriptor())
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	request := func() *http.Request {
		result, requestErr := http.NewRequest(http.MethodGet, "http://sluice.local/v1/health", nil)
		if requestErr != nil {
			t.Fatalf("create request: %v", requestErr)
		}
		return result
	}
	firstDone := make(chan error, 1)
	go func() {
		_, roundTripErr := client.RoundTrip(context.Background(), request())
		firstDone <- roundTripErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first handler did not start")
	}
	secondDone := make(chan error, 1)
	go func() {
		_, roundTripErr := client.RoundTrip(context.Background(), request())
		secondDone <- roundTripErr
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatalf("close saturated server: %v", err)
	}
	for index, done := range []<-chan error{firstDone, secondDone} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("client %d remained blocked after close", index+1)
		}
	}
}

func TestSessionDescriptorRoundTripAndPermissions(t *testing.T) {
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	descriptor := ipc.Descriptor{
		ProtocolVersion: ipc.ProtocolVersion,
		Network:         "tcp",
		Address:         "127.0.0.1:41234",
		Capability:      capability,
	}
	path := filepath.Join(t.TempDir(), "agent-session.json")
	if writeErr := ipc.WriteDescriptor(path, descriptor); writeErr != nil {
		t.Fatalf("write descriptor: %v", writeErr)
	}
	loaded, err := ipc.ReadDescriptor(path)
	if err != nil {
		t.Fatalf("read descriptor: %v", err)
	}
	if loaded != descriptor {
		t.Fatalf("loaded descriptor = %#v, want %#v", loaded, descriptor)
	}
}

func TestWriteDescriptorRejectsRelativePath(t *testing.T) {
	descriptor := validDescriptor(t)

	if err := ipc.WriteDescriptor("agent-session.json", descriptor); err == nil {
		t.Fatal("WriteDescriptor() error = nil, want relative-path rejection")
	}
}

func TestReadDescriptorRejectsOversizedAndTrailingData(t *testing.T) {
	tests := map[string]string{
		"oversized": strings.Repeat("x", 4097),
		"trailing":  `{"protocol_version":1,"network":"tcp","address":"127.0.0.1:1234","capability":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {}`,
	}
	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "agent-session.json")
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatalf("write descriptor fixture: %v", err)
			}
			if _, err := ipc.ReadDescriptor(path); !errors.Is(err, ipc.ErrUnavailable) {
				t.Fatalf("ReadDescriptor() error = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestReadDescriptorRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "agent-session.json")
	if err := ipc.WriteDescriptor(path, validDescriptor(t)); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}
	// #nosec G302 -- broad permissions are the security condition under test.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("broaden descriptor permissions: %v", err)
	}
	if _, err := ipc.ReadDescriptor(path); !errors.Is(err, ipc.ErrUnavailable) {
		t.Fatalf("ReadDescriptor() error = %v, want ErrUnavailable", err)
	}
}

func validDescriptor(t *testing.T) ipc.Descriptor {
	t.Helper()
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	return ipc.Descriptor{
		ProtocolVersion: ipc.ProtocolVersion,
		Network:         "tcp",
		Address:         "127.0.0.1:41234",
		Capability:      capability,
	}
}

func startServer(t *testing.T, handler http.Handler) (*ipc.Server, *ipc.Client) {
	t.Helper()
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	server, err := ipc.Listen(ipc.ServerConfig{
		Address:    "127.0.0.1:0",
		Capability: capability,
		Handler:    handler,
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.Serve() }()
	client, err := ipc.NewClient(server.Descriptor())
	if err != nil {
		_ = server.Close(context.Background())
		t.Fatalf("create client: %v", err)
	}
	return server, client
}

func closeServer(t *testing.T, server *ipc.Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatalf("close server: %v", err)
	}
}

func roundTrip(t *testing.T, client *ipc.Client, method, path, body, key string) *ipc.HTTPResponse {
	t.Helper()
	request, err := http.NewRequest(method, "http://sluice.local"+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+apiToken)
	request.Header.Set("Idempotency-Key", key)
	response, err := client.RoundTrip(context.Background(), request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	return response
}
