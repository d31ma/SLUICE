//go:build !android && !ios

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"github.com/d31ma/sluice/internal/platform/autostart"
	appui "github.com/d31ma/sluice/ui/app"
)

const nativeSmokeDuration = 1500 * time.Millisecond

type desktopOptions struct {
	background  bool
	nativeSmoke bool
}

type (
	managerFactory func(executable string) (autostart.Manager, error)
	desktopRunner  func(ctx context.Context, options desktopOptions) error
)

type desktopShell interface {
	Show()
	Hide()
	Notify(title, content string)
	RequestQuit()
	Run()
}

func main() {
	executable, err := os.Executable()
	if err != nil {
		writeError(os.Stderr, fmt.Errorf("resolve executable: %w", err))
		os.Exit(1)
	}
	os.Exit(run(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		executable,
		autostart.New,
		runDesktop,
	))
}

func run(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	executable string,
	newManager managerFactory,
	startDesktop desktopRunner,
) int {
	if len(args) > 0 && args[0] == "autostart" {
		return runAutostart(ctx, args[1:], stdout, stderr, executable, newManager)
	}

	flags := flag.NewFlagSet("sluice-desktop", flag.ContinueOnError)
	flags.SetOutput(stderr)
	background := flags.Bool("background", false, "start in the system tray without opening the window")
	nativeSmoke := flags.Bool("native-smoke", false, "exercise the native window, tray, notification, and shutdown lifecycle")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		if err := writeLine(stderr, "sluice-desktop: unexpected positional arguments"); err != nil {
			return 1
		}
		return 2
	}
	if err := startDesktop(ctx, desktopOptions{background: *background, nativeSmoke: *nativeSmoke}); err != nil {
		writeError(stderr, err)
		return 1
	}
	return 0
}

func runAutostart(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	executable string,
	newManager managerFactory,
) int {
	if len(args) != 1 || (args[0] != "enable" && args[0] != "disable" && args[0] != "status") {
		if err := writeLine(stderr, "usage: sluice-desktop autostart enable|disable|status"); err != nil {
			return 1
		}
		return 2
	}
	manager, err := newManager(executable)
	if err != nil {
		writeError(stderr, err)
		return 1
	}

	var state autostart.State
	switch args[0] {
	case "enable":
		err = manager.Enable(ctx)
		state = autostart.StateEnabled
	case "disable":
		err = manager.Disable(ctx)
		state = autostart.StateDisabled
	case "status":
		state, err = manager.Status(ctx)
	}
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	if err := writeLine(stdout, state); err != nil {
		writeError(stderr, fmt.Errorf("write result: %w", err))
		return 1
	}
	return 0
}

func writeLine(writer io.Writer, value any) error {
	_, err := fmt.Fprintln(writer, value)
	return err
}

func writeError(writer io.Writer, err error) {
	_ = writeLine(writer, fmt.Sprintf("sluice-desktop: %v", err))
}

func runDesktop(ctx context.Context, options desktopOptions) error {
	fyneapp.SetMetadata(fyne.AppMetadata{
		ID:         applicationID(),
		Name:       "Sluice",
		Version:    "0.0.0-dev",
		Build:      1,
		Migrations: map[string]bool{"fyneDo": true},
	})
	application := fyneapp.NewWithID(applicationID())
	shell := appui.NewDesktopShell(application)
	showDesktopShell(shell, options)
	runDesktopShell(ctx, shell, options.nativeSmoke)
	return nil
}

func showDesktopShell(shell desktopShell, options desktopOptions) {
	if !options.background || options.nativeSmoke {
		shell.Show()
	}
}

func runDesktopShell(ctx context.Context, shell desktopShell, nativeSmoke bool) {
	ctx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	finished := make(chan struct{})
	defer close(finished)
	go quitOnCancellation(ctx, finished, shell)
	defer scheduleNativeSmoke(nativeSmoke, shell)()

	shell.Run()
}

func scheduleNativeSmoke(enabled bool, shell desktopShell) func() {
	if !enabled {
		return func() {}
	}
	timer := time.AfterFunc(nativeSmokeDuration, func() {
		fyne.Do(func() { exerciseNativeShell(shell) })
	})
	return func() { timer.Stop() }
}

func exerciseNativeShell(shell desktopShell) {
	shell.Hide()
	shell.Show()
	shell.Notify("Sluice native check", "Window, tray, notification, and shutdown lifecycle reached.")
	shell.RequestQuit()
}

func quitOnCancellation(ctx context.Context, finished <-chan struct{}, shell desktopShell) {
	select {
	case <-ctx.Done():
		fyne.Do(shell.RequestQuit)
	case <-finished:
	}
}

func applicationID() string {
	return "ma.del.sluice.desktop"
}
