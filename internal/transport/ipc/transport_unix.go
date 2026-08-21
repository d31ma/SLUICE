//go:build darwin || linux

package ipc

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const maximumPortableUnixSocketPathBytes = 103

func nativeEndpoint(sessionFile, _ string) (string, string, error) {
	clean, err := descriptorPath(sessionFile)
	if err != nil {
		return "", "", err
	}
	if err := prepareDescriptorDirectory(filepath.Dir(clean)); err != nil {
		return "", "", err
	}
	directory := filepath.Join(os.TempDir(), fmt.Sprintf("sluice-ipc-%d", os.Getuid()))
	digest := sha256.Sum256([]byte(clean))
	address := filepath.Join(directory, fmt.Sprintf("%x.sock", digest[:12]))
	if len([]byte(address)) > maximumPortableUnixSocketPathBytes {
		return "", "", fmt.Errorf("unix agent IPC path exceeds %d bytes", maximumPortableUnixSocketPathBytes)
	}
	return "unix", address, nil
}

func validateTransportAddress(network, address string) error {
	switch network {
	case "tcp":
		return validateLoopbackTCP(address)
	case "unix":
		return validateUnixAddress(address)
	default:
		return unsupportedTransport()
	}
}

func validateUnixAddress(address string) error {
	if address == "" || !filepath.IsAbs(address) || filepath.Clean(address) != address {
		return fmt.Errorf("%w: invalid Unix socket address", ErrUnavailable)
	}
	if len([]byte(address)) > maximumPortableUnixSocketPathBytes {
		return fmt.Errorf("%w: Unix socket address is too long", ErrUnavailable)
	}
	return nil
}

func listenTransport(network, address string) (net.Listener, func() error, error) {
	if network == "tcp" {
		listener, err := net.Listen(network, address)
		return listener, noTransportCleanup, err
	}
	if network != "unix" {
		return nil, nil, unsupportedTransport()
	}
	return listenPrivateUnixSocket(address)
}

func dialTransport(ctx context.Context, network, address string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, address)
}

func listenPrivateUnixSocket(address string) (net.Listener, func() error, error) {
	if err := preparePrivateUnixSocketDirectory(filepath.Dir(address)); err != nil {
		return nil, nil, fmt.Errorf("prepare Unix agent IPC directory: %w", err)
	}
	if err := removeStaleUnixSocket(address); err != nil {
		return nil, nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		return nil, nil, err
	}
	listener.SetUnlinkOnClose(false)
	cleanup, err := protectUnixSocket(address)
	if err != nil {
		_ = listener.Close()
		_ = os.Remove(address)
		return nil, nil, err
	}
	return listener, cleanup, nil
}

func preparePrivateUnixSocketDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create Unix agent IPC directory: %w", err)
	}
	metadata, err := os.Lstat(directory)
	if err != nil || !metadata.IsDir() || metadata.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unix agent IPC directory must be a real directory")
	}
	stat, ok := metadata.Sys().(*syscall.Stat_t)
	if !ok || !ownedByCurrentUnixUser(stat.Uid) || metadata.Mode().Perm() != 0o700 {
		return fmt.Errorf("unix agent IPC directory must be private and owned by the current user")
	}
	return nil
}

func removeStaleUnixSocket(address string) error {
	metadata, err := os.Lstat(address)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Unix agent IPC endpoint: %w", err)
	}
	if !isOwnedUnixSocket(metadata) {
		return fmt.Errorf("unix agent IPC endpoint is not a current-user socket")
	}
	connection, dialErr := net.DialTimeout("unix", address, 100*time.Millisecond)
	if dialErr == nil {
		_ = connection.Close()
		return fmt.Errorf("unix agent IPC endpoint is already active")
	}
	if !errors.Is(dialErr, syscall.ECONNREFUSED) {
		return fmt.Errorf("verify stale Unix agent IPC endpoint: %w", dialErr)
	}
	if err := os.Remove(address); err != nil {
		return fmt.Errorf("remove stale Unix agent IPC endpoint: %w", err)
	}
	return nil
}

func protectUnixSocket(address string) (func() error, error) {
	if err := os.Chmod(address, 0o600); err != nil {
		return nil, fmt.Errorf("protect Unix agent IPC endpoint: %w", err)
	}
	created, err := os.Lstat(address)
	if err != nil || !isPrivateOwnedUnixSocket(created) {
		return nil, fmt.Errorf("verify Unix agent IPC endpoint permissions")
	}
	return func() error { return removeSameUnixSocket(address, created) }, nil
}

func isPrivateOwnedUnixSocket(metadata os.FileInfo) bool {
	return isOwnedUnixSocket(metadata) && metadata.Mode().Perm() == 0o600
}

func isOwnedUnixSocket(metadata os.FileInfo) bool {
	stat, ok := metadata.Sys().(*syscall.Stat_t)
	return ok && ownedByCurrentUnixUser(stat.Uid) &&
		metadata.Mode()&os.ModeSocket != 0
}

func ownedByCurrentUnixUser(owner uint32) bool {
	return strconv.FormatUint(uint64(owner), 10) == strconv.Itoa(os.Getuid())
}

func removeSameUnixSocket(address string, created os.FileInfo) error {
	current, err := os.Lstat(address)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Unix agent IPC endpoint during cleanup: %w", err)
	}
	if !os.SameFile(created, current) {
		return nil
	}
	if err := os.Remove(address); err != nil {
		return fmt.Errorf("remove Unix agent IPC endpoint: %w", err)
	}
	return nil
}
