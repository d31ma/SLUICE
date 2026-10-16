package ipc

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestCloseCancelsSaturatedHandlersAndPendingConnections(t *testing.T) {
	started := make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(started)
		<-request.Context().Done()
	})
	capability, err := NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	server, err := Listen(ServerConfig{
		Address:            "127.0.0.1:0",
		Capability:         capability,
		Handler:            handler,
		MaximumConnections: 1,
	})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.Serve() }()
	client, err := NewClient(server.Descriptor())
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
	waitForTrackedConnections(t, server, 2)
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

func waitForTrackedConnections(t *testing.T, server *Server, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		server.admission.Lock()
		count := len(server.connections)
		server.admission.Unlock()
		if count == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("tracked connections did not reach %d", want)
}
