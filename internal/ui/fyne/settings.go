package fyne

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// Default Client selector options shared by the onboarding screen and the
// global settings dialog. defaultClientMap is label->config value;
// revDefaultClientMap is the inverse. Default Client is a required, always-set
// value, so there is no "(none)" option.
var (
	defaultClientOptions = []string{"Vanilla OpenTTD (Releases)", "Vanilla OpenTTD (Nightly)", "JGRPP", "Custom Executable"}
	defaultClientMap     = map[string]string{"Vanilla OpenTTD (Releases)": "vanilla", "Vanilla OpenTTD (Nightly)": "vanilla-nightly", "JGRPP": "jgrpp", "Custom Executable": "custom"}
	revDefaultClientMap  = map[string]string{"vanilla": "Vanilla OpenTTD (Releases)", "vanilla-nightly": "Vanilla OpenTTD (Nightly)", "jgrpp": "JGRPP", "custom": "Custom Executable"}
)

// A stored OSType of "" means auto-detect (resolved per machine via
// platform.DefaultOSType()), so a shared config works on any PC; the other
// values are explicit overrides. osDisplayLabel supplies the friendly labels.
const osAutoLabelPrefix = "Auto-detect"

var osTypeValues = []string{
	"windows-win64", "windows-win32", "windows-arm64",
	"linux-generic-amd64", "linux-generic-arm64", "linux-generic-i386",
	"macos-universal",
}

// osAutoLabel annotates the auto-detect option with what this PC resolves to.
func osAutoLabel(detected string) string {
	return fmt.Sprintf("%s (this PC: %s)", osAutoLabelPrefix, osDisplayLabel(detected))
}

// osTypeOptions returns the dropdown labels: auto-detect, the explicit values,
// and any non-empty stored tag that is unknown (preserved so it is never lost).
func osTypeOptions(detected, stored string) []string {
	opts := []string{osAutoLabel(detected)}
	for _, v := range osTypeValues {
		opts = append(opts, osDisplayLabel(v))
	}
	if stored != "" && !knownOSType(stored) {
		opts = append(opts, stored)
	}
	return opts
}

func knownOSType(value string) bool {
	for _, v := range osTypeValues {
		if v == value {
			return true
		}
	}
	return false
}

// osTypeValueToLabel maps a stored OSType to its dropdown label ("" -> auto).
func osTypeValueToLabel(detected, value string) string {
	if value == "" {
		return osAutoLabel(detected)
	}
	if knownOSType(value) {
		return osDisplayLabel(value)
	}
	return value
}

// osTypeLabelToValue maps a selected label back to the stored OSType (auto -> "").
func osTypeLabelToValue(detected, label string) string {
	if label == osAutoLabel(detected) || strings.HasPrefix(label, osAutoLabelPrefix) {
		return ""
	}
	for _, v := range osTypeValues {
		if osDisplayLabel(v) == label {
			return v
		}
	}
	return label
}

const autoLaunchOffLabel = "Off (don't auto-launch)"

// autoLaunchOptions builds the Behavior-tab dropdown options. Index 0 is the
// "off" sentinel; each profile is prefixed with its 1-based position so a
// profile named exactly like the sentinel can't collide with it (Fyne's Select keys purely on the option string).
func autoLaunchOptions(profiles []domain.Profile) ([]string, map[string]int) {
	opts := []string{autoLaunchOffLabel}
	labelToIdx := make(map[string]int, len(profiles))
	for i, p := range profiles {
		label := fmt.Sprintf("%d. %s", i+1, p.Name)
		opts = append(opts, label)
		labelToIdx[label] = i
	}
	return opts, labelToIdx
}

// autoLaunchSelectedLabel returns the decorated label for the stored profile
// name, or the off sentinel when stored is empty or no longer matches.
func autoLaunchSelectedLabel(profiles []domain.Profile, stored string) string {
	if stored == "" {
		return autoLaunchOffLabel
	}
	for i, p := range profiles {
		if p.Name == stored {
			return fmt.Sprintf("%d. %s", i+1, p.Name)
		}
	}
	return autoLaunchOffLabel
}

// buildAppearanceTab builds the settings Appearance pane, reusing the theme/accent
// widgets from showThemeCustomizer. Both routes call um.applyAppearance so they can't drift;
// changes persist on click and are excluded from the dialog's dirty-state.
func (um *UIManager) buildAppearanceTab() fyne.CanvasObject {
	var currentMode string
	if um.Config.ThemeVariant == "light" {
		currentMode = "Light"
	} else {
		currentMode = "Dark"
	}

	modeSelect := NewSegmentedRadio([]string{"Light", "Dark"}, currentMode, func(s string) {
		um.applyAppearance(strings.ToLower(s), um.Config.AccentPreset)
	})

	colorGrid := container.NewGridWithColumns(4)
	colorButtons := make([]*canvas.Rectangle, len(ThemePresets))

	updateButtons := func() {
		for i, rect := range colorButtons {
			if i == um.Config.AccentPreset {
				rect.StrokeColor = theme.Color(theme.ColorNamePrimary)
				rect.StrokeWidth = 3
			} else {
				rect.StrokeColor = color.Transparent
				rect.StrokeWidth = 0
			}
			rect.Refresh()
		}
	}

	for i, p := range ThemePresets {
		idx := i
		hex := p.DarkHex
		if um.Config.ThemeVariant == "light" {
			hex = p.LightHex
		}
		c, _ := ParseHexColor(hex)

		rect := canvas.NewRectangle(c)
		rect.SetMinSize(fyne.NewSize(36, 36))
		rect.CornerRadius = 4
		colorButtons[idx] = rect

		btn := widget.NewButton("", func() {
			um.applyAppearance(um.Config.ThemeVariant, idx)
			updateButtons()
		})
		btn.Importance = widget.LowImportance

		colorGrid.Add(container.NewStack(rect, btn))
	}

	updateButtons()

	content := container.NewVBox(
		NewSectionHeader("Appearance"),
		NewLabeledField("Theme", "Light or dark colour scheme. Changes apply instantly.", modeSelect.Container),
		NewLabeledField("Accent Colour", "Used for highlights, buttons, and selections. Applies instantly.", colorGrid),
	)
	return container.NewBorder(nil, nil, nil, nil, container.NewVScroll(content))
}

// showSettingsView shows a dialog to edit global settings
func (um *UIManager) showSettingsView() {
	parentDirEntry := widget.NewEntry()
	parentDirEntry.SetText(um.Config.ParentDir)
	parentDirEntry.SetPlaceHolder("Folder where game files / executables will be automatically installed")

	parentDirBtn := widget.NewButton("Browse...", func() {
		um.browseDirectory(parentDirEntry, "Select Parent Directory", "Parent Directory (Settings)")
	})

	docsBasePathEntry := widget.NewEntry()
	docsBasePathEntry.SetText(um.Config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	validationIcon := widget.NewIcon(theme.CancelIcon())
	validationIcon.Hide()

	docsBasePathBtn := widget.NewButton("Browse...", func() {
		um.browseDirectory(docsBasePathEntry, "Select Docs Base Path", "Docs Base Path (Settings)")
	})

	updateDocsValidation := func(path string) {
		if path == "" {
			validationIcon.Hide()
			return
		}
		cfgPath := filepath.Join(path, "openttd.cfg")
		if _, err := os.Stat(cfgPath); err == nil {
			validationIcon.SetResource(theme.ConfirmIcon())
			validationIcon.Show()
		} else {
			validationIcon.SetResource(theme.CancelIcon())
			validationIcon.Show()
		}
	}

	docsBasePathEntry.OnChanged = func(s string) {
		updateDocsValidation(s)
	}
	updateDocsValidation(docsBasePathEntry.Text)

	jgrppApiUrlEntry := widget.NewEntry()
	jgrppApiUrlEntry.SetText(um.Config.JgrppApiUrl)
	jgrppApiUrlEntry.SetPlaceHolder("https://api.github.com/repos/JGRennison/OpenTTD-patches")

	osDetected := platform.DefaultOSType()
	osTypeSelect := widget.NewSelect(osTypeOptions(osDetected, um.Config.OSType), func(string) {})
	osTypeSelect.SetSelected(osTypeValueToLabel(osDetected, um.Config.OSType))

	vanillaMirrorEntry := widget.NewEntry()
	vanillaMirrorEntry.SetText(um.Config.VanillaMirror)
	vanillaMirrorEntry.SetPlaceHolder("https://cdn.openttd.org/openttd-releases/")

	nightlyMirrorEntry := widget.NewEntry()
	nightlyMirrorEntry.SetText(um.Config.NightlyMirror)
	nightlyMirrorEntry.SetPlaceHolder("https://cdn.openttd.org/openttd-nightlies/")

	// Default client selector. Pre-selects the configured client; an unknown or
	// empty stored value simply leaves the dropdown unselected.
	defaultClientSelect := widget.NewSelect(defaultClientOptions, func(string) {})
	if label, ok := revDefaultClientMap[um.Config.DefaultClient]; ok {
		defaultClientSelect.SetSelected(label)
	}

	autoCloseCheck, autoCloseGroup := NewLabeledCheckWithDescription(
		"Auto-close launcher when OpenTTD starts",
		"Hides the launcher once the game opens.",
		um.Config.AutoCloseOnStart,
	)

	autoOpenLogCheck, autoOpenLogGroup := NewLabeledCheckWithDescription(
		"Auto-open log panel when game starts",
		"Opens the log panel so you can watch download/start progress.",
		um.Config.AutoOpenLog,
	)

	verboseCheck, verboseGroup := NewLabeledCheckWithDescription(
		"Verbose logging (show all messages)",
		"Includes debug-level messages in the log panel. Useful for troubleshooting.",
		um.Config.Verbose,
	)

	autoLaunchOpts, autoLaunchLabelToIdx := autoLaunchOptions(um.Config.Profiles)
	autoLaunchSelect := widget.NewSelect(autoLaunchOpts, func(string) {})
	autoLaunchSelect.SetSelected(autoLaunchSelectedLabel(um.Config.Profiles, um.Config.AutoLaunchProfile))

	subfolderCheck, subfolderGroup := NewLabeledCheckWithDescription(
		"Organize downloaded clients into per-client subfolders",
		"Keeps each client's downloaded files in a separate folder, instead of all sharing the parent folder. "+
			"If you change this later, anything already downloaded gets fetched again.",
		um.Config.SubfolderPerClient,
	)

	// Files & Storage: the install/save anchors plus the subfolder layout switch.
	filesContent := container.NewVBox(
		NewSectionHeader("Installation Paths"),
		NewLabeledField("Parent Directory (required)",
			"RunTTD downloads, installs, and removes game clients here.",
			container.NewBorder(nil, nil, nil, parentDirBtn, parentDirEntry)),
		NewLabeledField("Docs Base Path (required)",
			"Where your saves and configuration (openttd.cfg) live. RunTTD reads from here but never modifies your files.",
			container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, docsBasePathBtn), docsBasePathEntry)),
		subfolderGroup,
	)
	// Scroll as the Border center so its 32px MinSize can't balloon the modal.
	filesBody := container.NewBorder(nil, nil, nil, nil, container.NewVScroll(filesContent))
	filesTab := container.NewTabItemWithIcon("Files & Storage", theme.FolderIcon(), filesBody)

	// Launching: the new-profile default plus everything that happens at startup.
	launchingContent := container.NewVBox(
		NewSectionHeader("Profile Defaults"),
		NewLabeledField("Default Client (new profiles)",
			"The client new profiles use by default. Change it per profile.",
			defaultClientSelect),
		NewSectionHeader("Launch Behavior"),
		NewLabeledField("Auto-launch profile on startup",
			"Launches the chosen profile when RunTTD opens. The launcher stays open for this one startup launch even if auto-close is on.",
			autoLaunchSelect),
		autoCloseGroup,
		autoOpenLogGroup,
		verboseGroup,
	)
	launchingBody := container.NewBorder(nil, nil, nil, nil, container.NewVScroll(launchingContent))
	launchingTab := container.NewTabItemWithIcon("Launching", theme.ConfirmIcon(), launchingBody)

	appearanceTab := container.NewTabItemWithIcon("Appearance", theme.ColorPaletteIcon(), um.buildAppearanceTab())

	// Network & System: expert-only download overrides + the build target.
	networkContent := container.NewVBox(
		NewSectionHeader("Download Sources"),
		NewLabeledField("Vanilla CDN (stable) base URL", "Where stable releases are fetched from.", vanillaMirrorEntry),
		NewLabeledField("Vanilla Nightly CDN base URL", "Where nightly builds are fetched from.", nightlyMirrorEntry),
		NewLabeledField("JGRPP GitHub API URL", "Where JGR's Patchpack releases are looked up.", jgrppApiUrlEntry),
		NewSectionHeader("System"),
		NewLabeledField("OS / Build Target",
			"Auto-detect is recommended and makes a shared config work on any PC. Override only to fetch builds for a different system.",
			osTypeSelect),
	)
	networkBody := container.NewBorder(nil, nil, nil, nil, container.NewVScroll(networkContent))
	networkTab := container.NewTabItemWithIcon("Network & System", theme.SettingsIcon(), networkBody)

	tabs := container.NewAppTabs(filesTab, launchingTab, appearanceTab, networkTab)
	tabs.SetTabLocation(container.TabLocationTop)

	var settingsDialog *widget.PopUp

	saveBtn := widget.NewButton("Save Settings", func() {
		prevSubfolderPerClient := um.Config.SubfolderPerClient

		um.Config.ParentDir = parentDirEntry.Text
		um.Config.DocsBasePath = docsBasePathEntry.Text
		um.Config.JgrppApiUrl = jgrppApiUrlEntry.Text
		um.Config.OSType = osTypeLabelToValue(osDetected, osTypeSelect.Selected)
		um.Config.AutoCloseOnStart = autoCloseCheck.Checked
		um.Config.AutoOpenLog = autoOpenLogCheck.Checked
		um.Config.Verbose = verboseCheck.Checked
		um.Config.SubfolderPerClient = subfolderCheck.Checked
		um.Config.VanillaMirror = vanillaMirrorEntry.Text
		um.Config.NightlyMirror = nightlyMirrorEntry.Text

		if sel := defaultClientSelect.Selected; sel != "" {
			if mapped, ok := defaultClientMap[sel]; ok {
				um.Config.DefaultClient = mapped
			}
		}

		if sel := autoLaunchSelect.Selected; sel == autoLaunchOffLabel || sel == "" {
			um.Config.AutoLaunchProfile = ""
		} else if idx, ok := autoLaunchLabelToIdx[sel]; ok {
			um.Config.AutoLaunchProfile = um.Config.Profiles[idx].Name
		}

		_ = domain.SaveConfig(um.ConfigPath, um.Config)
		if settingsDialog != nil {
			settingsDialog.Hide()
		}

		if um.Config.SubfolderPerClient != prevSubfolderPerClient {
			dialog.ShowInformation(
				"Install location changed",
				"The launcher will now look for clients in the new location and may re-download them. Existing downloads are left in place — move or delete them manually if you no longer need them.",
				um.Window,
			)
		}
	})

	cancelBtn := widget.NewButton("Cancel", func() {
		settingsDialog.Hide()
	})

	settingsDialog = NewModalDialog(um.Window.Canvas(), "Global Settings", tabs, cancelBtn, saveBtn)
	// ModalPopUp isn't drag-resizable; this is the initial size, kept honest by the Border-centred scrolls.
	settingsDialog.Resize(fyne.NewSize(760, 560))
	settingsDialog.Show()
}
