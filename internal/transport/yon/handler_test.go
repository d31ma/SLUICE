package yon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/d31ma/sluice/internal/transport/ipc"
	"github.com/d31ma/sluice/internal/transport/yon"
)

func TestServeHealthRequest(t *testing.T) {
	server, sessionFile := startAgent(t, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer api-secret" {
			t.Fatal("authorization header was not propagated to the agent")
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(response, `{"source":"persistent-agent"}`)
	}))
	defer closeAgent(t, server)

	request := `{"protocol_version":1,"kind":"request","request_id":"ABC123","operation":"health.get","route":"/v1/health","method":"GET","headers":{"authorization":["Bearer api-secret"]}}`
	var output bytes.Buffer
	if err := yon.ServeWithSessionFile(strings.NewReader(request), &output, sessionFile); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var response struct {
		Body   string `json:"body"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode response %q: %v", output.Bytes(), err)
	}
	if response.Status != 200 || response.Body != `{"source":"persistent-agent"}` {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestServeReturnsStableUnavailableResponse(t *testing.T) {
	request := `{"protocol_version":1,"kind":"request","request_id":"ABC123","operation":"health.get","route":"/v1/health","method":"GET","headers":{}}`
	var output bytes.Buffer
	if err := yon.ServeWithSessionFile(strings.NewReader(request), &output, filepath.Join(t.TempDir(), "missing.json")); err != nil {
		t.Fatalf("serve unavailable agent: %v", err)
	}
	var response yon.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != http.StatusServiceUnavailable || !strings.Contains(response.Body, `"code":"agent_unavailable"`) {
		t.Fatalf("unavailable response = %#v", response)
	}
}

func TestServeRejectsUnknownProtocolVersion(t *testing.T) {
	t.Parallel()

	request := `{"protocol_version":999,"kind":"request","request_id":"ABC123","operation":"health.get","route":"/v1/health","method":"GET","headers":{}}`
	var output bytes.Buffer
	if err := yon.Serve(strings.NewReader(request), &output); err == nil {
		t.Fatal("unknown protocol version was accepted")
	}
	if output.Len() != 0 {
		t.Fatalf("unknown protocol emitted a response: %q", output.Bytes())
	}
}

func startAgent(t *testing.T, handler http.Handler) (*ipc.Server, string) {
	t.Helper()
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	server, err := ipc.Listen(ipc.ServerConfig{Address: "127.0.0.1:0", Capability: capability, Handler: handler})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = server.Serve() }()
	path := filepath.Join(t.TempDir(), "agent-session.json")
	if err := ipc.WriteDescriptor(path, server.Descriptor()); err != nil {
		_ = server.Close(context.Background())
		t.Fatalf("write descriptor: %v", err)
	}
	return server, path
}

func closeAgent(t *testing.T, server *ipc.Server) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		t.Fatalf("close agent: %v", err)
	}
}
