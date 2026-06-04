package fyne

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
)

// Default Client selector options shared by the onboarding screen and the
// global settings dialog. defaultClientMap is label->config value;
// revDefaultClientMap is the inverse. Default Client is a required, always-set
// value, so there is no "(none)" option.
var (
	defaultClientOptions = []string{"OpenTTD (Stable)", "OpenTTD (Nightly)", "JGRPP", "Custom Executable"}
	defaultClientMap     = map[string]string{"OpenTTD (Stable)": "vanilla", "OpenTTD (Nightly)": "vanilla-nightly", "JGRPP": "jgrpp", "Custom Executable": "custom"}
	revDefaultClientMap  = map[string]string{"vanilla": "OpenTTD (Stable)", "vanilla-nightly": "OpenTTD (Nightly)", "jgrpp": "JGRPP", "custom": "Custom Executable"}
)

// scrollForwardingEntry forwards scroll events to parent containers
type scrollForwardingEntry struct {
	widget.Entry
	forward func(ev *fyne.ScrollEvent)
}

func newScrollForwardingEntry(forward func(ev *fyne.ScrollEvent)) *scrollForwardingEntry {
	e := &scrollForwardingEntry{forward: forward}
	e.ExtendBaseWidget(e)
	return e
}

func (e *scrollForwardingEntry) Scrolled(ev *fyne.ScrollEvent) {
	if e.forward != nil {
		e.forward(ev)
	}
}

// showSettingsView shows a dialog to edit global settings
func (um *UIManager) showSettingsView() {
	var scrolls map[*container.TabItem]*container.Scroll
	var tabs *container.AppTabs
	forwardScroll := func(ev *fyne.ScrollEvent) {
		if scrolls == nil || tabs == nil {
			return
		}
		if s, ok := scrolls[tabs.Selected()]; ok {
			s.Scrolled(ev)
		}
	}

	parentDirEntry := newScrollForwardingEntry(forwardScroll)
	parentDirEntry.SetText(um.Config.ParentDir)
	parentDirEntry.SetPlaceHolder("Folder where game files / executables will be automatically installed")

	parentDirBtn := widget.NewButton("Browse...", func() {
		um.browseDirectory(&parentDirEntry.Entry, "Select Parent Directory", "Parent Directory (Settings)")
	})

	docsBasePathEntry := newScrollForwardingEntry(forwardScroll)
	docsBasePathEntry.SetText(um.Config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	validationIcon := widget.NewIcon(theme.CancelIcon())
	validationIcon.Hide()

	docsBasePathBtn := widget.NewButton("Browse...", func() {
		um.browseDirectory(&docsBasePathEntry.Entry, "Select Docs Base Path", "Docs Base Path (Settings)")
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

	jgrppApiUrlEntry := newScrollForwardingEntry(forwardScroll)
	jgrppApiUrlEntry.SetText(um.Config.JgrppApiUrl)

	osTypeEntry := newScrollForwardingEntry(forwardScroll)
	osTypeEntry.SetText(um.Config.OSType)

	vanillaMirrorEntry := newScrollForwardingEntry(forwardScroll)
	vanillaMirrorEntry.SetText(um.Config.VanillaMirror)
	vanillaMirrorEntry.SetPlaceHolder("https://cdn.openttd.org/openttd-releases/")

	nightlyMirrorEntry := newScrollForwardingEntry(forwardScroll)
	nightlyMirrorEntry.SetText(um.Config.NightlyMirror)
	nightlyMirrorEntry.SetPlaceHolder("https://cdn.openttd.org/openttd-nightlies/")

	// Default client selector. Pre-selects the configured client; an unknown or
	// empty stored value simply leaves the dropdown unselected.
	defaultClientSelect := widget.NewSelect(defaultClientOptions, func(string) {})
	if label, ok := revDefaultClientMap[um.Config.DefaultClient]; ok {
		defaultClientSelect.SetSelected(label)
	}

	autoCloseCheck := widget.NewCheck("Auto-close launcher when OpenTTD starts", nil)
	autoCloseCheck.SetChecked(um.Config.AutoCloseOnStart)

	autoOpenLogCheck := widget.NewCheck("Auto-open log panel when game starts", nil)
	autoOpenLogCheck.SetChecked(um.Config.AutoOpenLog)

	verboseCheck, verboseGroup := NewLabeledCheckWithDescription(
		"Verbose logging (show all messages)",
		"Includes debug-level messages in the log panel. Useful for troubleshooting.",
		um.Config.Verbose,
	)

	subfolderCheck, subfolderGroup := NewLabeledCheckWithDescription(
		"Organize downloaded clients into per-client subfolders",
		"Keeps each client's downloaded files in a separate folder, instead of all sharing the parent folder. "+
			"If you change this later, anything already downloaded gets fetched again.",
		um.Config.SubfolderPerClient,
	)

	pathsContent := container.NewVBox(
		NewSectionHeader("Installation Paths"),
		widget.NewLabel("Parent Directory (where game files / executables will be automatically installed)"),
		container.NewBorder(nil, nil, nil, parentDirBtn, parentDirEntry),
		widget.NewLabel("Docs Base Path (Saves & config)"),
		container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, docsBasePathBtn), docsBasePathEntry),
		subfolderGroup,
	)
	pathsScroll := container.NewVScroll(pathsContent)
	pathsTab := container.NewTabItemWithIcon("Paths", theme.FolderIcon(), pathsScroll)

	behaviorContent := container.NewVBox(
		NewSectionHeader("Launch Behavior"),
		autoCloseCheck,
		autoOpenLogCheck,
		verboseGroup,
	)
	behaviorScroll := container.NewVScroll(behaviorContent)
	behaviorTab := container.NewTabItemWithIcon("Behavior", theme.ConfirmIcon(), behaviorScroll)

	advancedContent := container.NewVBox(
		NewSectionHeader("Download Sources"),
		widget.NewLabel("Vanilla CDN (stable) base URL"), vanillaMirrorEntry,
		widget.NewLabel("Vanilla Nightly CDN base URL"), nightlyMirrorEntry,
		widget.NewLabel("JGRPP GitHub API URL"), jgrppApiUrlEntry,
		NewSectionHeader("Profile Defaults"),
		widget.NewLabel("Default Client (new profiles)"), defaultClientSelect,
		NewSectionHeader("System"),
		widget.NewLabel("OS Type (detected automatically)"), osTypeEntry,
	)
	advancedScroll := container.NewVScroll(advancedContent)
	advancedTab := container.NewTabItemWithIcon("Advanced", theme.SettingsIcon(), advancedScroll)

	tabs = container.NewAppTabs(pathsTab, behaviorTab, advancedTab)
	scrolls = map[*container.TabItem]*container.Scroll{
		pathsTab:    pathsScroll,
		behaviorTab: behaviorScroll,
		advancedTab: advancedScroll,
	}
	tabs.SetTabLocation(container.TabLocationTop)

	var settingsDialog *widget.PopUp

	saveBtn := widget.NewButton("Save Settings", func() {
		prevSubfolderPerClient := um.Config.SubfolderPerClient

		um.Config.ParentDir = parentDirEntry.Text
		um.Config.DocsBasePath = docsBasePathEntry.Text
		um.Config.JgrppApiUrl = jgrppApiUrlEntry.Text
		um.Config.OSType = osTypeEntry.Text
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
	settingsDialog.Resize(fyne.NewSize(850, 600))
	settingsDialog.Show()
}
