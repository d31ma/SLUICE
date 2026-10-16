//go:build !windows

package runtimeexec

import (
	"os"
	"os/exec"
)

func configureInterruptibleProcess(_ *exec.Cmd) {}

func interruptProcess(process *os.Process) error {
	return process.Signal(os.Interrupt)
}

func isExpectedInterruptExit(error) bool {
	return false
}
