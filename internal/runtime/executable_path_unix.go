//go:build !windows

package runtimeexec

import "path/filepath"

func canonicalExecutablePath(path string) (string, error) {
	return filepath.EvalSymlinks(filepath.Clean(path))
}
