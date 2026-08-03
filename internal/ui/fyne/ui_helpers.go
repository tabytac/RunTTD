package fyne

import (
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
)

// showError displays a plain-message error dialog on the main window.
func (um *UIManager) showError(msg string) { dialog.ShowError(errors.New(msg), um.Window) }

// showErrorf displays an error dialog from a format string (use %w to keep a wrapped cause).
func (um *UIManager) showErrorf(format string, a ...any) {
	dialog.ShowError(fmt.Errorf(format, a...), um.Window)
}

const (
	// A word-wrapping label's minimum width is just its padding, so without a floor
	// Fyne sizes a confirm from the button row alone and breaks long paths mid-word.
	confirmDialogWidth    = 560
	confirmDialogMaxWidth = 900
	// Width a confirm spends on chrome: popup padding, dialog padding, label padding.
	confirmDialogChrome = 56
)

// confirmWidthFor widens a confirm until its longest unbreakable token (in practice a
// file path) fits on one line, so wrapping falls on spaces instead of mid-word.
func confirmWidthFor(msg string) float32 {
	var widest float32
	for _, token := range strings.Fields(msg) {
		if w := fyne.MeasureText(token, theme.TextSize(), fyne.TextStyle{}).Width; w > widest {
			widest = w
		}
	}
	switch need := widest + confirmDialogChrome; {
	case need < confirmDialogWidth:
		return confirmDialogWidth
	case need > confirmDialogMaxWidth:
		return confirmDialogMaxWidth
	default:
		return need
	}
}

// newConfirmDialog returns an unshown confirm with a centred, word-wrapping message.
// The zero height leaves the height to the content, and a modal popup clamps both axes
// to the canvas, so neither dimension can overflow a small window.
func (um *UIManager) newConfirmDialog(title, confirm, dismiss, msg string, callback func(bool)) *dialog.ConfirmDialog {
	msgLabel := widget.NewLabel(msg)
	msgLabel.Alignment = fyne.TextAlignCenter
	msgLabel.Wrapping = fyne.TextWrapWord
	d := dialog.NewCustomConfirm(title, confirm, dismiss, msgLabel, callback, um.Window)
	d.Resize(fyne.NewSize(confirmWidthFor(msg), 0))
	return d
}

// saveConfigOrWarn writes the config, dialoging on failure so a write error (a
// read-only, full, or dead path) isn't silently lost. Returns whether it
// succeeded, so a caller mid-edit (a modal Save button) can revert any in-memory
// mutation and keep its dialog open instead of proceeding as if the write landed;
// a caller with nothing sane to revert to may ignore the return, since the
// dialog above already told the user.
func (um *UIManager) saveConfigOrWarn() bool {
	if err := domain.SaveConfig(um.ConfigPath, um.Config); err != nil {
		um.showErrorf("could not save to %s: %w", um.ConfigPath, err)
		return false
	}
	return true
}

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

// NewLabeledCheckWithDescription returns a checkbox paired with an indented italic description label.
// onEscape/onEnter route through dialogCheck for modal use; pass nil, nil for a
// non-modal view (e.g. onboarding), where they're a no-op.
func NewLabeledCheckWithDescription(label, description string, checked bool, onEscape, onEnter func()) (*dialogCheck, fyne.CanvasObject) {
	check := newDialogCheck(label, nil, onEscape, onEnter)
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

// NewModalDialog returns a modal popup with a bold title bar, content area, and centred button toolbar.
// buttons takes fyne.CanvasObject (not *widget.Button) so dialogButton (and any other button-like
// wrapper) can be passed directly alongside plain widget.Button.
func NewModalDialog(canvas fyne.Canvas, title string, content fyne.CanvasObject, buttons ...fyne.CanvasObject) *widget.PopUp {
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
