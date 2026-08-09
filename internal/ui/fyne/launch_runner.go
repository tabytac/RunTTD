package fyne

import (
	"context"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

type launchPipelineFunc func(ctx context.Context, profile domain.Profile, updateStatus func(string), onProgress platform.ProgressFunc, onError func())

// startLaunch runs the launch pipeline for the profile at idx, owning the
// cross-view state: the guard, cancel, status and completion live here because
// a launch outlives the view that starts it. onStatus and onProgress render
// into the starting view and may be nil; status also reaches whichever profile
// view is current. Returns false when a launch is already running.
func (um *UIManager) startLaunch(idx int, onStatus func(string), onProgress platform.ProgressFunc) bool {
	if um.launchInProgress || idx < 0 || idx >= len(um.Config.Profiles) {
		return false
	}
	um.launchInProgress = true
	um.launchProfileIdx = idx
	profile := um.Config.Profiles[idx]

	ctx, cancel := context.WithCancel(context.Background())
	um.launchCancel = cancel

	emit := func(status string) {
		um.publishLaunchStatus(status)
		if onStatus != nil {
			onStatus(status)
		}
	}
	emit("Starting " + profile.Name)

	pipeline := um.launchPipeline
	if pipeline == nil {
		pipeline = um.launchProfile
	}
	failed := false
	um.startAsync(func() {
		defer fyne.Do(func() {
			um.launchInProgress = false
			cancel()
			um.launchCancel = nil
			um.hideLaunchCancel() // a log view opened over this launch has its own Cancel showing
			um.finishLaunch(failed, profile.Name)
		})
		pipeline(ctx, profile,
			func(status string) { fyne.Do(func() { emit(status) }) },
			func(done, total int64) {
				// Extraction is a synthesized status, fanned out like the rest;
				// it is also the point past which cancelling stops being offered.
				if total > 0 && done >= total {
					fyne.Do(func() {
						emit("Extracting (this can take a moment for large installs)")
						um.hideLaunchCancel()
					})
				}
				if onProgress != nil {
					onProgress(done, total)
				}
			},
			func() { failed = true },
		)
	})
	return true
}

// publishLaunchStatus records a launch's progress text and shows it on the
// current view. A launch survives a view rebuild (saving an edit, or leaving and
// returning from the library), so it cannot write to the widgets of the view
// that started it.
func (um *UIManager) publishLaunchStatus(status string) {
	um.launchStatus = status
	if um.mainView != nil {
		um.mainView.launchPhase.SetText(status)
	}
}

// finishLaunch settles the band and buttons on the current view once a launch
// ends, for the same reason publishLaunchStatus does not use captured widgets.
func (um *UIManager) finishLaunch(failed bool, profileName string) {
	um.launchStatus = ""
	if um.mainView != nil {
		um.mainView.finishLaunch(failed, profileName)
	}
}

// launchIndex starts the profile at idx, honoring the AutoOpenLog setting:
// either open the log view or launch in the background with toast + button
// feedback. Used by the Run button, Enter, and the digit quick-launch keys so
// all three behave identically.
func (mv *mainView) launchIndex(idx int) {
	um := mv.um
	if idx < 0 || idx >= len(um.Config.Profiles) {
		um.showError("select a profile to launch")
		return
	}
	if um.Config.AutoOpenLog {
		um.showLogView(idx)
		return
	}
	// um.launchInProgress is the cross-path guard (also checked by showLogView);
	// mv.launchInProgress mirrors it for this view's own button/band state, since
	// AutoOpenLog can be toggled mid-launch, switching which path a later Run
	// takes without the launch itself having finished.
	if um.launchInProgress {
		return // the visible band is the feedback; a toast overlay would block Cancel for 3s
	}
	mv.launchInProgress = true
	mv.launchGen++
	mv.launchLogsIdx = idx

	// Reset the band to a fresh "working" state (marquee until download starts)
	// before startLaunch publishes its first status into the phase label.
	mv.launchLogsBtn.Hide()
	mv.launchBar.Hide()
	mv.launchSpin.Show()
	mv.launchPhase.Importance = widget.MediumImportance
	mv.launchPhase.TextStyle = fyne.TextStyle{}

	lastPct := -1
	um.startLaunch(idx, nil, func(done, total int64) {
		if total <= 0 {
			return // unknown size: stay on the marquee
		}
		if done >= total {
			fyne.Do(func() {
				mv.launchBar.Hide()
				mv.launchSpin.Show()
				mv.cancelBtn.Hide() // extraction isn't cancellable; stop offering to
			})
			return
		}
		pct := int(done * 100 / total)
		if pct == lastPct {
			return // throttle to whole-percent steps
		}
		lastPct = pct
		fyne.Do(func() {
			mv.launchSpin.Hide()
			mv.launchBar.Show()
			mv.launchBar.SetValue(float64(done) / float64(total))
		})
	})

	mv.launchBand.Show()
	mv.launchBand.Refresh()
	mv.runBtn.Disable()
	if um.launchCancel != nil {
		mv.launchCancel = um.launchCancel
		mv.cancelBtn.Show()
	}
}

// finishLaunch settles the band and buttons once a launch ends. It runs on
// whichever view is on screen, which is not always the one that started the
// launch: saving an edit or returning from the library rebuilds the view while
// the launch goroutine carries on.
func (mv *mainView) finishLaunch(failed bool, profileName string) {
	um := mv.um
	mv.launchInProgress = false
	mv.launchCancel = nil
	mv.cancelBtn.Hide()
	mv.launchSpin.Hide()
	mv.launchBar.Hide()
	mv.runBtn.Enable()
	mv.updateButtonStates()
	if failed {
		// A cancel arrives on the same channel as a failure, but the user asked for
		// it: red and an invitation to read the logs would be alarming.
		if !strings.HasPrefix(mv.launchPhase.Text, "Cancelled") {
			mv.launchPhase.Importance = widget.DangerImportance
			mv.launchLogsBtn.Show()
		}
		mv.launchPhase.SetText(strings.TrimPrefix(mv.launchPhase.Text, "Failed: "))
		mv.launchPhase.Refresh()
		return
	}
	mv.launchPhase.Importance = widget.MediumImportance
	mv.launchPhase.TextStyle = fyne.TextStyle{Bold: true}
	mv.launchPhase.SetText("Launched " + profileName)
	mv.launchPhase.Refresh()
	if um.profileListRefresh != nil {
		um.profileListRefresh() // launchProfile already invalidated the disk cache on a fresh download
	}
	gen := mv.launchGen
	go func() {
		time.Sleep(6000 * time.Millisecond)
		fyne.Do(func() {
			if mv.shouldAutoHide(gen) {
				mv.launchBand.Hide()
			}
		})
	}()
}

// adoptLaunch shows an already-running launch in a freshly built view, so the
// band, its Cancel and the disabled Run all reflect a launch this view did not
// start. Without it a rebuilt view offers an enabled Run that silently does
// nothing, and no way to cancel the download.
func (mv *mainView) adoptLaunch() {
	um := mv.um
	mv.launchInProgress = true
	mv.launchLogsIdx = um.launchProfileIdx
	mv.launchPhase.Importance = widget.MediumImportance
	mv.launchPhase.SetText(um.launchStatus)
	// Match the state launchIndex sets up: marquee until this view sees progress
	// of its own, which it will not, since the download reports to the old one.
	mv.launchLogsBtn.Hide()
	mv.launchBar.Hide()
	mv.launchSpin.Show()
	mv.launchBand.Show()
	if um.launchCancel != nil {
		mv.launchCancel = um.launchCancel
		mv.cancelBtn.Show()
	}
	mv.runBtn.Disable()
}

// shouldAutoHide reports whether a success auto-hide timer started for gen may
// still hide the band: only if no newer launch has bumped launchGen and none is
// in flight.
func (mv *mainView) shouldAutoHide(gen int) bool {
	return gen == mv.launchGen && !mv.launchInProgress
}
