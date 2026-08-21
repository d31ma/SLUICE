package autostart

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileManagerLifecycle(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "sluice-registration")
	manager := newFileManager(path, []byte("expected\n"))

	assertState(t, manager, StateDisabled)
	if err := manager.Enable(context.Background()); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	assertState(t, manager, StateEnabled)

	contents, err := os.ReadFile(path) // #nosec G304 -- the path is inside the test's temporary directory.
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(contents) != "expected\n" {
		t.Fatalf("registration = %q, want expected contents", contents)
	}
	if runtime.GOOS != "windows" {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("Stat() error = %v", statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("registration mode = %o, want 600", info.Mode().Perm())
		}
	}

	if err := os.WriteFile(path, []byte("old executable\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	assertState(t, manager, StateNeedsRepair)
	if err := manager.Enable(context.Background()); err != nil {
		t.Fatalf("repair Enable() error = %v", err)
	}
	assertState(t, manager, StateEnabled)

	if err := manager.Disable(context.Background()); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	assertState(t, manager, StateDisabled)
	if err := manager.Disable(context.Background()); err != nil {
		t.Fatalf("idempotent Disable() error = %v", err)
	}
}

func TestFileManagerHonorsCanceledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registration")
	manager := newFileManager(path, []byte("expected"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := manager.Enable(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Enable() error = %v, want context canceled", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("registration exists after canceled enable: %v", err)
	}
}

func TestFileManagerDisableIsIdempotentWhenDirectoryDoesNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "nested", "registration")
	manager := newFileManager(path, []byte("expected"))

	if err := manager.Disable(context.Background()); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
}

func TestFileManagerRejectsRegistrationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows hosts")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target")
	path := filepath.Join(root, "registration")
	if err := os.WriteFile(target, []byte("do not replace"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	manager := newFileManager(path, []byte("expected"))

	if _, err := manager.Status(context.Background()); !errors.Is(err, ErrUnsafeRegistration) {
		t.Fatalf("Status() error = %v, want ErrUnsafeRegistration", err)
	}
	if err := manager.Enable(context.Background()); !errors.Is(err, ErrUnsafeRegistration) {
		t.Fatalf("Enable() error = %v, want ErrUnsafeRegistration", err)
	}
	if err := manager.Disable(context.Background()); !errors.Is(err, ErrUnsafeRegistration) {
		t.Fatalf("Disable() error = %v, want ErrUnsafeRegistration", err)
	}
}

func TestAtomicWriteCleansUpAfterRenameFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "registration")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	if err := atomicWrite(target, []byte("expected")); err == nil {
		t.Fatal("atomicWrite() error = nil, want rename failure")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "registration" || !entries[0].IsDir() {
		t.Fatalf("directory entries = %v, want only original registration directory", entries)
	}
}

func TestValidateExecutable(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "sluice-desktop")
	if err := os.WriteFile(executable, []byte("binary"), 0o700); err != nil { // #nosec G306 -- the fixture must be executable.
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := validateExecutable(executable)
	if err != nil {
		t.Fatalf("validateExecutable() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if got != want {
		t.Fatalf("validateExecutable() = %q, want %q", got, want)
	}

	for _, invalid := range []string{"sluice-desktop", executable + "\x00suffix", filepath.Dir(executable)} {
		if _, validateErr := validateExecutable(invalid); !errors.Is(validateErr, ErrInvalidExecutable) {
			t.Errorf("validateExecutable(%q) error = %v, want ErrInvalidExecutable", invalid, validateErr)
		}
	}
}

func assertState(t *testing.T, manager Manager, want State) {
	t.Helper()
	got, err := manager.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if got != want {
		t.Fatalf("Status() = %q, want %q", got, want)
	}
}
