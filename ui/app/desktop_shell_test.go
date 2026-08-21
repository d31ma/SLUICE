//go:build !android && !ios

package appui_test

import (
	"testing"

	"fyne.io/fyne/v2/test"
	appui "github.com/d31ma/sluice/ui/app"
)

func TestDesktopShellBuildsSharedStatusWindow(t *testing.T) {
	application := test.NewApp()
	t.Cleanup(application.Quit)

	shell := appui.NewDesktopShell(application)
	if shell.Window().Title() != "Sluice" {
		t.Fatalf("window title = %q, want Sluice", shell.Window().Title())
	}
	if shell.Window().Content() == nil {
		t.Fatal("window content is nil")
	}
}

func TestDesktopShellRoutesQuitRequest(t *testing.T) {
	application := test.NewApp()
	t.Cleanup(application.Quit)
	shell := appui.NewDesktopShell(application)
	calls := 0
	shell.SetQuitHandler(func() { calls++ })

	shell.RequestQuit()
	shell.RequestQuit()

	if calls != 2 {
		t.Fatalf("quit handler calls = %d, want 2", calls)
	}
}
