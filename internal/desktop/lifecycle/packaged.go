package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
)

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

func resolveStateRoot(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	return DefaultMacOSStateRoot()
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
