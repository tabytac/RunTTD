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

// section accumulates a profile-detail group. Short label/value pairs align in
// the form; long values stack full-width in extra so wrapping can't clip
// (FormLayout sizes rows from the unwrapped value).
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
	if strings.TrimSpace(value) == "" {
		return
	}
	val := widget.NewLabel(value)
	val.Wrapping = fyne.TextWrapWord
	val.Selectable = true
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

// addPathField renders a full-width path value (in s.extra, like addLongField)
// with a reveal-in-file-browser button. isFile selects the file in its folder;
// otherwise the folder is opened. Empty values render nothing.
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

// emit appends the section's content to dst as a themed box, or nothing if the
// section gathered no fields.
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
