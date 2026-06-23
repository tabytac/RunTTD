package fyne

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// rightClickButton is a button that also handles right-clicks
type rightClickButton struct {
	widget.Button
	onSecondaryTapped func()
}

func newRightClickButton(tapped func(), secondaryTapped func()) *rightClickButton {
	b := &rightClickButton{onSecondaryTapped: secondaryTapped}
	b.OnTapped = tapped
	b.Importance = widget.LowImportance
	b.ExtendBaseWidget(b)
	return b
}

func (b *rightClickButton) TappedSecondary(_ *fyne.PointEvent) {
	if b.onSecondaryTapped != nil {
		b.onSecondaryTapped()
	}
}

// searchEntry is an Entry that invokes onEscape when Escape is pressed.
type searchEntry struct {
	widget.Entry
	onEscape func()
}

func newSearchEntry() *searchEntry {
	e := &searchEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *searchEntry) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape && e.onEscape != nil {
		e.onEscape()
		return
	}
	e.Entry.TypedKey(key)
}
