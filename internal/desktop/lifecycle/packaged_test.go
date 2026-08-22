package lifecycle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/d31ma/sluice/internal/desktop/lifecycle"
)

func TestDefaultMacOSStateRootUsesPerUserApplicationData(t *testing.T) {
	root, err := lifecycle.DefaultMacOSStateRoot()
	if err != nil {
		t.Fatalf("DefaultMacOSStateRoot() error = %v", err)
	}
	if !filepath.IsAbs(root) || filepath.Base(root) != "Sluice" {
		t.Fatalf("state root = %q, want absolute Sluice directory", root)
	}
}

func TestDefaultLinuxStateRootUsesXDGStateHome(t *testing.T) {
	stateHome := filepath.Join(t.TempDir(), "state")
	t.Setenv("XDG_STATE_HOME", stateHome)
	root, err := lifecycle.DefaultLinuxStateRoot()
	if err != nil {
		t.Fatalf("DefaultLinuxStateRoot() error = %v", err)
	}
	if root != filepath.Join(stateHome, "sluice") {
		t.Fatalf("state root = %q", root)
	}
}

func TestDefaultLinuxStateRootRejectsRelativeXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "relative/state")
	if _, err := lifecycle.DefaultLinuxStateRoot(); err == nil {
		t.Fatal("DefaultLinuxStateRoot() accepted a relative XDG_STATE_HOME")
	}
}

func TestDefaultLinuxStateRootFallsBackToUserStateDirectory(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir() error = %v", err)
	}
	root, err := lifecycle.DefaultLinuxStateRoot()
	if err != nil {
		t.Fatalf("DefaultLinuxStateRoot() error = %v", err)
	}
	want := filepath.Join(home, ".local", "state", "sluice")
	if root != want {
		t.Fatalf("state root = %q, want %q", root, want)
	}
}
