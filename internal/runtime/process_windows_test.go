//go:build windows

package runtimeexec_test

import (
	"os"
	"os/exec"
	"testing"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
	"golang.org/x/sys/windows"
)

const expectedInterruptExitChild = "SLUICE_RUNTIME_EXPECTED_INTERRUPT_EXIT_CHILD"

func TestExpectedWindowsInterruptExit(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestExpectedWindowsInterruptExitChild$") //nolint:gosec // Exact current test binary; no shell.
	command.Env = append(os.Environ(), expectedInterruptExitChild+"=1")
	err := command.Run()
	if !runtimeexec.IsExpectedInterruptExit(err) {
		t.Fatalf("IsExpectedInterruptExit(%v) = false, want true", err)
	}
}

func TestExpectedWindowsInterruptExitChild(t *testing.T) {
	if os.Getenv(expectedInterruptExitChild) == "" {
		return
	}
	if err := windows.TerminateProcess(
		windows.CurrentProcess(),
		uint32(windows.STATUS_CONTROL_C_EXIT),
	); err != nil {
		t.Fatalf("TerminateProcess() error = %v", err)
	}
	select {}
}
