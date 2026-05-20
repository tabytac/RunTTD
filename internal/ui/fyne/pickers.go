package fyne

import (
	"fmt"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"
)

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
