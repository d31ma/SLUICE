package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerationResourcesRemoveTheCapturedDescriptor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-session.json")
	if err := os.WriteFile(path, []byte("captured"), 0o600); err != nil {
		t.Fatalf("write descriptor: %v", err)
	}
	resources := generationResources{descriptor: captureRegularFile(path)}
	if err := resources.cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("captured descriptor remains: %v", err)
	}
}

func TestGenerationResourcesPreserveAReplacementDescriptor(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "agent-session.json")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old descriptor: %v", err)
	}
	resources := generationResources{descriptor: captureRegularFile(path)}
	replacement := filepath.Join(directory, "replacement")
	if err := os.WriteFile(replacement, []byte("new"), 0o600); err != nil {
		t.Fatalf("write replacement descriptor: %v", err)
	}
	if runtime.GOOS == "windows" {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove old descriptor before Windows publication: %v", err)
		}
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatalf("replace descriptor: %v", err)
	}
	if err := resources.cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	contents, err := os.ReadFile(path) // #nosec G304 -- test-owned path beneath t.TempDir.
	if err != nil || string(contents) != "new" {
		t.Fatalf("replacement descriptor = %q, %v", contents, err)
	}
}
