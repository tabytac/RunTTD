package fyne

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	apppkg "runttd/internal/app"
	"runttd/internal/domain"
	"runttd/internal/platform"
)

// OnStarted is the ProcessObserver callback triggered when OpenTTD starts successfully
func (um *UIManager) OnStarted() {
	um.OnOpenTTDStarted()
}

// showToast shows a temporary notification at the bottom of the window
func (um *UIManager) showToast(message string) {
	toast := widget.NewLabel(message)
	toast.Alignment = fyne.TextAlignCenter

	content := container.NewPadded(toast)
	pop := widget.NewPopUp(content, um.Window.Canvas())

	// Position at bottom center, accounting for the popup's actual width.
	size := um.Window.Content().Size()
	popWidth := pop.MinSize().Width
	pop.ShowAtPosition(fyne.NewPos((size.Width-popWidth)/2, size.Height-60))

	go func() {
		time.Sleep(3 * time.Second)
		fyne.Do(func() {
			pop.Hide()
		})
	}()
}

// hideLaunchCancel hides whichever log view's Cancel button is currently on screen.
func (um *UIManager) hideLaunchCancel() {
	if um.launchCancelBtn != nil {
		um.launchCancelBtn.Hide()
		um.launchCancelBtn = nil
	}
}

// openingLaunchStatus is the status a log view opens on. A launch already in
// flight is not restarted by opening this view, so its own progress is the
// honest thing to show rather than a fixed opening line that never advances.
func (um *UIManager) openingLaunchStatus(isLaunch bool) string {
	if isLaunch && um.launchInProgress && um.launchStatus != "" {
		return um.launchStatus
	}
	return "Preparing launch"
}

// launchViewProfileIdx redirects a log-view request to the launch already in
// flight. Opening this view will not start the requested profile, so describing
// it would caption one launch's logs with another launch's details.
func (um *UIManager) launchViewProfileIdx(requested int) int {
	if requested >= 0 && um.launchInProgress && um.launchProfileIdx >= 0 && um.launchProfileIdx < len(um.Config.Profiles) {
		return um.launchProfileIdx
	}
	return requested
}

// showLogView shows a screen with live logs while launching a profile or in standalone mode
func (um *UIManager) showLogView(profileIdx int) {
	var profile domain.Profile
	isLaunch := profileIdx >= 0
	profileIdx = um.launchViewProfileIdx(profileIdx)
	if isLaunch {
		profile = um.Config.Profiles[profileIdx]
	}

	statusBinding := binding.NewString()
	_ = statusBinding.Set(um.openingLaunchStatus(isLaunch))

	var summaryObj fyne.CanvasObject
	if isLaunch {
		summary := widget.NewLabel(fmt.Sprintf("Profile: %s\nVersion: %s\nSave path: %s\nServer: %s", profile.Name, valueOrDefault(profile.Version, "latest"), valueOrDefault(profile.SavePath, "(none)"), valueOrDefault(profile.ServerIpPort, "(none)")))
		summary.Wrapping = fyne.TextWrapWord
		summaryObj = widget.NewCard("Launching", "Current launch context", summary)
	}

	statusLabel := widget.NewLabelWithData(statusBinding)
	statusLabel.Wrapping = fyne.TextWrapWord
	statusObj := widget.NewCard("Status", "Background operations", statusLabel)
	if !isLaunch {
		statusObj.Hide()
	}

	// Create log text widget using data binding (thread-safe)
	logBinding := binding.NewString()
	logLabel := widget.NewLabelWithData(logBinding)
	logLabel.Wrapping = fyne.TextWrapWord

	logBox := container.NewVScroll(logLabel)
	logBox.SetMinSize(fyne.NewSize(600, 400))

	// Update the log display whenever the logger gains new lines. The last seen
	// count is tracked so an unchanged buffer skips the rebuild, the binding
	// write, and the scroll, leaving the user free to scroll up without being
	// yanked back to the bottom every tick.
	lastLineCount := -1
	updateLogDisplay := func() {
		if um.Logger.Len() == lastLineCount {
			return
		}
		logs := um.Logger.GetAll()
		lastLineCount = len(logs)
		text := strings.Join(logs, "\n")
		fyne.Do(func() {
			_ = logBinding.Set(text)
			logBox.ScrollToBottom()
		})
	}

	// Start a goroutine to periodically update the log display
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				updateLogDisplay()
			case <-done:
				return
			}
		}
	}()

	// Leaving mid-launch is allowed: the launch goroutine outlives this view.
	back := func() {
		select {
		case <-done:
		default:
			close(done)
		}
		um.viewEscape = nil
		um.Window.SetContent(um.makeMainView())
	}
	closeBtn := newDialogButton("Back to profiles", back, um.runViewEscape)

	copyBtn := newDialogButton("Copy to clipboard", func() {
		um.App.Clipboard().SetContent(strings.Join(um.Logger.GetAll(), "\n"))
		um.showToast("Logs copied to clipboard!")
	}, um.runViewEscape)
	copyBtn.Icon = theme.ContentCopyIcon()

	clearBtn := newDialogButton("Clear logs", func() {
		confirmDlg := um.newConfirmDialog("Clear logs?", "Clear", "Cancel",
			"Remove all messages from this view? Any saved log file is left untouched.",
			func(ok bool) {
				if !ok {
					return
				}
				um.Logger.Clear()
				// Set the display directly (we're on the UI thread); the ticker
				// re-syncs its line count on the next pass.
				_ = logBinding.Set("")
				um.showToast("Logs cleared")
			})
		confirmDlg.SetConfirmImportance(widget.DangerImportance)
		confirmDlg.Show()
	}, um.runViewEscape)
	clearBtn.Icon = theme.ContentClearIcon()

	cancelBtn := newDialogButton("Cancel", func() {
		if um.launchCancel != nil {
			um.launchCancel()
		}
	}, um.runViewEscape)
	cancelBtn.Importance = widget.DangerImportance
	cancelBtn.Hide()

	top := container.NewVBox()
	if isLaunch {
		top.Add(summaryObj)
		top.Add(statusObj)
	}

	content := container.NewBorder(
		top,
		container.NewHBox(closeBtn, copyBtn, clearBtn, cancelBtn),
		nil,
		nil,
		logBox,
	)

	um.viewEscape = back
	um.Window.SetContent(content)

	// Launch OpenTTD in background if requested. Guarded at the UIManager level
	// (not mainView.launchInProgress, which this path bypasses entirely) so
	// leaving this view mid-download and pressing Run again can't start a second
	// concurrent download into the same folder.
	if isLaunch {
		if um.launchInProgress {
			// The original launch is still running (this is a fresh showLogView,
			// e.g. after "Back to profiles" then Run again); um.launchCancel still
			// targets it, so offer Cancel here too even though this view's own
			// status/progress aren't wired to that goroutine.
			um.showToast("A launch is already in progress")
			um.launchCancelBtn = cancelBtn
			cancelBtn.Show()
		} else {
			um.launchInProgress = true
			ctx, cancel := context.WithCancel(context.Background())
			um.launchCancel = cancel
			um.launchCancelBtn = cancelBtn
			cancelBtn.Show()
			lastPct := -1
			go func() {
				defer fyne.Do(func() {
					um.launchInProgress = false
					cancel()
					um.launchCancel = nil
					um.hideLaunchCancel()
				})
				um.launchProfile(ctx, profile, func(status string) {
					// binding.String.Set is documented safe from any goroutine.
					_ = statusBinding.Set(status)
				}, func(done, total int64) {
					if total <= 0 {
						return // unknown size: leave the current status text alone
					}
					if done >= total {
						fyne.Do(um.hideLaunchCancel) // extraction isn't cancellable; stop offering to
						_ = statusBinding.Set("Extracting (this can take a moment for large installs)")
						return
					}
					pct := int(done * 100 / total)
					if pct == lastPct {
						return // throttle to whole-percent steps
					}
					lastPct = pct
					_ = statusBinding.Set(fmt.Sprintf("Downloading… %d%%", pct))
				}, nil)
			}()
		}
	}
}

// launchProfile adapts the app-layer pipeline to this UIManager. ctx governs
// only the version-check and download/extract steps; cancelling it is how the
// Cancel button works.
func (um *UIManager) launchProfile(ctx context.Context, profile domain.Profile, updateStatus func(status string), onProgress platform.ProgressFunc, onError func()) {
	apppkg.LaunchProfile(ctx, profile, apppkg.LaunchDeps{
		Config:           um.Config,
		Logger:           um.Logger,
		Observer:         um,
		UpdateStatus:     updateStatus,
		OnProgress:       onProgress,
		OnError:          onError,
		InvalidateCaches: func() { um.diskLookups.invalidate() },
		Confirm:          um.confirmVeryOldVersion,
	})
}

// confirmVeryOldVersion asks whether to download a build that needs manual setup
// and blocks until the user answers. Callers are background goroutines, so the
// dialog is raised through fyne.Do.
func (um *UIManager) confirmVeryOldVersion(message string) bool {
	proceed := make(chan bool, 1)
	fyne.Do(func() {
		confirm := um.newConfirmDialog("Very old OpenTTD version", "Continue", "Cancel", message, func(ok bool) {
			um.blockingConfirm, um.blockingConfirmHide = nil, nil
			proceed <- ok
		})
		confirm.Show()
		// Escape reaches only the raw overlay, whose Hide() skips this callback;
		// route it through the dialog's own Hide() so proceed always gets a value.
		if top, ok := um.Window.Canvas().Overlays().Top().(*widget.PopUp); ok {
			um.blockingConfirm = top
			um.blockingConfirmHide = confirm.Hide
		}
	})
	return <-proceed
}
