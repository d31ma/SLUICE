//go:build !android && !ios

package appui

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	windowWidth  = 760
	windowHeight = 480
)

type DesktopShell struct {
	application fyne.App
	window      fyne.Window
	quitHandler func()
	handlerMu   sync.RWMutex
}

func NewDesktopShell(application fyne.App) *DesktopShell {
	icon := theme.FolderIcon()
	application.SetIcon(icon)

	shell := &DesktopShell{
		application: application,
		window:      application.NewWindow("Sluice"),
	}
	shell.window.Resize(fyne.NewSize(windowWidth, windowHeight))
	shell.window.CenterOnScreen()
	shell.window.SetContent(shell.buildContent())
	shell.window.SetCloseIntercept(shell.Hide)
	shell.configureSystemTray(icon)
	return shell
}

func (shell *DesktopShell) Window() fyne.Window {
	return shell.window
}

func (shell *DesktopShell) SetQuitHandler(handler func()) {
	shell.handlerMu.Lock()
	defer shell.handlerMu.Unlock()
	shell.quitHandler = handler
}

func (shell *DesktopShell) RequestQuit() {
	shell.handlerMu.RLock()
	handler := shell.quitHandler
	shell.handlerMu.RUnlock()
	if handler == nil {
		shell.Quit()
		return
	}
	handler()
}

func (shell *DesktopShell) Show() {
	shell.window.Show()
	shell.window.RequestFocus()
}

func (shell *DesktopShell) Hide() {
	shell.window.Hide()
}

func (shell *DesktopShell) Quit() {
	shell.application.Quit()
}

func (shell *DesktopShell) Notify(title, content string) {
	shell.application.SendNotification(fyne.NewNotification(title, content))
}

func (shell *DesktopShell) Run() {
	shell.application.Run()
}

func (shell *DesktopShell) configureSystemTray(icon fyne.Resource) {
	desktopApplication, supported := shell.application.(desktop.App)
	if !supported {
		return
	}
	desktopApplication.SetSystemTrayIcon(icon)
	desktopApplication.SetSystemTrayWindow(shell.window)
	desktopApplication.SetSystemTrayMenu(fyne.NewMenu(
		"Sluice",
		fyne.NewMenuItem("Show Sluice", shell.Show),
		fyne.NewMenuItem("Hide", shell.Hide),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Quit", shell.RequestQuit),
	))
}

func (shell *DesktopShell) buildContent() fyne.CanvasObject {
	status := widget.NewLabelWithStyle("Ready for configuration", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	description := widget.NewLabel("Connect an S3-compatible profile and choose a local folder to begin syncing.")
	description.Wrapping = fyne.TextWrapWord
	return container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Sluice", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		container.NewVBox(
			widget.NewCard("Status", "Local agent", container.NewVBox(status, description)),
			widget.NewCard(
				"Get started",
				"Your storage remains under your control",
				widget.NewLabel("Profile and folder setup becomes available after the local service is connected."),
			),
		),
	)
}
