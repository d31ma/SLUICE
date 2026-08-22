package lifecycle

import (
	"errors"
	"fmt"
	"os"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

type generationResources struct {
	descriptor fileIdentity
	endpoint   fileIdentity
}

type fileIdentity struct {
	metadata os.FileInfo
	path     string
}

func captureGenerationResources(sessionFile string, descriptor ipc.Descriptor) generationResources {
	return generationResources{
		descriptor: captureRegularFile(sessionFile),
		endpoint:   captureNativeEndpoint(descriptor),
	}
}

func captureRegularFile(path string) fileIdentity {
	metadata, err := os.Lstat(path)
	if err != nil || !metadata.Mode().IsRegular() || metadata.Mode()&os.ModeSymlink != 0 {
		return fileIdentity{}
	}
	return fileIdentity{path: path, metadata: metadata}
}

func (resources generationResources) cleanup() error {
	return errors.Join(
		resources.endpoint.remove(os.ModeSocket, "native agent IPC endpoint"),
		resources.descriptor.remove(0, "agent session descriptor"),
	)
}

func (identity fileIdentity) remove(requiredType os.FileMode, label string) error {
	if identity.metadata == nil || identity.path == "" {
		return nil
	}
	current, matches, err := identity.currentFile(label)
	if err != nil {
		return err
	}
	if !matches || !matchesRequiredType(current.Mode(), requiredType) {
		return nil
	}
	if err := os.Remove(identity.path); err != nil {
		return fmt.Errorf("remove stale %s: %w", label, err)
	}
	return nil
}

func (identity fileIdentity) currentFile(label string) (os.FileInfo, bool, error) {
	current, err := os.Lstat(identity.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect stale %s: %w", label, err)
	}
	return current, os.SameFile(identity.metadata, current), nil
}

func matchesRequiredType(mode, required os.FileMode) bool {
	if required != 0 {
		return mode&required != 0
	}
	if !mode.IsRegular() {
		return false
	}
	return true
}
