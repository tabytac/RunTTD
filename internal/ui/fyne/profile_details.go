package fyne

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/platform"
)

// section is a profile-detail group: short pairs align in form, long values go
// full-width in extra so wrapping can't clip (FormLayout sizes from the unwrapped value).
type section struct {
	form  *fyne.Container
	extra []fyne.CanvasObject
	count int
}

func newSection() *section {
	return &section{form: container.New(layout.NewFormLayout())}
}

func (s *section) addField(label, value string, mono bool) {
	if value == "" {
		return
	}
	val := widget.NewLabel(value)
	val.Wrapping = fyne.TextWrapOff
	val.Selectable = true
	if mono {
		val.TextStyle = fyne.TextStyle{Monospace: true}
	}
	s.form.Add(mutedLabel(label))
	s.form.Add(val)
	s.count++
}

func (s *section) addLongField(label, value string, mono bool) {
	s.longField(label, value, mono, widget.MediumImportance)
}

// addMutedLongField renders a long field at low emphasis, for a value that is
// configured but not currently applied; it stays visible and selectable.
func (s *section) addMutedLongField(label, value string, mono bool) {
	s.longField(label, value, mono, widget.LowImportance)
}

func (s *section) longField(label, value string, mono bool, importance widget.Importance) {
	if strings.TrimSpace(value) == "" {
		return
	}
	val := widget.NewLabel(value)
	val.Wrapping = fyne.TextWrapWord
	val.Selectable = true
	val.Importance = importance
	if mono {
		val.TextStyle = fyne.TextStyle{Monospace: true}
	}
	s.extra = append(s.extra, mutedLabel(label), val)
	s.count++
}

// addReveal adds a masked value with an eye button that toggles plaintext.
func (s *section) addReveal(label, value string) {
	if value == "" {
		return
	}
	val := widget.NewLabel("••••••••")
	val.Selectable = true
	shown := false
	var btn *widget.Button
	btn = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
		shown = !shown
		if shown {
			val.SetText(value)
			btn.SetIcon(theme.VisibilityOffIcon())
		} else {
			val.SetText("••••••••")
			btn.SetIcon(theme.VisibilityIcon())
		}
	})
	btn.Importance = widget.LowImportance
	s.form.Add(mutedLabel(label))
	s.form.Add(container.NewBorder(nil, nil, nil, btn, val))
	s.count++
}

// addPathField adds a full-width path with a reveal button (isFile selects the file, else opens the folder); empty renders nothing.
func (um *UIManager) addPathField(s *section, label, value string, isFile bool) {
	if strings.TrimSpace(value) == "" {
		return
	}
	val := widget.NewLabel(value)
	val.Wrapping = fyne.TextWrapWord
	val.Selectable = true
	val.TextStyle = fyne.TextStyle{Monospace: true}
	btn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		var err error
		if isFile {
			err = platform.RevealFileInFileManager(value)
		} else {
			err = platform.RevealInFileManager(value)
		}
		if err != nil {
			um.Logger.Append(fmt.Sprintf("Reveal failed for %s: %v", value, err))
			um.showErrorf("couldn't open the location: %w", err)
		}
	})
	btn.Importance = widget.LowImportance
	row := container.NewBorder(nil, nil, nil, btn, val)
	s.extra = append(s.extra, mutedLabel(label), row)
	s.count++
}

// emit appends the section to dst as a themed box, or nothing if it gathered no fields.
func (s *section) emit(title string, dst *fyne.Container) {
	if s.count == 0 {
		return
	}
	body := container.NewVBox(NewSectionHeader(title), s.form)
	for _, o := range s.extra {
		body.Add(o)
	}
	dst.Add(NewThemedBox(ColorNameContent, container.NewPadded(body)))
}
