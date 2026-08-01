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

func (um *UIManager) browseSavePath(startPath, title string, directory bool) (string, error) {
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

	if um.Config.DocsBasePath == "" {
		return selected, nil
	}

	saveBase := filepath.Join(um.Config.DocsBasePath, "save")
	if rel, relErr := filepath.Rel(saveBase, selected); relErr == nil && !strings.HasPrefix(rel, "..") {
		return rel, nil
	}
	if rel, relErr := filepath.Rel(um.Config.DocsBasePath, selected); relErr == nil && !strings.HasPrefix(rel, "..") {
		return rel, nil
	}
	return selected, nil
}

func (um *UIManager) browseConfigPath(startPath string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	selected, err := zenity.SelectFile(
		zenity.Title("Select OpenTTD config file"),
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

	if um.Config.DocsBasePath != "" {
		if rel, relErr := filepath.Rel(um.Config.DocsBasePath, selected); relErr == nil && !strings.HasPrefix(rel, "..") {
			return rel, nil
		}
	}

	return selected, nil
}

// browseDirectory opens a Zenity directory picker and writes the chosen path into entry
func (um *UIManager) browseDirectory(entry *widget.Entry, title, logLabel string) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer func() {
			if r := recover(); r != nil {
				um.Logger.Append(fmt.Sprintf("CRITICAL: Zenity panicked: %v", r))
			}
		}()
		um.Logger.Append(fmt.Sprintf("Opening %s picker...", logLabel))
		directory, err := zenity.SelectFile(
			zenity.Directory(),
			zenity.Title(title),
			zenity.Filename(entry.Text),
		)
		um.Logger.Append(fmt.Sprintf("Picker closed (%s). Err: %v", logLabel, err))
		if err == nil && directory != "" {
			fyne.Do(func() {
				entry.SetText(directory)
			})
		}
	}()
}
