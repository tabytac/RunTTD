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

// NewLabeledField stacks a plain field label, an indented italic description,
// and a control vertically. Used for onboarding path/preference rows so that the
// field name, its explanation, and its input read as one consistent unit. The
// label is intentionally not bold, so section headers stay visually dominant
// above the fields they group.
func NewLabeledField(label, description string, control fyne.CanvasObject) fyne.CanvasObject {
	return container.NewVBox(
		widget.NewLabel(label),
		container.NewBorder(nil, nil, widget.NewLabel("    "), nil, NewSectionDescription(description)),
		control,
	)
}

// NewModalDialog returns a modal popup with a bold title bar, content area, and centred button toolbar
func NewModalDialog(canvas fyne.Canvas, title string, content fyne.CanvasObject, buttons ...*widget.Button) *widget.PopUp {
	paddedButtons := make([]fyne.CanvasObject, len(buttons))
	for i, btn := range buttons {
		paddedButtons[i] = container.NewPadded(btn)
	}
	toolbar := container.NewCenter(container.NewHBox(paddedButtons...))

	frame := container.NewBorder(
		container.NewPadded(NewSectionTitle(title)),
		container.NewPadded(toolbar),
		nil,
		nil,
		container.NewPadded(content),
	)
	return widget.NewModalPopUp(frame, canvas)
}
