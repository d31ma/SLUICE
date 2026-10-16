//go:build windows

package runtimeexec

import "path/filepath"

func canonicalExecutablePath(path string) (string, error) {
	// Windows package and toolchain roots commonly traverse junctions. The Go
	// runtime resolves those paths correctly when opening and executing them,
	// while filepath.EvalSymlinks can reject a valid junction-backed path.
	// Preserve the absolute path and let the subsequent os.Stat validate that
	// its target exists and is a regular file.
	return filepath.Clean(path), nil
}
