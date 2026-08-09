package fyne

import (
	"fyne.io/fyne/v2"
)

// routeViewKey passes a key a focused widget declined on to the view's own
// handler, which is otherwise only reachable when nothing has focus.
func (um *UIManager) routeViewKey(key *fyne.KeyEvent) {
	if um.viewKeys != nil {
		um.viewKeys(key)
	}
}

// escapeOverlayAction resolves what Escape should do for the given top overlay, or nil
// if there's nothing to dismiss. Scoped overlays (settings, editor, a blocking confirm)
// route through their own dismiss so callbacks still fire; raw top.Hide() would skip them.
func (um *UIManager) escapeOverlayAction(top fyne.CanvasObject) func() {
	switch {
	case top == nil:
		return nil
	case top == um.settingsOverlay && um.settingsOnEscape != nil:
		return um.settingsOnEscape
	case top == um.editorOverlay && um.editorOnEscape != nil:
		return um.editorOnEscape
	case top == um.shortcutOverlay && um.shortcutOnEscape != nil:
		return um.shortcutOnEscape
	case top == um.blockingConfirm && um.blockingConfirmHide != nil:
		return um.blockingConfirmHide
	default:
		// A raw Hide skips the dialog's own callback, so a confirm dismissed here
		// would leave its affirmative action armed and answer the next overlay's
		// Enter instead of its own.
		return func() {
			um.confirmAction = nil
			top.Hide()
		}
	}
}

// escapeAction resolves Escape: an open overlay always wins over the active
// full-screen view's Back action.
func (um *UIManager) escapeAction() func() {
	if action := um.escapeOverlayAction(um.Window.Canvas().Overlays().Top()); action != nil {
		return action
	}
	return um.viewEscape
}

// runViewEscape fires the active full-screen view's Back action; a focused
// button would otherwise swallow Escape.
func (um *UIManager) runViewEscape() {
	if um.viewEscape != nil {
		um.viewEscape()
	}
}

// runLibraryRescan fires the library view's Refresh action, from either the canvas
// F5 handler or a focused library button. An open overlay suppresses it, matching
// the guard on the main view's other accelerators.
func (um *UIManager) runLibraryRescan() {
	if um.libraryRescan != nil && um.Window.Canvas().Overlays().Top() == nil {
		um.libraryRescan()
	}
}
