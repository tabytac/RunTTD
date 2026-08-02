package fyne

import "fyne.io/fyne/v2"

// handleDialogKey runs onEscape/onEnter for a key event and reports whether it
// consumed the key. Shift+Enter isn't special-cased here: multiline fields use
// their own wrapper that never sets onEnter, so a plain Entry always treats
// Enter as "submit" and a multiline Entry always treats it as "newline".
func handleDialogKey(ev *fyne.KeyEvent, onEscape, onEnter func()) bool {
	switch ev.Name {
	case fyne.KeyEscape:
		if onEscape != nil {
			onEscape()
			return true
		}
	case fyne.KeyReturn, fyne.KeyEnter:
		if onEnter != nil {
			onEnter()
			return true
		}
	}
	return false
}
