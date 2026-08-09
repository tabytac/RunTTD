package fyne

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"
)

// browseSavePath runs off the UI thread; docsBase is captured by the caller on
// the UI thread, since a settings save can write um.Config mid-pick.
func (um *UIManager) browseSavePath(docsBase, startPath, title string, directory bool) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var (
		selected string
		err      error
	)

	if directory {
		selected, err = zenity.SelectFile(
			zenity.Directory(),
			zenity.Title(title),
			zenity.Filename(startPath),
		)
	} else {
		selected, err = zenity.SelectFile(
			zenity.Title(title),
			zenity.FileFilters{
				{Name: "OpenTTD Saves/Scenarios", Patterns: []string{"*.sav", "*.scn"}},
			},
			zenity.Filename(startPath),
		)
	}
	if errors.Is(err, zenity.ErrCanceled) {
		return "", nil // a cancel is not an error; callers dialog on non-nil
	}
	if err != nil || selected == "" {
		return "", err
	}

	if docsBase == "" {
		return selected, nil
	}

	saveBase := filepath.Join(docsBase, "save")
	if rel, relErr := filepath.Rel(saveBase, selected); relErr == nil && !strings.HasPrefix(rel, "..") {
		return rel, nil
	}
	if rel, relErr := filepath.Rel(docsBase, selected); relErr == nil && !strings.HasPrefix(rel, "..") {
		return rel, nil
	}
	return selected, nil
}

// browseConfigPath runs off the UI thread; docsBase is captured by the caller
// on the UI thread for the same reason as browseSavePath's.
func (um *UIManager) browseConfigPath(docsBase, startPath string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	selected, err := zenity.SelectFile(
		zenity.Title("Select OpenTTD Config File"),
		zenity.FileFilters{
			{Name: "OpenTTD Config", Patterns: []string{"*.cfg"}},
		},
		zenity.Filename(startPath),
	)
	if errors.Is(err, zenity.ErrCanceled) {
		return "", nil
	}
	if err != nil || selected == "" {
		return "", err
	}

	if docsBase != "" {
		if rel, relErr := filepath.Rel(docsBase, selected); relErr == nil && !strings.HasPrefix(rel, "..") {
			return rel, nil
		}
	}

	return selected, nil
}

// browseDirectory opens a Zenity directory picker and writes the chosen path
// into entry; onDone (nil ok) runs on the UI thread once the picker closes.
func (um *UIManager) browseDirectory(entry *widget.Entry, title, logLabel string, onDone func()) {
	startText := entry.Text // read on the UI thread; the user can type while the picker is open
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if onDone != nil {
			defer fyne.Do(onDone) // deferred so it also fires on cancel, error, and panic
		}
		defer func() {
			if r := recover(); r != nil {
				um.Logger.Append(fmt.Sprintf("CRITICAL: Zenity panicked: %v", r))
				fyne.Do(func() {
					um.showErrorf("could not open the folder picker: %v", r)
				})
			}
		}()
		um.Logger.Append(fmt.Sprintf("Opening %s picker…", logLabel))
		directory, err := zenity.SelectFile(
			zenity.Directory(),
			zenity.Title(title),
			zenity.Filename(startText),
		)
		um.Logger.Append(fmt.Sprintf("Picker closed (%s). Err: %v", logLabel, err))
		if errors.Is(err, zenity.ErrCanceled) {
			return // a cancel is not an error; stay silent
		}
		if err != nil {
			fyne.Do(func() {
				um.showErrorf("could not open the folder picker: %w", err)
			})
			return
		}
		if directory != "" {
			fyne.Do(func() {
				entry.SetText(directory)
			})
		}
	}()
}
