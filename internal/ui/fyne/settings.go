package fyne

import (
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

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

type mirrorField struct{ name, preSave, stored string }

// mirrorResetDiff returns the display names of URL fields whose non-empty pre-save
// text differs from the value the config kept after sanitizeURLs ran (i.e. it was
// rejected and reset). Empty/blank input is never reported; blank means "use default".
func mirrorResetDiff(fields []mirrorField) []string {
	var reset []string
	for _, f := range fields {
		if strings.TrimSpace(f.preSave) != "" && f.preSave != f.stored {
			reset = append(reset, f.name)
		}
	}
	return reset
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
	}, func() {
		// buildAppearanceTab has no local onEscape closure to give this button (it only
		// ever runs inside showSettingsView's dialog), so it reads the global directly.
		// Safe: showSettingsView assigns settingsOnEscape before Show() and clears it in
		// hideSettings, and the nil check covers construction order regardless.
		if um.settingsOnEscape != nil {
			um.settingsOnEscape()
		}
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

// findTitleLabel locates the modal's title label by text so refreshDirty can
// retitle it; matching by text avoids coupling to NewModalDialog's frame layout.
func findTitleLabel(root fyne.CanvasObject, text string) *widget.Label {
	switch o := root.(type) {
	case *widget.Label:
		if o.Text == text {
			return o
		}
	case *fyne.Container:
		for _, child := range o.Objects {
			if l := findTitleLabel(child, text); l != nil {
				return l
			}
		}
	}
	return nil
}

// showSettingsView shows a dialog to edit global settings
func (um *UIManager) showSettingsView() {
	// cancelOrConfirm/commitSave are defined later (they need widgets built below), but the
	// dialog* wrappers constructed from here on need something to forward Escape/Enter to
	// right away; onEscape/onEnter close over the vars so they resolve once assigned.
	var cancelOrConfirm func()
	var commitSave func()
	onEscape := func() {
		if cancelOrConfirm != nil {
			cancelOrConfirm()
		}
	}
	onEnter := func() {
		if commitSave != nil {
			commitSave()
		}
	}

	parentDirEntry := newDialogEntry(onEscape, onEnter)
	parentDirEntry.SetText(um.Config.ParentDir)
	parentDirEntry.SetPlaceHolder("Folder where game files / executables will be automatically installed")
	parentDirEntry.Validator = func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("required")
		}
		return nil
	}
	parentDirEntry.AlwaysShowValidationError = true
	parentDirEntry.Validate()

	var parentDirBtn *dialogButton
	parentDirBtn = newDialogButton("Browse...", func() {
		parentDirBtn.Disable()
		um.browseDirectory(&parentDirEntry.Entry, "Select Parent Directory", "Parent Directory (Settings)", parentDirBtn.Enable)
	}, onEscape)

	docsBasePathEntry := newDialogEntry(onEscape, onEnter)
	docsBasePathEntry.SetText(um.Config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")
	docsBasePathEntry.Validator = func(s string) error {
		if strings.TrimSpace(s) == "" {
			return errors.New("required")
		}
		return nil
	}
	docsBasePathEntry.AlwaysShowValidationError = true
	docsBasePathEntry.Validate()

	validationIcon := widget.NewIcon(theme.CancelIcon())
	validationIcon.Hide()
	validationLabel := mutedLabel("")
	validationLabel.Hide()

	var docsBasePathBtn *dialogButton
	docsBasePathBtn = newDialogButton("Browse...", func() {
		docsBasePathBtn.Disable()
		um.browseDirectory(&docsBasePathEntry.Entry, "Select Docs Base Path", "Docs Base Path (Settings)", docsBasePathBtn.Enable)
	}, onEscape)

	// Debounced: os.Stat runs 300ms after typing pauses, off the UI thread, since
	// the path can be an unreachable network share (cancel-and-reset, not just a
	// discard-if-stale check, so a burst of keystrokes triggers exactly one stat).
	// docsCheckGuard additionally covers Timer.Stop()'s documented gap (it can't
	// cancel a callback that already started): a slow, superseded check is
	// dropped by generation rather than allowed to overwrite a newer result.
	var docsCheckTimer *time.Timer
	var docsCheckGuard debounceGuard
	updateDocsValidation := func(path string) {
		if docsCheckTimer != nil {
			docsCheckTimer.Stop()
		}
		if path == "" {
			docsCheckGuard.next() // supersede any in-flight check; there's nothing to show
			validationIcon.Hide()
			validationLabel.Hide()
			return
		}
		gen := docsCheckGuard.next()
		docsCheckTimer = time.AfterFunc(300*time.Millisecond, func() {
			cfgPath := filepath.Join(path, "openttd.cfg")
			_, statErr := os.Stat(cfgPath)
			fyne.Do(func() {
				if !docsCheckGuard.current(gen) {
					return // superseded by a newer check
				}
				if statErr == nil {
					validationIcon.SetResource(theme.ConfirmIcon())
					validationLabel.SetText("openttd.cfg found")
				} else {
					validationIcon.SetResource(theme.CancelIcon())
					validationLabel.SetText("No openttd.cfg here")
				}
				validationIcon.Show()
				validationLabel.Show()
			})
		})
	}

	updateDocsValidation(docsBasePathEntry.Text)

	// blank = use default; otherwise must be a valid https URL (shared with the stored-config guard).
	httpsOrBlankValidator := func(s string) error {
		if strings.TrimSpace(s) == "" || domain.IsValidHTTPSURL(s) {
			return nil
		}
		return errors.New("must be a valid https:// URL or blank")
	}

	jgrppApiUrlEntry := newDialogEntry(onEscape, onEnter)
	jgrppApiUrlEntry.SetText(um.Config.JgrppApiUrl)
	jgrppApiUrlEntry.SetPlaceHolder("https://api.github.com/repos/JGRennison/OpenTTD-patches")
	jgrppApiUrlEntry.Validator = httpsOrBlankValidator
	jgrppApiUrlEntry.AlwaysShowValidationError = true
	jgrppApiUrlEntry.Validate()

	osDetected := platform.DefaultOSType()
	osTypeSelect := newDialogSelect(osTypeOptions(osDetected, um.Config.OSType), func(string) {}, onEscape, onEnter)
	osTypeSelect.SetSelected(osTypeValueToLabel(osDetected, um.Config.OSType))

	vanillaMirrorEntry := newDialogEntry(onEscape, onEnter)
	vanillaMirrorEntry.SetText(um.Config.VanillaMirror)
	vanillaMirrorEntry.SetPlaceHolder("https://cdn.openttd.org/openttd-releases/")
	vanillaMirrorEntry.Validator = httpsOrBlankValidator
	vanillaMirrorEntry.AlwaysShowValidationError = true
	vanillaMirrorEntry.Validate()

	nightlyMirrorEntry := newDialogEntry(onEscape, onEnter)
	nightlyMirrorEntry.SetText(um.Config.NightlyMirror)
	nightlyMirrorEntry.SetPlaceHolder("https://cdn.openttd.org/openttd-nightlies/")
	nightlyMirrorEntry.Validator = httpsOrBlankValidator
	nightlyMirrorEntry.AlwaysShowValidationError = true
	nightlyMirrorEntry.Validate()

	// Default client selector. Pre-selects the configured client; an unknown or
	// empty stored value simply leaves the dropdown unselected.
	defaultClientSelect := newDialogSelect(defaultClientOptions, func(string) {}, onEscape, onEnter)
	if label, ok := revDefaultClientMap[um.Config.DefaultClient]; ok {
		defaultClientSelect.SetSelected(label)
	}

	autoCloseCheck, autoCloseGroup := NewLabeledCheckWithDescription(
		"Auto-close launcher when OpenTTD starts",
		"Hides the launcher once the game opens.",
		um.Config.AutoCloseOnStart, onEscape, onEnter,
	)

	autoOpenLogCheck, autoOpenLogGroup := NewLabeledCheckWithDescription(
		"Auto-open log panel when game starts",
		"Opens the log panel so you can watch download/start progress.",
		um.Config.AutoOpenLog, onEscape, onEnter,
	)

	verboseCheck, verboseGroup := NewLabeledCheckWithDescription(
		"Verbose logging (show all messages)",
		"Includes debug-level messages in the log panel. Useful for troubleshooting.",
		um.Config.Verbose, onEscape, onEnter,
	)

	autoLaunchOpts, autoLaunchLabelToIdx := autoLaunchOptions(um.Config.Profiles)
	autoLaunchSelect := newDialogSelect(autoLaunchOpts, func(string) {}, onEscape, onEnter)
	autoLaunchSelect.SetSelected(autoLaunchSelectedLabel(um.Config.Profiles, um.Config.AutoLaunchProfile))

	subfolderCheck, subfolderGroup := NewLabeledCheckWithDescription(
		"Organise downloaded clients into per-client subfolders",
		"Keeps each client's downloaded files in a separate folder, instead of all sharing the parent folder. "+
			"If you change this later, anything already downloaded gets fetched again.",
		um.Config.SubfolderPerClient, onEscape, onEnter,
	)

	// Dirty-state compares live widget values to a baseline snapshot, never an
	// OnChanged flag (Entry.OnChanged fires on SetText). Appearance is excluded
	// from the snapshot because it persists live on click.
	type settingsSnapshot struct {
		parentDir, docsBasePath                    string
		subfolder, autoClose, autoOpenLog, verbose bool
		defaultClient, autoLaunch, osType          string
		vanillaMirror, nightlyMirror, jgrppApiURL  string
	}
	current := func() settingsSnapshot {
		return settingsSnapshot{
			parentDir: parentDirEntry.Text, docsBasePath: docsBasePathEntry.Text,
			subfolder: subfolderCheck.Checked, autoClose: autoCloseCheck.Checked,
			autoOpenLog: autoOpenLogCheck.Checked, verbose: verboseCheck.Checked,
			defaultClient: defaultClientSelect.Selected, autoLaunch: autoLaunchSelect.Selected,
			osType:        osTypeSelect.Selected,
			vanillaMirror: vanillaMirrorEntry.Text, nightlyMirror: nightlyMirrorEntry.Text, jgrppApiURL: jgrppApiUrlEntry.Text,
		}
	}
	baseline := current()
	isDirty := func() bool { return current() != baseline }

	// Files & Storage: the install/save anchors plus the subfolder layout switch.
	filesContent := container.NewVBox(
		NewSectionHeader("Installation Paths"),
		NewLabeledField("Parent Directory (required)",
			"RunTTD downloads, installs, and removes game clients here.",
			container.NewBorder(nil, nil, nil, parentDirBtn, parentDirEntry)),
		NewLabeledField("Docs Base Path (required)",
			"Where your saves and configuration (openttd.cfg) live. RunTTD reads from here but never modifies your files.",
			container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, validationLabel, docsBasePathBtn), docsBasePathEntry)),
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
		NewSectionHeader("Launch Behaviour"),
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

	mirrorBanner := widget.NewLabel("Expert overrides. A value that isn't a valid https:// address is reset to the official default. Leave blank to use the default.")
	mirrorBanner.Wrapping = fyne.TextWrapWord
	mirrorBanner.Importance = widget.LowImportance

	// Network & System: expert-only download overrides + the build target.
	networkContent := container.NewVBox(
		NewSectionHeader("Download Sources"),
		mirrorBanner,
		NewLabeledField("Vanilla Releases CDN base URL",
			"Where vanilla releases, including betas and release candidates, are fetched from.", vanillaMirrorEntry),
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
	var refreshDirty func() // assigned below once the dialog exists; captured early by resetBtn

	// hideSettings clears the scoped-Escape refs (Task 11) on every dialog hide so a
	// stale overlay ref can't mis-route a later Escape.
	hideSettings := func() {
		um.settingsOverlay = nil
		um.settingsOnEscape = nil
		settingsDialog.Hide()
	}

	saveSettings := func() {
		if strings.TrimSpace(parentDirEntry.Text) == "" || strings.TrimSpace(docsBasePathEntry.Text) == "" {
			return // backstop; the disabled button is the primary guard
		}
		// Full value snapshot (Profiles is a slice but this handler never mutates it),
		// so a failed save can revert everything below, including what SaveConfig's
		// own sanitizeURLs rewrites in place before the write is attempted.
		prevConfig := *um.Config
		prevSubfolderPerClient := um.Config.SubfolderPerClient
		preVanilla, preNightly, preJgrpp := vanillaMirrorEntry.Text, nightlyMirrorEntry.Text, jgrppApiUrlEntry.Text

		um.Config.ParentDir = strings.TrimSpace(parentDirEntry.Text)
		um.Config.DocsBasePath = strings.TrimSpace(docsBasePathEntry.Text)
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

		if !um.saveConfigOrWarn() {
			*um.Config = prevConfig
			return // leave the dialog open so the edits aren't lost; the user can retry
		}
		// ParentDir/SubfolderPerClient may have moved where installs are searched;
		// invalidate unconditionally so a stale answer can't survive a nil hook.
		um.diskLookups.invalidate()
		if um.profileListRefresh != nil {
			um.profileListRefresh() // move the §14 startup marker if AutoLaunchProfile changed here
		}
		if settingsDialog != nil {
			hideSettings()
		}

		if um.Config.SubfolderPerClient != prevSubfolderPerClient {
			dialog.ShowInformation(
				"Install location changed",
				"The launcher will now look for clients in the new location and may re-download them. Existing downloads are left in place; move or delete them manually if you no longer need them.",
				um.Window,
			)
		}

		// sanitizeURLs (in SaveConfig) silently resets invalid mirror URLs; surface that.
		reset := mirrorResetDiff([]mirrorField{
			{"Vanilla Releases CDN base URL", preVanilla, um.Config.VanillaMirror},
			{"Vanilla Nightly CDN base URL", preNightly, um.Config.NightlyMirror},
			{"JGRPP GitHub API URL", preJgrpp, um.Config.JgrppApiUrl},
		})
		if len(reset) > 0 {
			vanillaMirrorEntry.SetText(um.Config.VanillaMirror)
			nightlyMirrorEntry.SetText(um.Config.NightlyMirror)
			jgrppApiUrlEntry.SetText(um.Config.JgrppApiUrl)
			dialog.ShowInformation("Download sources reset",
				"These weren't valid https URLs and were restored to their official defaults:\n  - "+strings.Join(reset, "\n  - "), um.Window)
		}
	}
	commitSave = saveSettings
	saveBtn := newDialogButton("Save Settings", saveSettings, onEscape)

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord
	statusLabel.Importance = widget.LowImportance

	// updateState is the real Save gate; Fyne only auto-validates Entry, so a new
	// required field must be wired in here.
	updateState := func(string) {
		switch {
		case strings.TrimSpace(parentDirEntry.Text) == "":
			statusLabel.SetText("Enter a Parent Directory to continue.")
			saveBtn.Disable()
		case strings.TrimSpace(docsBasePathEntry.Text) == "":
			statusLabel.SetText("Enter a Docs Base Path to continue.")
			saveBtn.Disable()
		default:
			statusLabel.SetText("")
			saveBtn.Enable()
		}
	}
	// cancelOrConfirm closes the dialog directly when clean, else asks before discarding.
	// Reused by the Cancel button and (Task 11) the scoped Escape handler.
	cancelOrConfirm = func() {
		if !isDirty() {
			hideSettings()
			return
		}
		um.newConfirmDialog("Discard changes?", "Discard", "Keep editing",
			"You have unsaved changes. Theme and accent changes are kept (they apply immediately).",
			func(discard bool) {
				if discard {
					hideSettings()
				}
			}).Show()
	}
	cancelBtn := newDialogButton("Cancel", cancelOrConfirm, onEscape)

	// Reset stages the factory defaults into the widgets (persisted only on Save);
	// theme/accent persist immediately via applyAppearance (persist-on-click model).
	resetBtn := newDialogButton("Reset to Defaults", func() {
		um.newConfirmDialog("Reset to defaults?", "Reset", "Cancel",
			"Reset all settings to their defaults? Your profiles are not affected.",
			func(ok bool) {
				if !ok {
					return
				}
				parentDirEntry.SetText(um.Defaults.ParentDir)
				docsBasePathEntry.SetText(um.Defaults.DocsBasePath)
				subfolderCheck.SetChecked(um.Defaults.SubfolderPerClient)
				vanillaMirrorEntry.SetText("")
				nightlyMirrorEntry.SetText("")
				jgrppApiUrlEntry.SetText("")
				osTypeSelect.SetSelected(osTypeValueToLabel(osDetected, "")) // auto-detect
				autoCloseCheck.SetChecked(false)
				autoOpenLogCheck.SetChecked(false)
				verboseCheck.SetChecked(false)
				autoLaunchSelect.SetSelected(autoLaunchOffLabel)
				// DefaultClient is left as-is (optional; see spec §12).
				um.applyAppearance("dark", 0) // persists immediately (persist-on-click model)
				updateState("")
				refreshDirty()
			}).Show()
	}, onEscape)
	resetBtn.Importance = widget.WarningImportance

	// statusLabel sits below the tabs (above the toolbar) so the blocking hint is visible from any tab.
	content := container.NewBorder(nil, statusLabel, nil, nil, tabs)
	settingsDialog = NewModalDialog(um.Window.Canvas(), "Global Settings", content, resetBtn, cancelBtn, saveBtn)
	// ModalPopUp isn't drag-resizable; this is the initial size, kept honest by the Border-centred scrolls.
	settingsDialog.Resize(fyne.NewSize(760, 560))

	// Scope the Escape handler to this overlay by identity; cleared on hide via hideSettings.
	um.settingsOverlay = settingsDialog
	um.settingsOnEscape = cancelOrConfirm

	titleLabel := findTitleLabel(settingsDialog.Content, "Global Settings")
	// refreshDirty reflects unsaved edits: Save turns high-importance and the title gains " *".
	refreshDirty = func() {
		dirty := isDirty()
		if dirty {
			saveBtn.Importance = widget.HighImportance
		} else {
			saveBtn.Importance = widget.MediumImportance
		}
		saveBtn.Refresh()
		if titleLabel != nil {
			if dirty {
				titleLabel.SetText("Global Settings *")
			} else {
				titleLabel.SetText("Global Settings")
			}
		}
	}

	parentDirEntry.OnChanged = func(s string) { updateState(s); refreshDirty() }
	docsBasePathEntry.OnChanged = func(s string) { updateDocsValidation(s); updateState(s); refreshDirty() }
	dirtyOnChanged := func(string) { refreshDirty() }
	subfolderCheck.OnChanged = func(bool) { refreshDirty() }
	autoCloseCheck.OnChanged = func(bool) { refreshDirty() }
	autoOpenLogCheck.OnChanged = func(bool) { refreshDirty() }
	verboseCheck.OnChanged = func(bool) { refreshDirty() }
	defaultClientSelect.OnChanged = dirtyOnChanged
	autoLaunchSelect.OnChanged = dirtyOnChanged
	osTypeSelect.OnChanged = dirtyOnChanged
	vanillaMirrorEntry.OnChanged = dirtyOnChanged
	nightlyMirrorEntry.OnChanged = dirtyOnChanged
	jgrppApiUrlEntry.OnChanged = dirtyOnChanged
	updateState("")
	refreshDirty()

	settingsDialog.Show()
	um.Window.Canvas().Focus(parentDirEntry)
}
