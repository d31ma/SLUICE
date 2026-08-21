//go:build darwin

package autostart

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDarwinManagerUsesPerUserLaunchAgent(t *testing.T) {
	home := t.TempDir()
	executable := filepath.Join(home, "Applications", "Sluice.app", "Contents", "MacOS", "sluice-desktop")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(executable, []byte("binary"), 0o700); err != nil { // #nosec G306 -- the fixture must be executable.
		t.Fatalf("WriteFile() error = %v", err)
	}

	manager, err := newDarwinManager(executable, home)
	if err != nil {
		t.Fatalf("newDarwinManager() error = %v", err)
	}
	if enableErr := manager.Enable(context.Background()); enableErr != nil {
		t.Fatalf("Enable() error = %v", enableErr)
	}

	registration := filepath.Join(home, "Library", "LaunchAgents", applicationID+".plist")
	contents, err := os.ReadFile(registration) // #nosec G304 -- the path is inside the test's temporary home.
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", registration, err)
	}
	canonicalExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if string(contents) != string(darwinRegistration(canonicalExecutable)) {
		t.Fatal("LaunchAgent does not match the canonical registration")
	}
}
