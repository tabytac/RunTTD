package fyne

import (
	_ "embed"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"

	apppkg "runttd/internal/app"
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
	Logger              *platform.Logger
	ConfigPath          string
	Version             string
	SelectedProfileName string
	LastListSelectID    int
	LastListSelectAt    time.Time
	CachedVersions      []string

	LauncherService *apppkg.LauncherService
}

// NewUIManager creates a new UIManager instance, configuring the static app icons and custom presets theme
func NewUIManager(config *domain.Config, configPath string, version string) *UIManager {
	fyneApp := app.New()
	appIcon := fyne.NewStaticResource("app_icon.png", appIconBytes)
	fyneApp.SetIcon(appIcon)
	window := fyneApp.NewWindow("RunTTD")
	window.SetIcon(appIcon)
	window.Resize(fyne.NewSize(1024, 768))

	um := &UIManager{
		App:              fyneApp,
		Window:           window,
		Config:           config,
		Logger:           platform.NewLogger(config.LogToFile, filepath.Join(filepath.Dir(configPath), "log.txt")),
		ConfigPath:       configPath,
		Version:          version,
		LastListSelectID: -1,
		LauncherService:  apppkg.NewLauncherService(),
	}

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
	if um.Config.ThemeVariant == "light" {
		v := theme.VariantLight
		pt.OverrideVariant = &v
	} else if um.Config.ThemeVariant == "dark" {
		v := theme.VariantDark
		pt.OverrideVariant = &v
	}

	um.App.Settings().SetTheme(pt)

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

// Show renders either the first-run onboarding screen or standard profile panel depending on loaded configs
func (um *UIManager) Show() {
	if um.Config.FirstRun {
		um.Window.SetContent(um.makeOnboardingView())
	} else {
		um.Window.SetContent(um.makeMainView())
	}
	um.Window.ShowAndRun()
}

// OnOpenTTDStarted terminates the launcher UI if auto-close is configured on startup
func (um *UIManager) OnOpenTTDStarted() {
	if um.Config.AutoCloseOnStart {
		um.App.Quit()
	}
}
