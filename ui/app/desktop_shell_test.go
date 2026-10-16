//go:build !android && !ios

package appui_test

import (
	"testing"

	"fyne.io/fyne/v2"
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

func TestDesktopShellExposesStatusCopyToNativeAccessibility(t *testing.T) {
	application := test.NewApp()
	t.Cleanup(application.Quit)

	shell := appui.NewDesktopShell(application)
	labels := accessibleLabels(shell.Window().Content())
	for _, want := range []string{
		"Sluice",
		"Status",
		"Local agent",
		"Ready for configuration",
		"Connect an S3-compatible profile and choose a local folder to begin syncing.",
		"Get started",
		"Your storage remains under your control",
		"Profile and folder setup becomes available after the local service is connected.",
	} {
		if !labels[want] {
			t.Errorf("native accessibility labels do not contain %q", want)
		}
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

func accessibleLabels(object fyne.CanvasObject) map[string]bool {
	labels := make(map[string]bool)
	var visit func(fyne.CanvasObject)
	visit = func(current fyne.CanvasObject) {
		if current == nil || !current.Visible() {
			return
		}
		accessible, ok := current.(fyne.Accessible)
		if !ok {
			return
		}
		labels[accessible.AccessibilityLabel()] = true
		if children, ok := current.(*fyne.Container); ok {
			for _, child := range children.Objects {
				visit(child)
			}
		}
	}
	visit(object)
	return labels
}
