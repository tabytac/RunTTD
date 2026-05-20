package fyne

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// NewSectionTitle returns a bold label for use as a section header
func NewSectionTitle(title string) *widget.Label {
	label := widget.NewLabel(title)
	label.TextStyle = fyne.TextStyle{Bold: true}
	return label
}

// NewSectionHeader returns a bold section title stacked above a thin separator
func NewSectionHeader(title string) fyne.CanvasObject {
	return container.NewVBox(
		NewSectionTitle(title),
		widget.NewSeparator(),
	)
}

// NewSectionDescription returns a wrapped italic label for descriptive text under a control
func NewSectionDescription(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	label.TextStyle = fyne.TextStyle{Italic: true}
	return label
}

// NewLabeledCheckWithDescription returns a checkbox paired with an indented italic description label
func NewLabeledCheckWithDescription(label, description string, checked bool) (*widget.Check, fyne.CanvasObject) {
	check := widget.NewCheck(label, nil)
	check.SetChecked(checked)
	desc := NewSectionDescription(description)
	group := container.NewVBox(
		check,
		container.NewBorder(nil, nil, widget.NewLabel("    "), nil, desc),
	)
	return check, group
}
