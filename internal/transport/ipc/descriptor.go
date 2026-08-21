package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const maxDescriptorBytes = 4096

type descriptorPublication struct {
	destination string
	temporary   string
}

// WriteDescriptor atomically publishes a private agent session descriptor.
func WriteDescriptor(path string, descriptor Descriptor) error {
	encoded, err := encodeDescriptor(descriptor)
	if err != nil {
		return err
	}
	publication, err := prepareDescriptorPublication(path, encoded)
	if err != nil {
		return err
	}
	defer publication.cleanup()
	return publication.publish()
}

func encodeDescriptor(descriptor Descriptor) ([]byte, error) {
	if err := validateDescriptor(descriptor); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		return nil, fmt.Errorf("encode agent session descriptor: %w", err)
	}
	return encoded, nil
}

func prepareDescriptorPublication(path string, encoded []byte) (descriptorPublication, error) {
	directory, destination, err := secureDescriptorDirectory(path)
	if err != nil {
		return descriptorPublication{}, err
	}
	temporary, err := writeTemporaryDescriptor(directory, encoded)
	if err != nil {
		return descriptorPublication{}, err
	}
	return descriptorPublication{destination: destination, temporary: temporary}, nil
}

func secureDescriptorDirectory(path string) (string, string, error) {
	destination, err := descriptorPath(path)
	if err != nil {
		return "", "", err
	}
	directory := filepath.Dir(destination)
	if err := prepareDescriptorDirectory(directory); err != nil {
		return "", "", err
	}
	return directory, destination, nil
}

func descriptorPath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("agent session descriptor path must be absolute")
	}
	return filepath.Clean(path), nil
}

func prepareDescriptorDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create agent session directory: %w", err)
	}
	return inspectDescriptorDirectory(directory)
}

func inspectDescriptorDirectory(directory string) error {
	directoryInfo, err := os.Lstat(directory)
	if err != nil {
		return fmt.Errorf("agent session directory must be a real directory")
	}
	return validateDescriptorDirectory(directoryInfo)
}

func validateDescriptorDirectory(directoryInfo os.FileInfo) error {
	if !isRealDirectory(directoryInfo) {
		return fmt.Errorf("agent session directory must be a real directory")
	}
	if !isPrivateDirectory(directoryInfo) {
		return fmt.Errorf("agent session directory must not be writable by other users")
	}
	return nil
}

func isRealDirectory(info os.FileInfo) bool {
	return info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func isPrivateDirectory(info os.FileInfo) bool {
	return runtime.GOOS == "windows" || info.Mode().Perm()&0o022 == 0
}

func writeTemporaryDescriptor(directory string, encoded []byte) (string, error) {
	temporary, err := os.CreateTemp(directory, ".sluice-agent-session-*")
	if err != nil {
		return "", fmt.Errorf("create agent session descriptor: %w", err)
	}
	if err := finalizeTemporaryDescriptor(temporary, encoded); err != nil {
		_ = os.Remove(temporary.Name())
		return "", err
	}
	return temporary.Name(), nil
}

func finalizeTemporaryDescriptor(temporary *os.File, encoded []byte) error {
	if err := protectTemporaryDescriptor(temporary); err != nil {
		return err
	}
	return persistTemporaryDescriptor(temporary, encoded)
}

func persistTemporaryDescriptor(temporary *os.File, encoded []byte) error {
	if err := writeAndSyncDescriptor(temporary, encoded); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close agent session descriptor: %w", err)
	}
	return nil
}

func protectTemporaryDescriptor(temporary *os.File) error {
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect agent session descriptor: %w", err)
	}
	return nil
}

func writeAndSyncDescriptor(temporary *os.File, encoded []byte) error {
	if _, err := temporary.Write(encoded); err != nil {
		return fmt.Errorf("write agent session descriptor: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync agent session descriptor: %w", err)
	}
	return nil
}

func (publication descriptorPublication) cleanup() {
	_ = os.Remove(publication.temporary)
}

func (publication descriptorPublication) publish() error {
	if runtime.GOOS == "windows" {
		_ = os.Remove(publication.destination)
	}
	if err := os.Rename(publication.temporary, publication.destination); err != nil {
		return fmt.Errorf("publish agent session descriptor: %w", err)
	}
	return nil
}

// ReadDescriptor reads and validates a live agent's private descriptor.
func ReadDescriptor(path string) (Descriptor, error) {
	file, err := openDescriptor(path)
	if err != nil {
		return Descriptor{}, err
	}
	defer func() { _ = file.Close() }()
	return readValidatedDescriptor(file)
}

func openDescriptor(path string) (*os.File, error) {
	clean, err := readableDescriptorPath(path)
	if err != nil {
		return nil, err
	}
	pathInfo, err := descriptorPathInfo(clean)
	if err != nil {
		return nil, err
	}
	return openVerifiedDescriptor(clean, pathInfo)
}

func readableDescriptorPath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: invalid session descriptor path", ErrUnavailable)
	}
	return filepath.Clean(path), nil
}

func descriptorPathInfo(path string) (os.FileInfo, error) {
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: session descriptor cannot be opened", ErrUnavailable)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: session descriptor cannot be opened", ErrUnavailable)
	}
	return pathInfo, nil
}

func openVerifiedDescriptor(path string, pathInfo os.FileInfo) (*os.File, error) {
	// #nosec G304 -- path is absolute, cleaned, lstat'd, and rejected when it
	// names a symlink before this open; the post-open identity check closes the
	// remaining replacement race.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: session descriptor cannot be opened", ErrUnavailable)
	}
	if err := verifyOpenedDescriptor(file, pathInfo); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func verifyOpenedDescriptor(file *os.File, pathInfo os.FileInfo) error {
	metadata, err := file.Stat()
	if err != nil {
		return fmt.Errorf("%w: session descriptor is not a regular file", ErrUnavailable)
	}
	return validateDescriptorMetadata(pathInfo, metadata)
}

func validateDescriptorMetadata(pathInfo, metadata os.FileInfo) error {
	if !isUnchangedRegularFile(pathInfo, metadata) {
		return fmt.Errorf("%w: session descriptor is not a regular file", ErrUnavailable)
	}
	if !isPrivateDescriptorFile(metadata) {
		return fmt.Errorf("%w: session descriptor permissions are too broad", ErrUnavailable)
	}
	return nil
}

func isUnchangedRegularFile(pathInfo, metadata os.FileInfo) bool {
	return metadata.Mode().IsRegular() && os.SameFile(pathInfo, metadata)
}

func isPrivateDescriptorFile(metadata os.FileInfo) bool {
	return runtime.GOOS == "windows" || metadata.Mode().Perm()&0o077 == 0
}

func readValidatedDescriptor(file *os.File) (Descriptor, error) {
	limited := io.LimitReader(file, maxDescriptorBytes+1)
	encoded, err := io.ReadAll(limited)
	if err != nil || len(encoded) > maxDescriptorBytes {
		return Descriptor{}, fmt.Errorf("%w: invalid session descriptor", ErrUnavailable)
	}
	return decodeValidatedDescriptor(encoded)
}

func decodeValidatedDescriptor(encoded []byte) (Descriptor, error) {
	descriptor, err := decodeDescriptor(encoded)
	if err != nil {
		return Descriptor{}, err
	}
	if err := validateDescriptor(descriptor); err != nil {
		return Descriptor{}, err
	}
	return descriptor, nil
}

func decodeDescriptor(encoded []byte) (Descriptor, error) {
	var descriptor Descriptor
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("%w: invalid session descriptor", ErrUnavailable)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Descriptor{}, fmt.Errorf("%w: invalid session descriptor", ErrUnavailable)
	}
	return descriptor, nil
}
