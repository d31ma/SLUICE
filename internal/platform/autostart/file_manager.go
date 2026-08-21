package autostart

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type fileManager struct {
	path     string
	contents []byte
}

func newFileManager(path string, contents []byte) Manager {
	return &fileManager{path: path, contents: bytes.Clone(contents)}
}

func (manager *fileManager) Enable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := manager.validateTarget(); err != nil {
		return err
	}
	if err := ensureRegistrationDirectory(filepath.Dir(manager.path)); err != nil {
		return err
	}
	if err := atomicWrite(manager.path, manager.contents); err != nil {
		return fmt.Errorf("enable autostart: %w", err)
	}
	return nil
}

func (manager *fileManager) Disable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := manager.validateTarget(); err != nil {
		return err
	}
	if err := os.Remove(manager.path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("disable autostart: %w", err)
	}
	if err := syncDirectory(filepath.Dir(manager.path)); err != nil {
		return fmt.Errorf("persist disabled autostart: %w", err)
	}
	return nil
}

func (manager *fileManager) Status(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return StateDisabled, err
	}
	if err := manager.validateTarget(); err != nil {
		return StateDisabled, err
	}
	contents, err := os.ReadFile(manager.path)
	if errors.Is(err, os.ErrNotExist) {
		return StateDisabled, nil
	}
	if err != nil {
		return StateDisabled, fmt.Errorf("read autostart registration: %w", err)
	}
	if !bytes.Equal(contents, manager.contents) {
		return StateNeedsRepair, nil
	}
	return StateEnabled, nil
}

func (manager *fileManager) validateTarget() error {
	info, err := os.Lstat(manager.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect autostart registration: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s must be a regular file", ErrUnsafeRegistration, manager.path)
	}
	return nil
}

func ensureRegistrationDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create autostart directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect autostart directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: %s must be a directory", ErrUnsafeRegistration, path)
	}
	return nil
}

func atomicWrite(path string, contents []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".sluice-autostart-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer discardTemporary(temporary, temporaryPath)

	if err := writeTemporary(temporary, contents); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func writeTemporary(temporary *os.File, contents []byte) error {
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	if _, err := temporary.Write(contents); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return nil
}

func discardTemporary(temporary *os.File, path string) {
	_ = temporary.Close()
	_ = os.Remove(path)
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(path) // #nosec G304 -- path is the fixed parent of Sluice's per-user registration.
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}
