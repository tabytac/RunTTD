package fyne

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
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

// showLogView shows a screen with live logs while launching a profile or in standalone mode
func (um *UIManager) showLogView(profileIdx int) {
	var profile domain.Profile
	isLaunch := profileIdx >= 0
	if isLaunch {
		profile = um.Config.Profiles[profileIdx]
	}

	statusBinding := binding.NewString()
	_ = statusBinding.Set("Preparing launch")

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
	// write, and the scroll — leaving the user free to scroll up without being
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

	closeBtn := widget.NewButton("Back to Profiles", func() {
		select {
		case <-done:
		default:
			close(done)
		}
		um.Window.SetContent(um.makeMainView())
	})

	copyBtn := widget.NewButtonWithIcon("Copy to Clipboard", theme.ContentCopyIcon(), func() {
		um.App.Clipboard().SetContent(strings.Join(um.Logger.GetAll(), "\n"))
		um.showToast("Logs copied to clipboard!")
	})

	clearBtn := widget.NewButtonWithIcon("Clear Logs", theme.ContentClearIcon(), func() {
		dialog.ShowConfirm("Clear logs?",
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
			}, um.Window)
	})

	cancelBtn := widget.NewButton("Cancel", func() {
		if um.launchCancel != nil {
			um.launchCancel()
		}
	})
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

	um.Window.SetContent(content)

	// Launch OpenTTD in background if requested. Guarded at the UIManager level
	// (not mainView.launchInProgress, which this path bypasses entirely) so
	// leaving this view mid-download and pressing Run again can't start a second
	// concurrent download into the same folder.
	if isLaunch {
		if um.launchInProgress {
			// The original launch is still running (this is a fresh showLogView,
			// e.g. after "Back to Profiles" then Run again); um.launchCancel still
			// targets it, so offer Cancel here too even though this view's own
			// status/progress aren't wired to that goroutine.
			um.showToast("A launch is already in progress")
			cancelBtn.Show()
		} else {
			um.launchInProgress = true
			ctx, cancel := context.WithCancel(context.Background())
			um.launchCancel = cancel
			cancelBtn.Show()
			lastPct := -1
			go func() {
				defer fyne.Do(func() {
					um.launchInProgress = false
					cancel()
					um.launchCancel = nil
					cancelBtn.Hide()
				})
				um.launchProfile(ctx, profile, func(status string) {
					// binding.String.Set is documented safe from any goroutine.
					_ = statusBinding.Set(status)
				}, func(done, total int64) {
					if total <= 0 {
						return // unknown size: leave the current status text alone
					}
					if done >= total {
						fyne.Do(cancelBtn.Hide) // extraction isn't cancellable; stop offering to
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

// reportLaunchResult surfaces whether ExecuteOpenTTD actually started the game,
// instead of the previous unconditional "Launch command sent" regardless of outcome.
func reportLaunchResult(started bool, updateStatus func(string), onError func()) {
	if started {
		if updateStatus != nil {
			updateStatus("Launch command sent")
		}
		return
	}
	if updateStatus != nil {
		updateStatus("Failed: OpenTTD did not start")
	}
	if onError != nil {
		onError()
	}
}

// launchProfile launches OpenTTD with the specified profile configuration and
// logging observers. ctx governs only the version-check and download/extract
// steps below (cancelling it is how the Cancel button works) — every
// ExecuteOpenTTD call site deliberately uses context.Background() instead, since
// cancelling after the game has started would kill it via exec.CommandContext.
func (um *UIManager) launchProfile(ctx context.Context, profile domain.Profile, updateStatus func(status string), onProgress platform.ProgressFunc, onError func()) {
	if updateStatus != nil {
		updateStatus("Resolving profile and version")
	}
	um.LogImportant(fmt.Sprintf("Launching profile %q", profile.Name))
	um.LogVerbose(fmt.Sprintf("Profile config: version=%q savePath=%q server=%q company=%q", profile.Version, profile.SavePath, profile.ServerIpPort, profile.ServerCompanyNumber))

	requested := strings.TrimSpace(profile.Version)
	version := requested
	client := apppkg.EffectiveClient(profile.Client, um.Config.DefaultClient)

	if client == "custom" {
		folder := strings.TrimSpace(profile.CustomExecutablePath)
		if folder == "" {
			if updateStatus != nil {
				updateStatus("Failed: custom executable folder is not set")
			}
			um.LogImportant("Custom client selected but no executable folder is configured.")
			if onError != nil {
				onError()
			}
			return
		}
		if _, err := os.Stat(folder); err != nil {
			if updateStatus != nil {
				updateStatus("Failed: custom executable folder does not exist")
			}
			um.LogImportant(fmt.Sprintf("Custom executable folder not found: %s (%v)", folder, err))
			if onError != nil {
				onError()
			}
			return
		}
		um.LogVerbose(fmt.Sprintf("Using custom executable folder: %s", folder))
		if updateStatus != nil {
			updateStatus("Starting OpenTTD from custom folder")
		}
		// context.Background() here (not a cancellable ctx) is deliberate: once
		// ExecuteOpenTTD's exec.CommandContext starts the game, cancelling would kill it.
		started := platform.ExecuteOpenTTD(context.Background(), folder, profile, um.Config.DocsBasePath, apppkg.ClientSupportsCompanyPassword(client), um)
		reportLaunchResult(started, updateStatus, onError)
		return
	}

	isLatestRequest := apppkg.IsLatestVersion(requested)
	latestTrack := apppkg.LatestTrack(client, requested)

	if isLatestRequest {
		if updateStatus != nil {
			updateStatus(fmt.Sprintf("Resolving latest %s version (%s)", client, latestTrack))
		}
		um.LogImportant(fmt.Sprintf("Resolving latest %s version (%s)", client, latestTrack))
		version = platform.CheckForNewVersionForClientTrack(ctx, client, um.Config, latestTrack)
		if errors.Is(ctx.Err(), context.Canceled) {
			// A cancel here must abort the whole launch, not fall back to
			// launching whatever's already installed — that would be Cancel
			// silently not cancelling, and the launch band would show a
			// dishonest "Launched" straight after the user clicked Cancel.
			um.LogImportant("Cancelled: version check was cancelled by the user.")
			if updateStatus != nil {
				updateStatus("Cancelled")
			}
			if onError != nil {
				onError()
			}
			return
		}
		if version == "" {
			// The remote lookup failed or returned nothing — typically because the
			// download server is unreachable (offline). Skip the update check and
			// fall back to launching the newest install already on disk.
			um.LogImportant("Could not reach the download server to check for a newer version; falling back to the latest local install.")
			if updateStatus != nil {
				updateStatus("Update check unavailable (offline?), using latest local install")
			}
			// Highest-version install on this track (matches the online launch target
			// and the status dot); NOT newest-by-mod-time, which a re-downloaded older build wins.
			versionFolder := apppkg.HighestInstalledFolderInRoot(um.Config, client, latestTrack)
			if versionFolder == "" {
				if updateStatus != nil {
					updateStatus("Failed: offline and no local installation found for client")
				}
				um.LogImportant("No local installation found for client, and the download server could not be reached.")
				if onError != nil {
					onError()
				}
				return
			}
			um.LogVerbose(fmt.Sprintf("Using latest local version folder: %s", versionFolder))
			if updateStatus != nil {
				updateStatus("Starting OpenTTD from latest local installation")
			}
			started := platform.ExecuteOpenTTD(context.Background(), versionFolder, profile, um.Config.DocsBasePath, apppkg.ClientSupportsCompanyPassword(client), um)
			reportLaunchResult(started, updateStatus, onError)
			return
		}
	}
	if requested != "" && !isLatestRequest {
		if updateStatus != nil {
			updateStatus(fmt.Sprintf("Using requested %s version %s", client, version))
		}
		um.LogImportant(fmt.Sprintf("Using requested %s version %s", client, version))
	}

	if updateStatus != nil {
		updateStatus("Looking for local version folder")
	}
	versionFolder := platform.FindVersionFolderClient(platform.ClientDownloadDir(um.Config, client), version, client, um.Config)
	if versionFolder == "" {
		if updateStatus != nil {
			updateStatus("Version not found locally, downloading")
		}
		um.LogImportant(fmt.Sprintf("Version %s not found locally. Attempting to download for client %s.", version, client))
		// Block until user confirms before downloading a pre-1.2.0 vanilla build (no bundled graphics).
		if (client == "vanilla" || client == "vanilla-nightly") && platform.VanillaNeedsBaseSetWarning(version) {
			proceed := make(chan bool, 1)
			msg := fmt.Sprintf("OpenTTD %s needs manual setup before it will run through RunTTD. "+
				"Versions before 1.2.0 don't include free graphics, so you'll need original "+
				"Transport Tycoon Deluxe data files to play. Some old releases also predate builds for many systems.", version)
			fyne.Do(func() {
				confirm := dialog.NewConfirm("Very old OpenTTD version", msg, func(ok bool) {
					um.blockingConfirm, um.blockingConfirmHide = nil, nil
					proceed <- ok
				}, um.Window)
				confirm.Show()
				// Escape reaches only the raw overlay, whose Hide() skips this callback;
				// route it through the dialog's own Hide() so proceed always gets a value.
				if top, ok := um.Window.Canvas().Overlays().Top().(*widget.PopUp); ok {
					um.blockingConfirm = top
					um.blockingConfirmHide = confirm.Hide
				}
			})
			if !<-proceed {
				um.LogImportant(fmt.Sprintf("Cancelled: %s needs manual setup before it can run.", version))
				if updateStatus != nil {
					updateStatus("Cancelled (version needs manual setup)")
				}
				if onError != nil {
					onError()
				}
				return
			}
		}
		if !platform.DownloadAndExtractVersionForClientWithLogger(ctx, version, client, um.Config, um.Logger, onProgress) {
			if errors.Is(ctx.Err(), context.Canceled) {
				if updateStatus != nil {
					updateStatus("Cancelled")
				}
				um.LogImportant(fmt.Sprintf("Cancelled: download of version %s was cancelled by the user.", version))
			} else {
				if updateStatus != nil {
					updateStatus(fmt.Sprintf("Failed: download of version %s did not complete", version))
				}
				um.LogImportant(fmt.Sprintf("Failed to download version %s for client %s.", version, client))
			}
			if onError != nil {
				onError()
			}
			return
		}
		// A new folder landed on disk; the dot cache's answers for this client are
		// now stale regardless of which view (or none) is showing when this completes.
		um.diskLookups.invalidate()
		if updateStatus != nil {
			updateStatus("Download complete, resolving extracted folder")
		}
		versionFolder = platform.FindVersionFolderClient(platform.ClientDownloadDir(um.Config, client), version, client, um.Config)
		if versionFolder == "" {
			if updateStatus != nil {
				updateStatus("Failed: downloaded version folder could not be located")
			}
			um.LogImportant("Failed to locate downloaded version.")
			if onError != nil {
				onError()
			}
			return
		}
	}

	// A cancel can land here with nothing left to interrupt (e.g. the version was
	// already installed locally, so the download step above never even ran) —
	// check explicitly rather than launching anyway just because nothing failed.
	if errors.Is(ctx.Err(), context.Canceled) {
		um.LogImportant("Cancelled: launch was cancelled by the user before starting OpenTTD.")
		if updateStatus != nil {
			updateStatus("Cancelled")
		}
		if onError != nil {
			onError()
		}
		return
	}

	um.LogVerbose(fmt.Sprintf("Using version folder: %s", versionFolder))
	if updateStatus != nil {
		updateStatus("Starting OpenTTD")
	}
	started := platform.ExecuteOpenTTD(context.Background(), versionFolder, profile, um.Config.DocsBasePath, apppkg.ClientSupportsCompanyPassword(client), um)
	reportLaunchResult(started, updateStatus, onError)
}
