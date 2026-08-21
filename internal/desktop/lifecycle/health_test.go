package lifecycle

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

	"github.com/d31ma/sluice/internal/transport/ipc"
)

func TestWaitForDescriptorRequiresANewValidPublication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-session.json")
	previous, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create previous capability: %v", err)
	}
	current, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create current capability: %v", err)
	}
	process := &testManagedProcess{done: make(chan struct{})}
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = ipc.WriteDescriptor(path, ipc.Descriptor{
			ProtocolVersion: ipc.ProtocolVersion,
			Network:         "tcp",
			Address:         "127.0.0.1:43129",
			Capability:      current,
		})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	descriptor, err := waitForDescriptor(ctx, descriptorWaitRequest{
		Path:               path,
		PreviousCapability: previous,
		Process:            process,
	})
	if err != nil || descriptor.Capability != current {
		t.Fatalf("waitForDescriptor() = %+v, %v", descriptor, err)
	}
}

func TestWaitForDescriptorReportsEarlyAgentExit(t *testing.T) {
	done := make(chan struct{})
	close(done)
	process := &testManagedProcess{done: done, err: errors.New("exit 7")}
	_, err := waitForDescriptor(context.Background(), descriptorWaitRequest{
		Path:    filepath.Join(t.TempDir(), "missing.json"),
		Process: process,
	})
	if err == nil || !strings.Contains(err.Error(), "exit 7") {
		t.Fatalf("waitForDescriptor() error = %v", err)
	}
}

func TestAwaitAuthenticatedHealthRetriesUntilExactHealthyResponse(t *testing.T) {
	var calls atomic.Int32
	token := strings.Repeat("t", 32)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+token {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		if calls.Add(1) == 1 {
			response.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"status":"ok"}`)
	}))
	defer server.Close()
	process := &testManagedProcess{done: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := awaitAuthenticatedHealth(ctx, server.URL, token, process); err != nil {
		t.Fatalf("awaitAuthenticatedHealth() error = %v", err)
	}
	if calls.Load() < 2 {
		t.Fatalf("health calls = %d, want retry", calls.Load())
	}
}

func TestProbeHealthRejectsCredentialsAndMalformedSuccess(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{}`},
		{name: "forbidden", status: http.StatusForbidden, body: `{}`},
		{name: "malformed success", status: http.StatusOK, body: `{"status":"wrong"}`},
		{name: "oversized", status: http.StatusOK, body: strings.Repeat("x", maximumHealthBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(test.status)
				_, _ = io.WriteString(response, test.body)
			}))
			defer server.Close()
			client := &http.Client{Timeout: time.Second}
			healthy, err := probeHealthOnce(context.Background(), client, server.URL, strings.Repeat("t", 32))
			if healthy || err == nil {
				t.Fatalf("probeHealthOnce() = %t, %v", healthy, err)
			}
		})
	}
}

type testManagedProcess struct {
	done chan struct{}
	err  error
}

func (process *testManagedProcess) Done() <-chan struct{} { return process.done }
func (process *testManagedProcess) Err() error            { return process.err }
func (*testManagedProcess) Close(context.Context) error   { return nil }
