package fyne

import (
	"context"
	_ "embed"
	"fmt"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

//go:embed app_icon.png
var appIconBytes []byte

// UIManager manages the Fyne GUI application, window states, caching, and services
type UIManager struct {
	App                   fyne.App
	Window                fyne.Window
	Config                *domain.Config
	Defaults              *domain.Config // factory defaults for Reset (set by cmd/runttd)
	Logger                *platform.Logger
	ConfigPath            string
	Version               string
	updateChecked         bool   // true once the GitHub update check has resolved
	updateTag             string // newer release tag, "" if none/unknown
	updateURL             string // newer release page URL
	SelectedProfileName   string
	LastListSelectID      int
	LastListSelectAt      time.Time
	pendingLaunchIdx      int
	suppressAutoCloseOnce bool // Skip auto-close for the one startup auto-launch
	quit                  func()
	CachedVersions        []string
	upstream              *upstreamCache
	setupIssues           *setupIssueCache
	diskLookups           *diskLookupCache
	runAsync              func(func())         // how the three async caches spawn work; nil means a fresh goroutine (see startAsync)
	profileListRefresh    func()               // set by the main view; refreshes the profile list from any goroutine (via fyne.Do)
	detailsRefresh        func()               // set by the main view; rebuilds the details pane from any goroutine (via fyne.Do)
	settingsOverlay       *widget.PopUp        // the open settings dialog, for the scoped Escape handler; nil when closed
	settingsOnEscape      func()               // Escape on the settings overlay routes here (dirty -> discard-confirm)
	blockingConfirm       *widget.PopUp        // a confirm whose caller blocks on its response; raw overlay.Hide() would skip the callback and hang forever
	blockingConfirmHide   func()               // resolves blockingConfirm via the dialog's own Hide(), so Escape still answers "No"
	confirmAction         func()               // the open confirm's Confirm(); Fyne's dialog buttons ignore Enter and take no focus
	editorOverlay         *widget.PopUp        // the open profile editor, for the scoped Escape handler; nil when closed
	editorOnEscape        func()               // Escape on the editor overlay routes here (dirty -> discard-confirm)
	shortcutOverlay       *widget.PopUp        // the open create-shortcut dialog; nil when closed
	shortcutOnEscape      func()               // Escape on the shortcut overlay routes here, clearing the handles above
	libraryRescan         func()               // set by showLibraryView while it's the active view; the F5 accelerator's target, nil (no-op) elsewhere
	viewEscape            func()               // Escape's target for the view on screen: Back in the library/log views, clear-the-search in the profile view
	launchInProgress      bool                 // the cross-path launch guard; mainView.launchInProgress is a per-view mirror
	launchCancel          func()               // cancels the in-flight launch's download context, if any; nil once no launch is running or the download step has already finished
	launchCancelBtn       *dialogButton        // the log view Cancel button currently on screen; a reopened view replaces it
	launchStatus          string               // the in-flight launch's latest status line, so a view built mid-launch can show where it got to
	launchProfileIdx      int                  // the profile an in-flight launch is for, so an adopting view can point View logs at it
	mainView              *mainView            // the newest profile view; a launch outlives the view that began it, so it reports here rather than into captured widgets
	viewKeys              func(*fyne.KeyEvent) // the profile view's bare-key handler, so a focused button can hand back what it did not use
	launchPipeline        launchPipelineFunc   // tests substitute the network-bound pipeline; nil means um.launchProfile
}

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

// snapshotConfig returns a copy for background goroutines to read: settings
// and editor saves write *um.Config on the UI thread mid-flight, and it has no
// mutex. Profiles is copied too, since an editor save writes elements of the
// shared backing array; Profile itself is all value types, so this is deep.
func (um *UIManager) snapshotConfig() *domain.Config {
	cfg := *um.Config
	cfg.Profiles = append([]domain.Profile(nil), um.Config.Profiles...)
	return &cfg
}

// routeViewKey passes a key a focused widget declined on to the view's own
// handler, which is otherwise only reachable when nothing has focus.
func (um *UIManager) routeViewKey(key *fyne.KeyEvent) {
	if um.viewKeys != nil {
		um.viewKeys(key)
	}
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

// NewUIManager creates a new UIManager instance, configuring the static app icons and custom presets theme
func NewUIManager(config *domain.Config, configPath string, version string) *UIManager {
	fyneApp := app.New()
	appIcon := fyne.NewStaticResource("app_icon.png", appIconBytes)
	fyneApp.SetIcon(appIcon)
	window := fyneApp.NewWindow("RunTTD")
	window.SetIcon(appIcon)
	w, h := config.WindowSize()
	window.Resize(fyne.NewSize(w, h))

	um := &UIManager{
		App:              fyneApp,
		Window:           window,
		Config:           config,
		Logger:           platform.NewLogger(config.LogToFile, filepath.Join(filepath.Dir(configPath), "log.txt")),
		ConfigPath:       configPath,
		Version:          version,
		LastListSelectID: -1,
		pendingLaunchIdx: -1,
		upstream:         newUpstreamCache(),
		setupIssues:      newSetupIssueCache(),
		diskLookups:      newDiskLookupCache(),
	}
	um.Logger.Append("RunTTD " + versionCaption(version) + " starting")

	if um.Config.ThemeVariant == "" {
		um.Config.ThemeVariant = "dark"
	}
	if um.Config.AccentPreset < 0 || um.Config.AccentPreset >= len(ThemePresets) {
		um.Config.AccentPreset = 0
	}

	preset := ThemePresets[um.Config.AccentPreset]
	light, _ := ParseHexColor(preset.LightHex)
	dark, _ := ParseHexColor(preset.DarkHex)

	pt := &LauncherTheme{
		Theme:       theme.DefaultTheme(),
		AccentDark:  dark,
		AccentLight: light,
	}
	switch um.Config.ThemeVariant {
	case "light":
		v := theme.VariantLight
		pt.OverrideVariant = &v
	case "dark":
		v := theme.VariantDark
		pt.OverrideVariant = &v
	}

	um.quit = um.App.Quit
	um.App.Settings().SetTheme(pt)

	// Persist the window size on close so the next launch reopens at it. Canvas
	// size is the readable content size; the WindowSize floor guards a bad value.
	window.SetOnClosed(func() {
		sz := window.Canvas().Size()
		um.persistWindowSize(int(sz.Width), int(sz.Height))
	})

	return um
}

// persistWindowSize stores the window size for the next launch. It writes nothing
// while onboarding is unfinished, since creating the config there would make the
// next launch skip setup.
func (um *UIManager) persistWindowSize(w, h int) {
	if um.Config.FirstRun {
		return
	}
	um.Config.WindowWidth = w
	um.Config.WindowHeight = h
	// Log-only: the window is already closing, so a dialog here has no one to show it to.
	if err := domain.SaveConfig(um.ConfigPath, um.Config); err != nil {
		um.Logger.Append(fmt.Sprintf("could not save window size: %v", err))
	}
}

// startAsync dispatches fn through runAsync, or a fresh goroutine when it is nil.
// Tests substitute a runner, because the Fyne test driver executes fyne.Do inline
// on the calling goroutine, so background work would mutate the widget tree mid-render.
func (um *UIManager) startAsync(fn func()) {
	if um.runAsync != nil {
		um.runAsync(fn)
		return
	}
	go fn()
}

// LogVerbose appends a message only if verbose logging is enabled in settings
func (um *UIManager) LogVerbose(msg string) {
	if um.Config.Verbose {
		um.Logger.Append(msg)
	}
}

// LogImportant appends critical status messages directly to operational logs
func (um *UIManager) LogImportant(msg string) {
	um.Logger.Append(msg)
}

// ArmLaunchProfile queues the named profile to launch as soon as the main view
// appears. suppressAutoClose keeps the window open afterwards, which the startup
// setting needs (closing instantly would lock the user out of the launcher) but a
// deliberate --profile run does not.
func (um *UIManager) ArmLaunchProfile(name string, suppressAutoClose bool) {
	for i, p := range um.Config.Profiles {
		if p.Name == name {
			um.pendingLaunchIdx = i
			um.suppressAutoCloseOnce = suppressAutoClose
			return
		}
	}
}

// armAutoLaunch queues the configured startup profile through pendingLaunchIdx and suppresses auto-close for that one launch.
func (um *UIManager) armAutoLaunch() {
	// An already-armed launch is a --profile request, which outranks the setting.
	if um.Config.AutoLaunchProfile == "" || um.pendingLaunchIdx >= 0 {
		return
	}
	um.ArmLaunchProfile(um.Config.AutoLaunchProfile, true)
}

// Show renders either the first-run onboarding screen or standard profile panel depending on loaded configs
func (um *UIManager) Show() {
	if um.Config.FirstRun {
		um.Window.SetContent(um.makeOnboardingView())
	} else {
		um.armAutoLaunch()
		um.Window.SetContent(um.makeMainView())
	}
	um.Window.ShowAndRun()
}

// OnOpenTTDStarted terminates the launcher UI if auto-close is configured, unless suppressed for the startup auto-launch.
func (um *UIManager) OnOpenTTDStarted() {
	if um.suppressAutoCloseOnce {
		um.suppressAutoCloseOnce = false
		return
	}
	if um.Config.AutoCloseOnStart {
		um.quit()
	}
}
