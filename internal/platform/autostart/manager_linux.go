//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

func New(executable string) (Manager, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve user home: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return newLinuxManager(executable, configHome)
}

func newLinuxManager(executable, configHome string) (Manager, error) {
	canonical, err := validateExecutable(executable)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(configHome) {
		return nil, fmt.Errorf("resolve XDG autostart directory: XDG_CONFIG_HOME must be absolute")
	}
	registration := filepath.Join(configHome, "autostart", applicationID+".desktop")
	return newFileManager(registration, linuxRegistration(canonical)), nil
}
