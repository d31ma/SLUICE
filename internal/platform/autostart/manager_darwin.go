//go:build darwin

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

func New(executable string) (Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user home: %w", err)
	}
	return newDarwinManager(executable, home)
}

func newDarwinManager(executable, home string) (Manager, error) {
	canonical, err := validateExecutable(executable)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(home) {
		return nil, fmt.Errorf("resolve LaunchAgents directory: home path must be absolute")
	}
	registration := filepath.Join(home, "Library", "LaunchAgents", applicationID+".plist")
	return newFileManager(registration, darwinRegistration(canonical)), nil
}
