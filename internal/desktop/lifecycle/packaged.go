package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
)

// NewPackaged resolves the native package layout for the running operating
// system. Unsupported targets and ordinary development binaries return
// runtimeexec.ErrNotPackaged.
func NewPackaged(executable, stateRoot, runtimeVersion string) (*Coordinator, error) {
	switch runtime.GOOS {
	case "darwin":
		return NewMacOSPackaged(executable, stateRoot, runtimeVersion)
	case "linux":
		return NewLinuxPackaged(executable, stateRoot, runtimeVersion)
	default:
		return nil, runtimeexec.ErrNotPackaged
	}
}

// NewMacOSPackaged resolves one relocatable Sluice.app and its per-user state.
// An empty stateRoot selects the platform application-data directory.
func NewMacOSPackaged(executable, stateRoot, runtimeVersion string) (*Coordinator, error) {
	bundle, err := runtimeexec.OpenMacOSBundle(executable)
	if err != nil {
		return nil, err
	}
	stateRoot, err = resolveStateRoot(stateRoot)
	if err != nil {
		return nil, err
	}
	return New(Config{
		Bundle:         bundle,
		StateRoot:      stateRoot,
		RuntimeVersion: runtimeVersion,
	})
}

// NewLinuxPackaged resolves one relocatable Sluice.AppDir and its per-user
// state. An empty stateRoot selects the XDG state directory.
func NewLinuxPackaged(executable, stateRoot, runtimeVersion string) (*Coordinator, error) {
	bundle, err := runtimeexec.OpenLinuxBundle(executable)
	if err != nil {
		return nil, err
	}
	stateRoot, err = resolveLinuxStateRoot(stateRoot)
	if err != nil {
		return nil, err
	}
	return New(Config{
		Bundle:         bundle,
		StateRoot:      stateRoot,
		RuntimeVersion: runtimeVersion,
	})
}

func resolveStateRoot(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	return DefaultMacOSStateRoot()
}

func resolveLinuxStateRoot(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	return DefaultLinuxStateRoot()
}

// DefaultMacOSStateRoot returns ~/Library/Application Support/Sluice through
// the operating system rather than constructing a home-relative path.
func DefaultMacOSStateRoot() (string, error) {
	configuration, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve per-user application data: %w", err)
	}
	if !filepath.IsAbs(configuration) {
		return "", fmt.Errorf("per-user application data path is not absolute")
	}
	return filepath.Join(configuration, "Sluice"), nil
}

// DefaultLinuxStateRoot returns the XDG state location without falling back to
// the configuration directory, which has different backup and lifecycle
// semantics.
func DefaultLinuxStateRoot() (string, error) {
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home for state data: %w", err)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(stateHome) {
		return "", fmt.Errorf("XDG state data path is not absolute")
	}
	return filepath.Join(stateHome, "sluice"), nil
}
