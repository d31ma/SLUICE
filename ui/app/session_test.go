package appui_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	appui "github.com/d31ma/sluice/ui/app"
)

func TestSessionStartsAgentBeforeShowingShell(t *testing.T) {
	sequence := make([]string, 0, 2)
	agent := &recordingAgent{onStart: func() { sequence = append(sequence, "agent") }}
	shell := &recordingShell{onShow: func() { sequence = append(sequence, "shell") }}
	session := appui.NewSession(agent, shell)

	if err := session.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if len(sequence) != 2 || sequence[0] != "agent" || sequence[1] != "shell" {
		t.Fatalf("startup sequence = %v, want [agent shell]", sequence)
	}
}

func TestSessionDoesNotShowShellWhenAgentFails(t *testing.T) {
	agent := &recordingAgent{startErr: errors.New("agent unavailable")}
	shell := &recordingShell{}
	session := appui.NewSession(agent, shell)

	if err := session.Start(context.Background()); err == nil {
		t.Fatal("Start() error = nil, want agent failure")
	}
	if shell.shows != 0 {
		t.Fatalf("shell shows = %d, want 0", shell.shows)
	}
}

func TestSessionQuitIsIdempotentAndClosesBeforeShellQuit(t *testing.T) {
	sequence := make([]string, 0, 2)
	agent := &recordingAgent{onClose: func() { sequence = append(sequence, "close") }}
	shell := &recordingShell{onQuit: func() { sequence = append(sequence, "quit") }}
	session := appui.NewSession(agent, shell)

	if err := session.Quit(context.Background()); err != nil {
		t.Fatalf("first Quit() error = %v", err)
	}
	if err := session.Quit(context.Background()); err != nil {
		t.Fatalf("second Quit() error = %v", err)
	}
	if agent.closes != 1 || shell.quits != 1 {
		t.Fatalf("close/quit calls = %d/%d, want 1/1", agent.closes, shell.quits)
	}
	if len(sequence) != 2 || sequence[0] != "close" || sequence[1] != "quit" {
		t.Fatalf("shutdown sequence = %v, want [close quit]", sequence)
	}
}

type recordingAgent struct {
	startErr error
	closeErr error
	onStart  func()
	onClose  func()
	closes   int
	mu       sync.Mutex
}

func (agent *recordingAgent) Start(context.Context) error {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if agent.onStart != nil {
		agent.onStart()
	}
	return agent.startErr
}

func (agent *recordingAgent) Close(context.Context) error {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	agent.closes++
	if agent.onClose != nil {
		agent.onClose()
	}
	return agent.closeErr
}

type recordingShell struct {
	onShow func()
	onQuit func()
	shows  int
	quits  int
}

func (shell *recordingShell) Show() {
	shell.shows++
	if shell.onShow != nil {
		shell.onShow()
	}
}

func (*recordingShell) Hide() {}

func (shell *recordingShell) Quit() {
	shell.quits++
	if shell.onQuit != nil {
		shell.onQuit()
	}
}

func (*recordingShell) Notify(string, string) {}
