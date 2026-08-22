//go:build !android && !ios

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"github.com/d31ma/sluice/internal/desktop/lifecycle"
	"github.com/d31ma/sluice/internal/platform/autostart"
	runtimeexec "github.com/d31ma/sluice/internal/runtime"
	appui "github.com/d31ma/sluice/ui/app"
)

const (
	nativeSmokeDuration    = 1500 * time.Millisecond
	packagedRuntimeVersion = "26.33.02"
	runtimeShutdownTimeout = 8 * time.Second
)

type desktopOptions struct {
	executable           string
	stateRoot            string
	background           bool
	nativeSmoke          bool
	packagedRuntimeSmoke bool
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

type runtimeSession interface {
	Close(context.Context) error
}

type runtimeStarter func(context.Context, desktopOptions) (runtimeSession, error)

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
	packagedRuntimeSmoke := flags.Bool("packaged-runtime-smoke", false, "exercise the packaged runtime lifecycle without initializing the graphical shell")
	stateRoot := flags.String("qualification-state-root", "", "use an isolated state root during a qualification smoke run")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		if err := writeLine(stderr, "sluice-desktop: unexpected positional arguments"); err != nil {
			return 1
		}
		return 2
	}
	if err := validateQualificationOptions(*nativeSmoke, *packagedRuntimeSmoke, *stateRoot); err != nil {
		writeError(stderr, err)
		return 2
	}
	if err := startDesktop(ctx, desktopOptions{
		background:           *background,
		nativeSmoke:          *nativeSmoke,
		packagedRuntimeSmoke: *packagedRuntimeSmoke,
		executable:           executable,
		stateRoot:            *stateRoot,
	}); err != nil {
		writeError(stderr, err)
		return 1
	}
	return 0
}

func validateQualificationOptions(nativeSmoke, packagedRuntimeSmoke bool, stateRoot string) error {
	if nativeSmoke && packagedRuntimeSmoke {
		return fmt.Errorf("--native-smoke and --packaged-runtime-smoke are mutually exclusive")
	}
	if stateRoot == "" {
		return nil
	}
	if !nativeSmoke && !packagedRuntimeSmoke {
		return fmt.Errorf("--qualification-state-root requires a qualification smoke mode")
	}
	if !filepath.IsAbs(stateRoot) {
		return fmt.Errorf("--qualification-state-root must be absolute")
	}
	return nil
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
	if options.packagedRuntimeSmoke {
		return runDesktopWithLifecycle(ctx, options, startPackagedRuntime, func() error {
			return awaitPackagedRuntimeSmoke(ctx, nativeSmokeDuration)
		})
	}
	return runDesktopWithLifecycle(ctx, options, startPackagedRuntime, func() error {
		return runFyneDesktop(ctx, options)
	})
}

func runDesktopWithLifecycle(
	ctx context.Context,
	options desktopOptions,
	startRuntime runtimeStarter,
	runUI func() error,
) error {
	session, err := startRuntime(ctx, options)
	if err != nil {
		return err
	}
	uiErr := runUI()
	if session == nil {
		return uiErr
	}
	shutdown, cancel := context.WithTimeout(context.Background(), runtimeShutdownTimeout)
	defer cancel()
	return errors.Join(uiErr, session.Close(shutdown))
}

func startPackagedRuntime(ctx context.Context, options desktopOptions) (runtimeSession, error) {
	coordinator, packaged, err := openPackagedCoordinator(options)
	if err != nil {
		return nil, err
	}
	if !packaged {
		return nil, unpackagedRuntimeSmokeError(options.packagedRuntimeSmoke)
	}
	return startCoordinator(ctx, coordinator)
}

func unpackagedRuntimeSmokeError(required bool) error {
	if required {
		return fmt.Errorf("packaged runtime smoke requires an installed application package")
	}
	return nil
}

func awaitPackagedRuntimeSmoke(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func startCoordinator(ctx context.Context, coordinator *lifecycle.Coordinator) (runtimeSession, error) {
	session, err := coordinator.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("start packaged runtime: %w", err)
	}
	return session, nil
}

func openPackagedCoordinator(options desktopOptions) (*lifecycle.Coordinator, bool, error) {
	coordinator, err := lifecycle.NewPackaged(
		options.executable,
		options.stateRoot,
		packagedRuntimeVersion,
	)
	if errors.Is(err, runtimeexec.ErrNotPackaged) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("open packaged runtime: %w", err)
	}
	return coordinator, true, nil
}

func runFyneDesktop(ctx context.Context, options desktopOptions) error {
	var smokeLog bytes.Buffer
	if options.nativeSmoke {
		previousLogWriter := log.Writer()
		log.SetOutput(io.MultiWriter(previousLogWriter, &smokeLog))
		defer log.SetOutput(previousLogWriter)
	}

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
	return nativeSmokeLogFailure(smokeLog.String())
}

func nativeSmokeLogFailure(output string) error {
	if strings.Contains(output, "window creation error") {
		return fmt.Errorf("native window initialization failed")
	}
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
