//go:build windows

package lifecycle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/d31ma/sluice/internal/desktop/lifecycle"
)

func TestDefaultWindowsStateRootUsesLocalApplicationData(t *testing.T) {
	localData, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir() error = %v", err)
	}
	root, err := lifecycle.DefaultWindowsStateRoot()
	if err != nil {
		t.Fatalf("DefaultWindowsStateRoot() error = %v", err)
	}
	want := filepath.Join(localData, "Sluice")
	if root != want {
		t.Fatalf("state root = %q, want %q", root, want)
	}
}
