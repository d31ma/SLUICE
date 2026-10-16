//go:build windows

package runtimeexec

import (
	"errors"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureInterruptibleProcess(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}

func interruptProcess(process *os.Process) error {
	return windows.GenerateConsoleCtrlEvent(
		windows.CTRL_BREAK_EVENT,
		uint32(process.Pid), //nolint:gosec // Windows process identifiers are unsigned 32-bit values.
	)
}

func isExpectedInterruptExit(err error) bool {
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		return false
	}
	wait, ok := exitError.Sys().(syscall.WaitStatus)
	return ok && wait.ExitCode == uint32(windows.STATUS_CONTROL_C_EXIT)
}
