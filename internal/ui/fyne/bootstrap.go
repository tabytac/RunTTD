package fyne

import (
	_ "embed"
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
	App                 fyne.App
	Window              fyne.Window
	Config              *domain.Config
	Defaults            *domain.Config // factory defaults for Reset (set by cmd/runttd)
	Logger              *platform.Logger
	ConfigPath          string
	Version             string
	updateChecked       bool   // true once the GitHub update check has resolved
	updateTag           string // newer release tag, "" if none/unknown
	updateURL           string // newer release page URL
	SelectedProfileName string
	LastListSelectID    int
	LastListSelectAt    time.Time
	pendingLaunchIdx      int
	suppressAutoCloseOnce bool // Skip auto-close for the one startup auto-launch
	quit                  func()
	CachedVersions        []string
	upstream            *upstreamCache
	profileListRefresh  func() // set by the main view; refreshes the profile list from any goroutine (via fyne.Do)
	settingsOverlay     *widget.PopUp // the open settings dialog, for the scoped Escape handler; nil when closed
	settingsOnEscape    func()        // Escape on the settings overlay routes here (dirty -> discard-confirm)
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
		um.Config.WindowWidth = int(sz.Width)
		um.Config.WindowHeight = int(sz.Height)
		_ = domain.SaveConfig(um.ConfigPath, um.Config)
	})

	return um
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

// armAutoLaunch queues the configured startup profile through pendingLaunchIdx and suppresses auto-close for that one launch.
func (um *UIManager) armAutoLaunch() {
	if um.Config.AutoLaunchProfile == "" {
		return
	}
	for i, p := range um.Config.Profiles {
		if p.Name == um.Config.AutoLaunchProfile {
			um.pendingLaunchIdx = i
			um.suppressAutoCloseOnce = true
			return
		}
	}
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
