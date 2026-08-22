//go:build darwin || linux

package lifecycle

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

func TestGenerationResourcesRemoveTheCapturedUnixSocket(t *testing.T) {
	path, listener := persistentUnixSocket(t)
	resources := generationResources{endpoint: captureNativeEndpoint(ipc.Descriptor{
		Network: "unix",
		Address: path,
	})}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	if err := resources.cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("captured Unix socket remains: %v", err)
	}
}

func TestGenerationResourcesPreserveAReplacementUnixEndpoint(t *testing.T) {
	path, listener := persistentUnixSocket(t)
	resources := generationResources{endpoint: captureNativeEndpoint(ipc.Descriptor{
		Network: "unix",
		Address: path,
	})}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove captured socket: %v", err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatalf("write replacement endpoint: %v", err)
	}
	if err := resources.cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	contents, err := os.ReadFile(path) // #nosec G304 -- test-owned path beneath t.TempDir.
	if err != nil || string(contents) != "replacement" {
		t.Fatalf("replacement endpoint = %q, %v", contents, err)
	}
}

func persistentUnixSocket(t *testing.T) (string, *net.UnixListener) {
	t.Helper()
	directory, err := os.MkdirTemp("", "sluice-resource-")
	if err != nil {
		t.Fatalf("create short socket directory: %v", err)
	}
	path := filepath.Join(directory, "agent.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatalf("listen on Unix socket: %v", err)
	}
	listener.SetUnlinkOnClose(false)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(path)
		_ = os.Remove(directory)
	})
	return path, listener
}
