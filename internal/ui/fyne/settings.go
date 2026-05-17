package fyne

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/ncruces/zenity"

	"openttd-launcher/internal/domain"
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
	var scrollBox *container.Scroll
	forwardScroll := func(ev *fyne.ScrollEvent) {
		if scrollBox != nil {
			scrollBox.Scrolled(ev)
		}
	}

	parentDirEntry := newScrollForwardingEntry(forwardScroll)
	parentDirEntry.SetText(um.Config.ParentDir)
	parentDirEntry.SetPlaceHolder("Folder where game files / executables will be automatically installed")

	parentDirBtn := widget.NewButton("Browse...", func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer func() {
				if r := recover(); r != nil {
					um.Logger.Append(fmt.Sprintf("CRITICAL: Zenity panicked: %v", r))
				}
			}()
			um.Logger.Append("Opening Parent Directory picker (Settings)...")
			directory, err := zenity.SelectFile(
				zenity.Directory(),
				zenity.Title("Select Parent Directory"),
				zenity.Filename(parentDirEntry.Text),
			)
			um.Logger.Append(fmt.Sprintf("Picker closed (Settings). Err: %v", err))
			if err == nil && directory != "" {
				parentDirEntry.SetText(directory)
			}
		}()
	})

	docsBasePathEntry := newScrollForwardingEntry(forwardScroll)
	docsBasePathEntry.SetText(um.Config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	validationIcon := widget.NewIcon(theme.CancelIcon())
	validationIcon.Hide()

	docsBasePathBtn := widget.NewButton("Browse...", func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer func() {
				if r := recover(); r != nil {
					um.Logger.Append(fmt.Sprintf("CRITICAL: Zenity panicked: %v", r))
				}
			}()
			um.Logger.Append("Opening Docs Base Path picker (Settings)...")
			directory, err := zenity.SelectFile(
				zenity.Directory(),
				zenity.Title("Select Docs Base Path"),
				zenity.Filename(docsBasePathEntry.Text),
			)
			um.Logger.Append(fmt.Sprintf("Picker closed (Settings). Err: %v", err))
			if err == nil && directory != "" {
				docsBasePathEntry.SetText(directory)
			}
		}()
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

	githubApiUrlEntry := newScrollForwardingEntry(forwardScroll)
	githubApiUrlEntry.SetText(um.Config.GithubApiUrl)

	osTypeEntry := newScrollForwardingEntry(forwardScroll)
	osTypeEntry.SetText(um.Config.OSType)

	vanillaMirrorEntry := newScrollForwardingEntry(forwardScroll)
	vanillaMirrorEntry.SetText(um.Config.VanillaMirror)
	vanillaMirrorEntry.SetPlaceHolder("https://cdn.openttd.org/openttd-releases/")

	nightlyMirrorEntry := newScrollForwardingEntry(forwardScroll)
	nightlyMirrorEntry.SetText(um.Config.NightlyMirror)
	nightlyMirrorEntry.SetPlaceHolder("https://cdn.openttd.org/openttd-nightlies/")

	// Default client selector: empty means no default
	defaultClientOptions := []string{"(none)", "OpenTTD (Stable)", "OpenTTD (Nightly)", "JGRPP"}
	defaultClientMap := map[string]string{"(none)": "", "OpenTTD (Stable)": "vanilla", "OpenTTD (Nightly)": "vanilla-nightly", "JGRPP": "jgrpp"}
	revDefaultClientMap := map[string]string{"": "(none)", "vanilla": "OpenTTD (Stable)", "vanilla-nightly": "OpenTTD (Nightly)", "jgrpp": "JGRPP"}
	defaultClientSelect := widget.NewSelect(defaultClientOptions, func(string) {})
	if label, ok := revDefaultClientMap[um.Config.DefaultClient]; ok {
		defaultClientSelect.SetSelected(label)
	}

	autoCloseCheck := widget.NewCheck("Auto-close launcher when OpenTTD starts", nil)
	autoCloseCheck.SetChecked(um.Config.AutoCloseOnStart)

	autoOpenLogCheck := widget.NewCheck("Auto-open log panel when game starts", nil)
	autoOpenLogCheck.SetChecked(um.Config.AutoOpenLog)

	verboseCheck := widget.NewCheck("Verbose logging (show all messages)", nil)
	verboseCheck.SetChecked(um.Config.Verbose)

	sectionTitle := func(title string) *widget.Label {
		label := widget.NewLabel(title)
		label.TextStyle = fyne.TextStyle{Bold: true}
		return label
	}

	pathsTab := container.NewTabItemWithIcon("Paths", theme.FolderIcon(), container.NewVBox(
		sectionTitle("Install Locations"),
		widget.NewLabel("Parent Directory (where game files / executables will be automatically installed)"),
		container.NewBorder(nil, nil, nil, parentDirBtn, parentDirEntry),
		widget.NewLabel("Docs Base Path (Saves & config)"),
		container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, docsBasePathBtn), docsBasePathEntry),
	))

	behaviorTab := container.NewTabItemWithIcon("Behavior", theme.ConfirmIcon(), container.NewVBox(
		sectionTitle("Launch Behavior"),
		container.NewGridWithColumns(1, autoCloseCheck, autoOpenLogCheck, verboseCheck),
	))

	advancedTab := container.NewTabItemWithIcon("Advanced", theme.SettingsIcon(), container.NewVBox(
		sectionTitle("System Settings"),
		widget.NewLabel("GitHub API URL"), githubApiUrlEntry,
		widget.NewLabel("OS Type (detected automatically)"), osTypeEntry,
		widget.NewSeparator(),
		widget.NewLabel("Vanilla CDN (stable) base URL"), vanillaMirrorEntry,
		widget.NewLabel("Vanilla Nightly CDN base URL"), nightlyMirrorEntry,
		widget.NewLabel("Default Client (new profiles)"), defaultClientSelect,
	))

	tabs := container.NewAppTabs(pathsTab, behaviorTab, advancedTab)
	tabs.SetTabLocation(container.TabLocationTop)

	var settingsDialog *widget.PopUp

	saveBtn := widget.NewButton("Save Settings", func() {
		um.Config.ParentDir = parentDirEntry.Text
		um.Config.DocsBasePath = docsBasePathEntry.Text
		um.Config.GithubApiUrl = githubApiUrlEntry.Text
		um.Config.OSType = osTypeEntry.Text
		um.Config.AutoCloseOnStart = autoCloseCheck.Checked
		um.Config.AutoOpenLog = autoOpenLogCheck.Checked
		um.Config.Verbose = verboseCheck.Checked
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
	})

	cancelBtn := widget.NewButton("Cancel", func() {
		settingsDialog.Hide()
	})

	title := widget.NewLabel("Global Settings")
	title.TextStyle = fyne.TextStyle{Bold: true}

	toolbar := container.NewCenter(container.NewHBox(
		container.NewPadded(cancelBtn),
		container.NewPadded(saveBtn),
	))

	content := container.NewBorder(
		container.NewPadded(title),
		container.NewPadded(toolbar),
		nil,
		nil,
		container.NewPadded(tabs),
	)

	settingsDialog = widget.NewModalPopUp(content, um.Window.Canvas())
	settingsDialog.Resize(fyne.NewSize(850, 600))
	settingsDialog.Show()
}
