package lifecycle

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/d31ma/sluice/internal/transport/ipc"
)

type generationResources struct {
	descriptor fileIdentity
	endpoint   fileIdentity
}

type fileIdentity struct {
	metadata       os.FileInfo
	path           string
	fingerprint    [sha256.Size]byte
	hasFingerprint bool
}

const maximumIdentityBytes = 4096

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
	fingerprint, ok := fingerprintRegularFile(path, metadata)
	if !ok {
		return fileIdentity{}
	}
	return fileIdentity{
		path:           path,
		metadata:       metadata,
		fingerprint:    fingerprint,
		hasFingerprint: true,
	}
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
	if !os.SameFile(identity.metadata, current) {
		return current, false, nil
	}
	if !identity.hasFingerprint {
		return current, true, nil
	}
	fingerprint, ok := fingerprintRegularFile(identity.path, current)
	return current, ok && fingerprint == identity.fingerprint, nil
}

func fingerprintRegularFile(path string, expected os.FileInfo) ([sha256.Size]byte, bool) {
	var empty [sha256.Size]byte
	// #nosec G304 -- path is the captured private descriptor path and the
	// opened file is checked against the expected identity before use.
	file, err := os.Open(path)
	if err != nil {
		return empty, false
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(expected, opened) {
		return empty, false
	}
	contents, err := io.ReadAll(io.LimitReader(file, maximumIdentityBytes+1))
	if err != nil || len(contents) > maximumIdentityBytes {
		return empty, false
	}
	return sha256.Sum256(contents), true
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
