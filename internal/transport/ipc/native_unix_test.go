//go:build darwin || linux

package ipc_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

func TestNativeListenerRejectsAlreadyActiveUnixSocket(t *testing.T) {
	sessionFile := filepath.Join(t.TempDir(), "session", "agent-session.json")
	server := startNativeServer(t, sessionFile)
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	_, err = ipc.ListenNative(ipc.ServerConfig{
		Capability: capability,
		Handler:    http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	}, sessionFile)
	if err == nil {
		t.Fatal("ListenNative() error = nil, want active-socket rejection")
	}
	closeServer(t, server)
}

func TestNativeListenerRecoversOwnedStaleUnixSocket(t *testing.T) {
	sessionFile := filepath.Join(t.TempDir(), "session", "agent-session.json")
	server := startNativeServer(t, sessionFile)
	endpoint := server.Descriptor().Address
	closeServer(t, server)

	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: endpoint, Net: "unix"})
	if err != nil {
		t.Fatalf("create stale Unix socket: %v", err)
	}
	stale.SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatalf("close stale Unix socket: %v", err)
	}
	server = startNativeServer(t, sessionFile)
	closeServer(t, server)
	if _, err := os.Lstat(endpoint); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovered endpoint after close error = %v, want not-exist", err)
	}
}

func TestNativeListenerUsesPrivateUnixSocketAndRemovesItOnClose(t *testing.T) {
	sessionFile := filepath.Join(t.TempDir(), "session", "agent-session.json")
	server := startNativeServer(t, sessionFile)
	descriptor := server.Descriptor()
	if descriptor.Network != "unix" {
		t.Fatalf("network = %q, want unix", descriptor.Network)
	}
	metadata, err := os.Lstat(descriptor.Address)
	if err != nil {
		t.Fatalf("inspect Unix socket: %v", err)
	}
	if metadata.Mode()&os.ModeSocket == 0 || metadata.Mode().Perm() != 0o600 {
		t.Fatalf("Unix socket mode = %v, want socket 0600", metadata.Mode())
	}

	client, err := ipc.NewClient(descriptor)
	if err != nil {
		t.Fatalf("create native client: %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, "http://sluice.local/v1/health", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err := client.RoundTrip(context.Background(), request)
	if err != nil {
		t.Fatalf("native round trip: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	closeServer(t, server)
	if _, err := os.Lstat(descriptor.Address); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket after close error = %v, want not-exist", err)
	}
}

func TestNativeListenerRejectsBroadSessionDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "session")
	if err := os.Mkdir(directory, 0o770); err != nil { // #nosec G301 -- Broad permissions are the security condition under test.
		t.Fatalf("create broad session directory: %v", err)
	}
	if err := os.Chmod(directory, 0o770); err != nil { // #nosec G302 -- Broad permissions are the security condition under test.
		t.Fatalf("broaden session directory: %v", err)
	}
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	_, err = ipc.ListenNative(ipc.ServerConfig{
		Capability: capability,
		Handler:    http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	}, filepath.Join(directory, "agent-session.json"))
	if err == nil {
		t.Fatal("ListenNative() error = nil, want broad-directory rejection")
	}
}

func TestNativeListenerRejectsAndPreservesNonSocketEndpoint(t *testing.T) {
	sessionFile := filepath.Join(t.TempDir(), "session", "agent-session.json")
	server := startNativeServer(t, sessionFile)
	endpoint := server.Descriptor().Address
	closeServer(t, server)
	t.Cleanup(func() { _ = os.Remove(endpoint) })
	if err := os.WriteFile(endpoint, []byte("do not remove"), 0o600); err != nil {
		t.Fatalf("write endpoint fixture: %v", err)
	}
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	_, err = ipc.ListenNative(ipc.ServerConfig{
		Capability: capability,
		Handler:    http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
	}, sessionFile)
	if err == nil {
		t.Fatal("ListenNative() error = nil, want non-socket rejection")
	}
	contents, readErr := os.ReadFile(endpoint) // #nosec G304 -- Test-owned endpoint from a validated native descriptor.
	if readErr != nil || string(contents) != "do not remove" {
		t.Fatalf("endpoint fixture = %q, %v; want preserved", contents, readErr)
	}
}

func TestNativeCloseDoesNotRemoveReplacementEndpoint(t *testing.T) {
	sessionFile := filepath.Join(t.TempDir(), "session", "agent-session.json")
	server := startNativeServer(t, sessionFile)
	endpoint := server.Descriptor().Address
	t.Cleanup(func() { _ = os.Remove(endpoint) })
	if err := os.Remove(endpoint); err != nil {
		t.Fatalf("unlink live socket: %v", err)
	}
	if err := os.WriteFile(endpoint, []byte("replacement"), 0o600); err != nil {
		t.Fatalf("write replacement: %v", err)
	}
	closeServer(t, server)
	contents, err := os.ReadFile(endpoint) // #nosec G304 -- Test-owned endpoint from a validated native descriptor.
	if err != nil || string(contents) != "replacement" {
		t.Fatalf("replacement after close = %q, %v; want preserved", contents, err)
	}
}

func startNativeServer(t *testing.T, sessionFile string) *ipc.Server {
	t.Helper()
	capability, err := ipc.NewCapability()
	if err != nil {
		t.Fatalf("create capability: %v", err)
	}
	server, err := ipc.ListenNative(ipc.ServerConfig{
		Capability: capability,
		Handler: http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusNoContent)
		}),
	}, sessionFile)
	if err != nil {
		t.Fatalf("listen native: %v", err)
	}
	go func() { _ = server.Serve() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Close(ctx)
	})
	return server
}
