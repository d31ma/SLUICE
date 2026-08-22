//go:build !android && !ios

package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/d31ma/sluice/internal/platform/autostart"
)

func TestRunAutostartCommands(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		initial    autostart.State
		wantState  autostart.State
		wantOutput string
	}{
		{name: "enable", command: "enable", initial: autostart.StateDisabled, wantState: autostart.StateEnabled, wantOutput: "enabled\n"},
		{name: "disable", command: "disable", initial: autostart.StateEnabled, wantState: autostart.StateDisabled, wantOutput: "disabled\n"},
		{name: "status enabled", command: "status", initial: autostart.StateEnabled, wantState: autostart.StateEnabled, wantOutput: "enabled\n"},
		{name: "status repair", command: "status", initial: autostart.StateNeedsRepair, wantState: autostart.StateNeedsRepair, wantOutput: "needs-repair\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := &memoryManager{state: test.initial}
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := run(
				context.Background(),
				[]string{"autostart", test.command},
				&stdout,
				&stderr,
				"/Applications/Sluice.app/Contents/MacOS/sluice-desktop",
				func(string) (autostart.Manager, error) { return manager, nil },
				func(context.Context, desktopOptions) error { return errors.New("desktop must not run") },
			)
			if exitCode != 0 {
				t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
			}
			if stdout.String() != test.wantOutput {
				t.Fatalf("stdout = %q, want %q", stdout.String(), test.wantOutput)
			}
			if manager.state != test.wantState {
				t.Fatalf("manager state = %q, want %q", manager.state, test.wantState)
			}
		})
	}
}

func TestRunDesktopOptions(t *testing.T) {
	var got desktopOptions
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"--background", "--native-smoke"},
		&stdout,
		&stderr,
		"/installed/sluice-desktop",
		func(string) (autostart.Manager, error) { return nil, errors.New("autostart must not initialize") },
		func(_ context.Context, options desktopOptions) error {
			got = options
			return nil
		},
	)
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !got.background || !got.nativeSmoke {
		t.Fatalf("desktop options = %+v, want background and native smoke", got)
	}
	if got.executable != "/installed/sluice-desktop" {
		t.Fatalf("desktop executable = %q", got.executable)
	}
}

func TestRunPackagedRuntimeSmokeOptions(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "qualification-state")
	var got desktopOptions
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"--packaged-runtime-smoke", "--qualification-state-root", stateRoot},
		&bytes.Buffer{},
		&stderr,
		"/installed/sluice-desktop",
		func(string) (autostart.Manager, error) { return nil, errors.New("unused") },
		func(_ context.Context, options desktopOptions) error { got = options; return nil },
	)
	if exitCode != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", exitCode, stderr.String())
	}
	if !got.packagedRuntimeSmoke || got.nativeSmoke || got.stateRoot != stateRoot {
		t.Fatalf("desktop options = %+v, want packaged runtime smoke with isolated state", got)
	}
}

func TestRunRejectsConflictingSmokeModes(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"--native-smoke", "--packaged-runtime-smoke"},
		&bytes.Buffer{},
		&stderr,
		"/installed/sluice-desktop",
		func(string) (autostart.Manager, error) { return nil, errors.New("unused") },
		func(context.Context, desktopOptions) error { return errors.New("desktop must not run") },
	)
	if exitCode != 2 || stderr.Len() == 0 {
		t.Fatalf("conflicting smoke modes = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestRunAcceptsAnIsolatedStateRootOnlyForQualificationSmoke(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "qualification-state")
	var got desktopOptions
	exitCode := run(
		context.Background(),
		[]string{"--native-smoke", "--qualification-state-root", stateRoot},
		&bytes.Buffer{},
		&bytes.Buffer{},
		"/installed/sluice-desktop",
		func(string) (autostart.Manager, error) { return nil, errors.New("unused") },
		func(_ context.Context, options desktopOptions) error { got = options; return nil },
	)
	if exitCode != 0 || got.stateRoot != stateRoot {
		t.Fatalf("run() = %d, options = %+v", exitCode, got)
	}

	var stderr bytes.Buffer
	exitCode = run(
		context.Background(),
		[]string{"--qualification-state-root", stateRoot},
		&bytes.Buffer{},
		&stderr,
		"/installed/sluice-desktop",
		func(string) (autostart.Manager, error) { return nil, errors.New("unused") },
		func(context.Context, desktopOptions) error { return errors.New("desktop must not run") },
	)
	if exitCode != 2 || stderr.Len() == 0 {
		t.Fatalf("unbounded state override = exit %d, stderr %q", exitCode, stderr.String())
	}
}

func TestAwaitPackagedRuntimeSmoke(t *testing.T) {
	if err := awaitPackagedRuntimeSmoke(context.Background(), 0); err != nil {
		t.Fatalf("awaitPackagedRuntimeSmoke() error = %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := awaitPackagedRuntimeSmoke(cancelled, nativeSmokeDuration); !errors.Is(err, context.Canceled) {
		t.Fatalf("awaitPackagedRuntimeSmoke() error = %v, want context cancellation", err)
	}
}

func TestUnpackagedRuntimeSmokeError(t *testing.T) {
	if err := unpackagedRuntimeSmokeError(false); err != nil {
		t.Fatalf("ordinary unpackaged launch error = %v", err)
	}
	if err := unpackagedRuntimeSmokeError(true); err == nil {
		t.Fatal("required packaged runtime smoke accepted an unpackaged launch")
	}
}

func TestRunDesktopWithLifecycleWrapsTheFyneShell(t *testing.T) {
	events := make([]string, 0, 3)
	session := &recordingRuntimeSession{events: &events}
	err := runDesktopWithLifecycle(
		context.Background(),
		desktopOptions{executable: "/Applications/Sluice.app/Contents/MacOS/sluice-desktop"},
		func(context.Context, desktopOptions) (runtimeSession, error) {
			events = append(events, "runtime:start")
			return session, nil
		},
		func() error {
			events = append(events, "fyne:run")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("runDesktopWithLifecycle() error = %v", err)
	}
	want := []string{"runtime:start", "fyne:run", "runtime:close"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestRunRejectsInvalidCommandWithoutStartingDesktop(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"autostart", "maybe"},
		&bytes.Buffer{},
		&stderr,
		"/installed/sluice-desktop",
		func(string) (autostart.Manager, error) { return &memoryManager{}, nil },
		func(context.Context, desktopOptions) error { return errors.New("desktop must not run") },
	)
	if exitCode != 2 {
		t.Fatalf("run() exit code = %d, want 2", exitCode)
	}
	if stderr.String() == "" {
		t.Fatal("stderr is empty")
	}
}

func TestRunReportsAutostartFailure(t *testing.T) {
	var stderr bytes.Buffer
	exitCode := run(
		context.Background(),
		[]string{"autostart", "enable"},
		&bytes.Buffer{},
		&stderr,
		"/installed/sluice-desktop",
		func(string) (autostart.Manager, error) { return nil, errors.New("registry unavailable") },
		func(context.Context, desktopOptions) error { return nil },
	)
	if exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
	if stderr.String() != "sluice-desktop: registry unavailable\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestShowDesktopShellHonorsStartupMode(t *testing.T) {
	tests := []struct {
		name    string
		options desktopOptions
		want    int
	}{
		{name: "foreground", options: desktopOptions{}, want: 1},
		{name: "background", options: desktopOptions{background: true}, want: 0},
		{name: "native smoke overrides background", options: desktopOptions{background: true, nativeSmoke: true}, want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			shell := &recordingDesktopShell{}
			showDesktopShell(shell, test.options)
			if shell.showCalls != test.want {
				t.Fatalf("Show() calls = %d, want %d", shell.showCalls, test.want)
			}
		})
	}
}

func TestExerciseNativeShellRunsCompleteLifecycle(t *testing.T) {
	shell := &recordingDesktopShell{}
	exerciseNativeShell(shell)

	want := []string{"hide", "show", "notify:Sluice native check:Window, tray, notification, and shutdown lifecycle reached.", "quit"}
	if len(shell.events) != len(want) {
		t.Fatalf("events = %v, want %v", shell.events, want)
	}
	for index := range want {
		if shell.events[index] != want[index] {
			t.Fatalf("events = %v, want %v", shell.events, want)
		}
	}
}

func TestNativeSmokeLogFailureRejectsWindowInitializationErrors(t *testing.T) {
	if err := nativeSmokeLogFailure("Fyne error:  window creation error\n  Cause: APIUnavailable: WGL"); err == nil {
		t.Fatal("native smoke accepted a logged window creation failure")
	}
	if err := nativeSmokeLogFailure("Fyne error:  Failed to send notification\n"); err != nil {
		t.Fatalf("notification permission outcome rejected native window lifecycle: %v", err)
	}
	if err := nativeSmokeLogFailure(""); err != nil {
		t.Fatalf("clean native smoke log rejected: %v", err)
	}
}

type memoryManager struct {
	state autostart.State
}

type recordingDesktopShell struct {
	events    []string
	showCalls int
}

type recordingRuntimeSession struct {
	events *[]string
}

func (session *recordingRuntimeSession) Close(context.Context) error {
	*session.events = append(*session.events, "runtime:close")
	return nil
}

func (shell *recordingDesktopShell) Show() {
	shell.showCalls++
	shell.events = append(shell.events, "show")
}

func (shell *recordingDesktopShell) Hide() {
	shell.events = append(shell.events, "hide")
}

func (shell *recordingDesktopShell) Notify(title, content string) {
	shell.events = append(shell.events, "notify:"+title+":"+content)
}

func (shell *recordingDesktopShell) RequestQuit() {
	shell.events = append(shell.events, "quit")
}

func (*recordingDesktopShell) Run() {}

func (manager *memoryManager) Enable(context.Context) error {
	manager.state = autostart.StateEnabled
	return nil
}

func (manager *memoryManager) Disable(context.Context) error {
	manager.state = autostart.StateDisabled
	return nil
}

func (manager *memoryManager) Status(context.Context) (autostart.State, error) {
	return manager.state, nil
}
