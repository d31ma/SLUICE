package runtimeexec

import (
	"os"
	"os/exec"
)

// ConfigureInterruptibleProcess prepares a command for a targeted, graceful
// interrupt on the current operating system. Call it before Cmd.Start.
func ConfigureInterruptibleProcess(command *exec.Cmd) {
	configureInterruptibleProcess(command)
}

// InterruptProcess requests a graceful interrupt from a configured process.
func InterruptProcess(process *os.Process) error {
	return interruptProcess(process)
}

// IsExpectedInterruptExit reports whether err is the platform's normal wait
// result when a configured process accepts InterruptProcess.
func IsExpectedInterruptExit(err error) bool {
	return isExpectedInterruptExit(err)
}
