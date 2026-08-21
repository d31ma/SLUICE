//go:build windows

package autostart

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	windowsRunKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	windowsValueName = "Sluice"
)

type windowsManager struct {
	command string
}

func New(executable string) (Manager, error) {
	canonical, err := validateExecutable(executable)
	if err != nil {
		return nil, err
	}
	return &windowsManager{command: windowsRunCommand(canonical)}, nil
}

func (manager *windowsManager) Enable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		windowsRunKey,
		registry.QUERY_VALUE|registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("open per-user Run registry key: %w", err)
	}
	setErr := key.SetStringValue(windowsValueName, manager.command)
	closeErr := key.Close()
	if err := errors.Join(setErr, closeErr); err != nil {
		return fmt.Errorf("enable autostart: %w", err)
	}
	return nil
}

func (manager *windowsManager) Disable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKey, registry.SET_VALUE)
	if err == registry.ErrNotExist {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open per-user Run registry key: %w", err)
	}
	deleteErr := key.DeleteValue(windowsValueName)
	if deleteErr == registry.ErrNotExist {
		deleteErr = nil
	}
	closeErr := key.Close()
	if err := errors.Join(deleteErr, closeErr); err != nil {
		return fmt.Errorf("disable autostart: %w", err)
	}
	return nil
}

func (manager *windowsManager) Status(ctx context.Context) (State, error) {
	if err := ctx.Err(); err != nil {
		return StateDisabled, err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, windowsRunKey, registry.QUERY_VALUE)
	if err == registry.ErrNotExist {
		return StateDisabled, nil
	}
	if err != nil {
		return StateDisabled, fmt.Errorf("open per-user Run registry key: %w", err)
	}
	command, _, valueErr := key.GetStringValue(windowsValueName)
	if valueErr == registry.ErrNotExist {
		valueErr = nil
		command = ""
	}
	closeErr := key.Close()
	if err := errors.Join(valueErr, closeErr); err != nil {
		return StateDisabled, fmt.Errorf("read autostart registry value: %w", err)
	}
	if command == "" {
		return StateDisabled, nil
	}
	if command != manager.command {
		return StateNeedsRepair, nil
	}
	return StateEnabled, nil
}
