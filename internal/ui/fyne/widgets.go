package fyne

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// rightClickButton is a button that also handles right-clicks
type rightClickButton struct {
	widget.Button
	onSecondaryTapped func()
	onLaunch          func()
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

// TypedKey adds Enter/Return as "activate" alongside Button's own Space
// handling: OnTapped's 450ms double-tap detection is for the mouse, where a
// single tap is ambiguous between select and activate; a keyboard Enter isn't.
func (b *rightClickButton) TypedKey(ev *fyne.KeyEvent) {
	if (ev.Name == fyne.KeyReturn || ev.Name == fyne.KeyEnter) && b.onLaunch != nil {
		b.onLaunch()
		return
	}
	b.Button.TypedKey(ev)
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

// dialogEntry is a single-line Entry (including password entries) that routes
// Escape/Enter to a modal's cancel/save actions regardless of which field in
// the modal currently has focus (see docs/superpowers/specs on keyboard/focus).
type dialogEntry struct {
	widget.Entry
	onEscape, onEnter func()
}

func newDialogEntry(onEscape, onEnter func()) *dialogEntry {
	e := &dialogEntry{onEscape: onEscape, onEnter: onEnter}
	e.ExtendBaseWidget(e)
	return e
}

func (e *dialogEntry) TypedKey(key *fyne.KeyEvent) {
	if handleDialogKey(key, e.onEscape, e.onEnter) {
		return
	}
	e.Entry.TypedKey(key)
}

// dialogSelectEntry is a SelectEntry (a combo box: free text plus a dropdown)
// that routes Escape/Enter the same way as dialogEntry. SelectEntry has no
// TypedKey override of its own (it inherits Entry's directly), so this needs
// the same forwarding treatment.
type dialogSelectEntry struct {
	widget.SelectEntry
	onEscape, onEnter func()
}

func newDialogSelectEntry(options []string, onEscape, onEnter func()) *dialogSelectEntry {
	e := &dialogSelectEntry{onEscape: onEscape, onEnter: onEnter}
	e.ExtendBaseWidget(e)
	e.SetOptions(options)
	return e
}

func (e *dialogSelectEntry) TypedKey(key *fyne.KeyEvent) {
	if handleDialogKey(key, e.onEscape, e.onEnter) {
		return
	}
	e.SelectEntry.TypedKey(key)
}

// dialogMultiLineEntry is a multiline Entry for modal forms. It never forwards
// Enter (a multiline field's Enter always means "insert a newline"), and Tab
// moves focus to the next widget instead of inserting a literal tab character.
type dialogMultiLineEntry struct {
	widget.Entry
	onEscape func()
}

func newDialogMultiLineEntry(onEscape func()) *dialogMultiLineEntry {
	e := &dialogMultiLineEntry{onEscape: onEscape}
	e.MultiLine = true
	e.ExtendBaseWidget(e)
	return e
}

func (e *dialogMultiLineEntry) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyTab {
		// fyne.KeyEvent carries no modifier; a multiline Entry's own typedKeyTab
		// checks the driver directly (entry.go) for the same reason, since this
		// wrapper intercepts Tab before that stock handling ever runs.
		shift := false
		if dd, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
			shift = dd.CurrentKeyModifiers()&fyne.KeyModifierShift != 0
		}
		if c := fyne.CurrentApp().Driver().CanvasForObject(e); c != nil {
			if shift {
				c.FocusPrevious()
			} else {
				c.FocusNext()
			}
		}
		return
	}
	if handleDialogKey(key, e.onEscape, nil) {
		return
	}
	e.Entry.TypedKey(key)
}

// dialogSelect is a Select that routes Escape/Enter the same way as dialogEntry.
type dialogSelect struct {
	widget.Select
	onEscape, onEnter func()
}

func newDialogSelect(options []string, changed func(string), onEscape, onEnter func()) *dialogSelect {
	s := &dialogSelect{onEscape: onEscape, onEnter: onEnter}
	s.Options = options
	s.OnChanged = changed
	s.ExtendBaseWidget(s)
	return s
}

func (s *dialogSelect) TypedKey(key *fyne.KeyEvent) {
	if handleDialogKey(key, s.onEscape, s.onEnter) {
		return
	}
	s.Select.TypedKey(key)
}

// dialogCheck is a Check that routes Escape/Enter the same way as dialogEntry.
type dialogCheck struct {
	widget.Check
	onEscape, onEnter func()
}

func newDialogCheck(label string, changed func(bool), onEscape, onEnter func()) *dialogCheck {
	c := &dialogCheck{onEscape: onEscape, onEnter: onEnter}
	c.Text = label
	c.OnChanged = changed
	c.ExtendBaseWidget(c)
	return c
}

// Space is absent by design: stock widget.Check.TypedKey is an empty stub,
// and Space already toggles the check via TypedRune(' ').
func (c *dialogCheck) TypedKey(key *fyne.KeyEvent) {
	if handleDialogKey(key, c.onEscape, c.onEnter) {
		return
	}
	c.Check.TypedKey(key)
}

// dialogButton is a Button that routes Escape to a modal's cancel action, so a
// focused Cancel/Save/Browse button (or a SegmentedRadio option) doesn't
// swallow it just by holding focus. Enter is treated the same as Space
// (activates *this* button, matching the universal button convention), not
// forwarded to the modal's default action the way dialogEntry/Select/Check do.
type dialogButton struct {
	widget.Button
	onEscape func()
}

func newDialogButton(label string, tapped func(), onEscape func()) *dialogButton {
	b := &dialogButton{onEscape: onEscape}
	b.Text = label
	b.OnTapped = tapped
	b.ExtendBaseWidget(b)
	return b
}

func (b *dialogButton) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyEscape:
		if b.onEscape != nil {
			b.onEscape()
			return
		}
	case fyne.KeyReturn, fyne.KeyEnter:
		b.Tapped(nil)
		return
	}
	b.Button.TypedKey(key)
}
