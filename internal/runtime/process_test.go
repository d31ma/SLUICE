package runtimeexec_test

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"testing"
	"time"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
)

const interruptChildReady = "SLUICE_RUNTIME_INTERRUPT_CHILD_READY"

func TestInterruptProcessStopsConfiguredChildCleanly(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "ready")
	command := exec.Command(os.Args[0], "-test.run=^TestInterruptProcessChild$") //nolint:gosec // Exact current test binary; no shell.
	command.Env = append(os.Environ(), interruptChildReady+"="+ready)
	runtimeexec.ConfigureInterruptibleProcess(command)
	if err := command.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	completed := false
	t.Cleanup(func() {
		if completed {
			return
		}
		_ = command.Process.Kill()
		<-done
	})
	waitForInterruptChild(t, ready)

	if err := runtimeexec.InterruptProcess(command.Process); err != nil {
		t.Fatalf("InterruptProcess() error = %v", err)
	}
	select {
	case err := <-done:
		completed = true
		if err != nil && !runtimeexec.IsExpectedInterruptExit(err) {
			t.Fatalf("Wait() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("configured child did not stop after interrupt")
	}
}

func TestInterruptProcessChild(t *testing.T) {
	ready := os.Getenv(interruptChildReady)
	if ready == "" {
		return
	}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)
	if err := os.WriteFile(ready, []byte("ready"), 0o600); err != nil { //nolint:gosec // Parent supplies a test-owned temporary path to this exact child.
		t.Fatalf("WriteFile(ready) error = %v", err)
	}
	select {
	case <-interrupt:
	case <-time.After(10 * time.Second):
		t.Fatal("child did not receive interrupt")
	}
}

func waitForInterruptChild(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for interrupt child")
}
