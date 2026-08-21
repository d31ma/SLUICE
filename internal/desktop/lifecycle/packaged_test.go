package lifecycle_test

import (
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
