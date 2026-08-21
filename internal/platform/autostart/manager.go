// Package autostart manages the current user's Sluice start-at-login entry.
package autostart

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrInvalidExecutable  = errors.New("invalid autostart executable")
	ErrUnsafeRegistration = errors.New("unsafe autostart registration")
)

type State string

const (
	StateDisabled    State = "disabled"
	StateEnabled     State = "enabled"
	StateNeedsRepair State = "needs-repair"
)

type Manager interface {
	Enable(ctx context.Context) error
	Disable(ctx context.Context) error
	Status(ctx context.Context) (State, error)
}

func validateExecutable(path string) (string, error) {
	if path == "" || strings.ContainsRune(path, 0) || !filepath.IsAbs(path) {
		return "", fmt.Errorf("%w: path must be absolute", ErrInvalidExecutable)
	}

	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidExecutable, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidExecutable, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: path is not a regular file", ErrInvalidExecutable)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("%w: file is not executable", ErrInvalidExecutable)
	}
	return canonical, nil
}
