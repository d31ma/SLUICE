package appui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func newAccessibleCard(title, subtitle string, content fyne.CanvasObject) fyne.CanvasObject {
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	titleLabel.SizeName = theme.SizeNameHeadingText
	body := container.NewVBox(
		titleLabel,
		widget.NewLabel(subtitle),
		content,
	)
	return container.NewStack(
		widget.NewCard("", "", nil),
		container.NewPadded(container.NewPadded(body)),
	)
}
