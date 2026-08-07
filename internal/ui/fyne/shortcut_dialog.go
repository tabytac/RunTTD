package fyne

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	apppkg "runttd/internal/app"
	"runttd/internal/domain"
)

// showShortcutDialog asks for a title, then writes a shortcut that launches
// profile headlessly. The command line is fixed, so the only choice here is what
// to call the thing; the profile already carries everything else the launch needs.
func (um *UIManager) showShortcutDialog(profile domain.Profile) {
	var popup *widget.PopUp
	dismiss := func() {
		if popup != nil {
			popup.Hide()
			um.shortcutOverlay, um.shortcutOnEscape = nil, nil
		}
	}

	var create func()
	titleEntry := newDialogEntry(dismiss, func() { create() })
	titleEntry.SetText(profile.Name)
	titleEntry.SetPlaceHolder("Shortcut name")

	hint := NewSectionDescription(fmt.Sprintf(
		"Creates a shortcut next to RunTTD that launches %q with no launcher window.", profile.Name))

	create = func() {
		path, err := apppkg.GenerateProfileShortcut(profile, titleEntry.Text)
		if err != nil {
			um.showErrorf("could not create the shortcut: %w", err)
			return
		}
		um.Logger.Append("Created shortcut " + path)
		dismiss()
		um.showToast("Created " + filepath.Base(path))
	}

	createBtn := newDialogButton("Create", create, dismiss)
	createBtn.Importance = widget.HighImportance
	cancelBtn := newDialogButton("Cancel", dismiss, dismiss)

	content := container.NewVBox(titleEntry, hint)
	popup = NewModalDialog(um.Window.Canvas(), "Create Shortcut", content, cancelBtn, createBtn)
	um.shortcutOverlay, um.shortcutOnEscape = popup, dismiss
	popup.Show()
	um.Window.Canvas().Focus(titleEntry)
}
