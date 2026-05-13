package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// UIManager manages the Fyne GUI application
type UIManager struct {
	app          fyne.App
	window       fyne.Window
	config       *Config
	logger       *Logger
	configPath   string
}

// NewUIManager creates a new UI manager
func NewUIManager(config *Config, configPath string) *UIManager {
	fyneApp := app.New()
	window := fyneApp.NewWindow("JGRPP Launcher")
	window.Resize(fyne.NewSize(800, 600))

	return &UIManager{
		app:    fyneApp,
		window: window,
		config: config,
		logger: NewLogger(config.LogToFile, resolveLogPath(configPath)),
		configPath: configPath,
	}
}

// LogVerbose appends a message only if verbose logging is enabled
func (um *UIManager) LogVerbose(msg string) {
	if um.config.Verbose {
		um.logger.Append(msg)
	}
}

// LogImportant always appends a message (for important messages, errors, etc.)
func (um *UIManager) LogImportant(msg string) {
	um.logger.Append(msg)
}

// Show displays the launcher UI and runs the event loop
func (um *UIManager) Show() {
	um.window.SetContent(um.makeMainView())
	um.window.ShowAndRun()
}

// makeMainView creates the main profile selection view
func (um *UIManager) makeMainView() fyne.CanvasObject {
	selectedIdx := -1
	selectedLabel := widget.NewLabel("No profile selected")
	selectedLabel.TextStyle = fyne.TextStyle{Bold: true}

	selectedSummary := widget.NewLabel("Choose a profile to see its version, save path, and multiplayer settings.")
	selectedSummary.Wrapping = fyne.TextWrapWord

	selectedConfig := widget.NewLabel("")
	selectedConfig.Wrapping = fyne.TextWrapWord

	selectionHint := widget.NewLabel("Tip: use Run after selecting a profile, or double-click a row to launch.")
	selectionHint.Wrapping = fyne.TextWrapWord

	refreshDetails := func() {
		if selectedIdx < 0 || selectedIdx >= len(um.config.Profiles) {
			selectedLabel.SetText("No profile selected")
			selectedSummary.SetText("Choose a profile to see its version, save path, and multiplayer settings.")
			selectedConfig.SetText("")
			return
		}

		profile := um.config.Profiles[selectedIdx]
		selectedLabel.SetText(profile.Name)

		versionText := profile.Version
		if versionText == "" {
			versionText = "latest"
		}

		selectedSummary.SetText(fmt.Sprintf("Version: %s\nSave path: %s", versionText, valueOrDefault(profile.SavePath, "(none)")))
		selectedConfig.SetText(fmt.Sprintf(
			"Server: %s\nCompany: %s\nServer password: %s\nCompany password: %s",
			valueOrDefault(profile.ServerIpPort, "(none)"),
			valueOrDefault(profile.ServerCompanyNumber, "(spectator)"),
			maskedOrEmpty(profile.ServerPassword),
			maskedOrEmpty(profile.ServerCompanyPassword),
		))
	}

	profileList := widget.NewList(
		func() int { return len(um.config.Profiles) },
		func() fyne.CanvasObject { return widget.NewLabel("Profile") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			label := o.(*widget.Label)
			if i < len(um.config.Profiles) {
				profile := um.config.Profiles[i]
				versionText := profile.Version
				if versionText == "" {
					versionText = "latest"
				}
				label.SetText(fmt.Sprintf("%s   •   %s", profile.Name, versionText))
			}
		},
	)
	profileList.OnSelected = func(id widget.ListItemID) {
		selectedIdx = int(id)
		refreshDetails()
		selectedSummary.Refresh()
		selectedConfig.Refresh()
		selectedLabel.Refresh()
	}
	profileList.OnUnselected = func(_ widget.ListItemID) {
		selectedIdx = -1
		refreshDetails()
		selectedSummary.Refresh()
		selectedConfig.Refresh()
		selectedLabel.Refresh()
	}
	profileList.OnSelected = func(id widget.ListItemID) {
		selectedIdx = int(id)
		refreshDetails()
		selectedSummary.Refresh()
		selectedConfig.Refresh()
		selectedLabel.Refresh()
	}

	newBtn := widget.NewButton("New Profile", func() {
		um.showProfileEditor(-1)
	})
	editBtn := widget.NewButton("Edit", func() {
		if selectedIdx >= 0 {
			um.showProfileEditor(selectedIdx)
		} else {
			dialog.ShowError(fmt.Errorf("select a profile to edit"), um.window)
		}
	})
	deleteBtn := widget.NewButton("Delete", func() {
		if selectedIdx >= 0 {
			if len(um.config.Profiles) > 1 {
				um.config.Profiles = append(um.config.Profiles[:selectedIdx], um.config.Profiles[selectedIdx+1:]...)
				_ = SaveConfig(um.configPath, um.config)
				selectedIdx = -1
				profileList.UnselectAll()
				profileList.Refresh()
				refreshDetails()
				selectedSummary.Refresh()
				selectedConfig.Refresh()
				selectedLabel.Refresh()
			} else {
				dialog.ShowError(fmt.Errorf("cannot delete the last profile"), um.window)
			}
		}
	})

	runBtn := widget.NewButton("Run Selected", func() {
		if selectedIdx >= 0 {
			um.showLogView(selectedIdx)
		} else {
			dialog.ShowError(fmt.Errorf("select a profile to launch"), um.window)
		}
	})
	runBtn.Importance = widget.HighImportance

	settingsBtn := widget.NewButton("Settings", func() {
		um.showSettingsView()
	})

	leftPanel := container.NewBorder(
		widget.NewCard("Profiles", "", widget.NewLabel("Select a profile to edit or run it.")),
		nil,
		nil,
		nil,
		profileList,
	)

	detailsSection := container.NewVBox(
		widget.NewCard("Profile Details", "", container.NewVBox(selectedLabel, selectedSummary, widget.NewSeparator(), selectedConfig)),
		widget.NewCard("Actions", "", container.NewVBox(runBtn, editBtn, deleteBtn, widget.NewSeparator(), newBtn, settingsBtn)),
		selectionHint,
	)

	rightPanel := container.NewVScroll(detailsSection)
	rightPanel.SetMinSize(fyne.NewSize(320, 0))

	refreshDetails()

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.42

	header := widget.NewCard("JGRPP Launcher", "Profiles, details, and launch actions", widget.NewLabel("Choose a profile on the left, review its details on the right, then run or edit it."))

	return container.NewBorder(header, nil, nil, nil, split)
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func maskedOrEmpty(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return "set"
}

// showProfileEditor shows a dialog to create or edit a profile
func (um *UIManager) showProfileEditor(profileIdx int) {
	var profile Profile
	isNew := profileIdx < 0

	if !isNew {
		profile = um.config.Profiles[profileIdx]
	}

	// Form fields
	nameEntry := widget.NewEntry()
	nameEntry.SetText(profile.Name)
	nameEntry.SetPlaceHolder("Profile name")

	versionEntry := widget.NewEntry()
	versionEntry.SetText(profile.Version)
	versionEntry.SetPlaceHolder("latest or 0.71.2")

	savePathEntry := widget.NewEntry()
	savePathEntry.SetText(profile.SavePath)
	savePathEntry.SetPlaceHolder("Optional save folder")

	ipPortEntry := widget.NewEntry()
	ipPortEntry.SetText(profile.ServerIpPort)
	ipPortEntry.SetPlaceHolder("host:port")

	serverPassEntry := widget.NewEntry()
	serverPassEntry.SetText(profile.ServerPassword)
	serverPassEntry.Password = true
	serverPassEntry.SetPlaceHolder("Optional password")

	companyNumEntry := widget.NewEntry()
	companyNumEntry.SetText(profile.ServerCompanyNumber)
	companyNumEntry.SetPlaceHolder("Optional company number")

	companyPassEntry := widget.NewEntry()
	companyPassEntry.SetText(profile.ServerCompanyPassword)
	companyPassEntry.Password = true
	companyPassEntry.SetPlaceHolder("Optional company password")

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord
	var editDialog dialog.Dialog

	sectionTitle := func(title string) *widget.Label {
		label := widget.NewLabel(title)
		label.TextStyle = fyne.TextStyle{Bold: true}
		return label
	}

	makeSection := func(title string, objects ...fyne.CanvasObject) fyne.CanvasObject {
		content := make([]fyne.CanvasObject, 0, len(objects)+1)
		content = append(content, sectionTitle(title))
		content = append(content, objects...)
		return container.NewVBox(content...)
	}

	validate := func() (bool, string) {
		if strings.TrimSpace(nameEntry.Text) == "" {
			return false, "Profile name is required."
		}

		if strings.TrimSpace(versionEntry.Text) == "" {
			return false, "JGRPP version is required or use latest."
		}

		if strings.TrimSpace(ipPortEntry.Text) != "" && !strings.Contains(ipPortEntry.Text, ":") {
			return false, "Server IP:Port should look like host:port."
		}

		if strings.TrimSpace(companyNumEntry.Text) != "" {
			if _, err := strconv.Atoi(strings.TrimSpace(companyNumEntry.Text)); err != nil {
				return false, "Company number must be numeric."
			}
		}

		return true, ""
	}

	setStatus := func() {
		ok, message := validate()
		statusLabel.SetText(message)
		_ = ok
		statusLabel.Refresh()
	}

	var saveBtn *widget.Button
	var saveAndRunBtn *widget.Button

	saveProfile := func(runAfter bool) {
		if ok, message := validate(); !ok {
			statusLabel.SetText(message)
			statusLabel.Refresh()
			return
		}

		profile.Name = strings.TrimSpace(nameEntry.Text)
		profile.Version = strings.TrimSpace(versionEntry.Text)
		if profile.Version == "" {
			profile.Version = "latest"
		}
		profile.SavePath = strings.TrimSpace(savePathEntry.Text)
		profile.ServerIpPort = strings.TrimSpace(ipPortEntry.Text)
		profile.ServerPassword = serverPassEntry.Text
		profile.ServerCompanyNumber = strings.TrimSpace(companyNumEntry.Text)
		profile.ServerCompanyPassword = companyPassEntry.Text

		savedIdx := profileIdx
		if isNew {
			um.config.Profiles = append(um.config.Profiles, profile)
			savedIdx = len(um.config.Profiles) - 1
		} else {
			um.config.Profiles[profileIdx] = profile
		}

		_ = SaveConfig(um.configPath, um.config)
		dialog.ShowInformation("Success", "Profile saved", um.window)
		editDialog.Hide()
		um.window.SetContent(um.makeMainView())
		if runAfter {
			um.showLogView(savedIdx)
		}
	}

	identitySection := makeSection(
		"Identity and Version",
		widget.NewLabel("Name"), nameEntry,
		widget.NewLabel("JGRPP Version"), versionEntry,
	)
	storageSection := makeSection(
		"Save Path",
		widget.NewLabel("Save folder"), savePathEntry,
	)
	multiplayerSection := makeSection(
		"Multiplayer",
		widget.NewLabel("Server IP:Port"), ipPortEntry,
		widget.NewLabel("Server Password"), serverPassEntry,
		widget.NewLabel("Company Number"), companyNumEntry,
		widget.NewLabel("Company Password"), companyPassEntry,
	)

	form := container.NewVBox(
		statusLabel,
		identitySection,
		storageSection,
		multiplayerSection,
	)

	saveBtn = widget.NewButton("Save", func() { saveProfile(false) })
	saveAndRunBtn = widget.NewButton("Save & Run", func() { saveProfile(true) })
	if isNew {
		saveAndRunBtn.SetText("Create & Run")
	}
	cancelBtn := widget.NewButton("Cancel", func() {
		editDialog.Hide()
	})

	toolbar := container.NewHBox(cancelBtn, saveBtn, saveAndRunBtn)
	top := widget.NewCard("Edit Profile", "Sectioned profile setup", form)
	content := container.NewBorder(nil, toolbar, nil, nil, top)

	updateState := func() {
		ok, _ := validate()
		if ok {
			saveBtn.Enable()
			saveAndRunBtn.Enable()
		} else {
			saveBtn.Disable()
			saveAndRunBtn.Disable()
		}
		setStatus()
	}
	for _, entry := range []*widget.Entry{nameEntry, versionEntry, savePathEntry, ipPortEntry, serverPassEntry, companyNumEntry, companyPassEntry} {
		entry.OnChanged = func(string) {
			updateState()
		}
	}
	updateState()

	editDialog = dialog.NewCustom("Edit Profile", "Close", content, um.window)
	editDialog.Show()
}

// showSettingsView shows a dialog to edit global settings
func (um *UIManager) showSettingsView() {
	parentDirEntry := widget.NewEntry()
	parentDirEntry.SetText(um.config.ParentDir)

	docsBasePathEntry := widget.NewEntry()
	docsBasePathEntry.SetText(um.config.DocsBasePath)

	githubApiUrlEntry := widget.NewEntry()
	githubApiUrlEntry.SetText(um.config.GithubApiUrl)

	osTypeEntry := widget.NewEntry()
	osTypeEntry.SetText(um.config.OSType)

	autoCloseCheck := widget.NewCheck("Auto-close launcher when OpenTTD starts", nil)
	autoCloseCheck.SetChecked(um.config.AutoCloseOnStart)

	verboseCheck := widget.NewCheck("Verbose logging (show all messages)", nil)
	verboseCheck.SetChecked(um.config.Verbose)

	form := container.NewVBox(
		widget.NewCard("Parent Dir", "", parentDirEntry),
		widget.NewCard("Docs Base Path", "", docsBasePathEntry),
		widget.NewCard("GitHub API URL", "", githubApiUrlEntry),
		widget.NewCard("OS Type", "", osTypeEntry),
		autoCloseCheck,
		verboseCheck,
	)

	saveBtn := widget.NewButton("Save Settings", func() {
		um.config.ParentDir = parentDirEntry.Text
		um.config.DocsBasePath = docsBasePathEntry.Text
		um.config.GithubApiUrl = githubApiUrlEntry.Text
		um.config.OSType = osTypeEntry.Text
		um.config.AutoCloseOnStart = autoCloseCheck.Checked
		um.config.Verbose = verboseCheck.Checked

		_ = SaveConfig(um.configPath, um.config)
		dialog.ShowInformation("Success", "Settings saved", um.window)
	})

	scrollBox := container.NewVScroll(form)
	scrollBox.SetMinSize(fyne.NewSize(400, 300))

	content := container.NewBorder(
		nil,
		saveBtn,
		nil,
		nil,
		scrollBox,
	)

	settingsDialog := dialog.NewCustom("Settings", "Close", content, um.window)
	settingsDialog.Show()
}

// showLogView shows a window with logs while launching a profile
func (um *UIManager) showLogView(profileIdx int) {
	profile := um.config.Profiles[profileIdx]

	// Create log text widget using data binding (thread-safe)
	logBinding := binding.NewString()
	logLabel := widget.NewLabelWithData(logBinding)
	logLabel.Wrapping = fyne.TextWrapWord

	// Update the log display whenever logger changes
	updateLogDisplay := func() {
		logs := um.logger.GetAll()
		text := ""
		for _, line := range logs {
			text += line + "\n"
		}
		_ = logBinding.Set(text)
	}

	// Start a goroutine to periodically update the log display
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			updateLogDisplay()
		}
	}()

	logBox := container.NewVScroll(logLabel)
	logBox.SetMinSize(fyne.NewSize(600, 400))

	closeBtn := widget.NewButton("Close", func() {
		um.window.SetContent(um.makeMainView())
	})

	content := container.NewBorder(
		widget.NewRichTextFromMarkdown("**Launching: " + profile.Name + "**"),
		closeBtn,
		nil,
		nil,
		logBox,
	)

	um.window.SetContent(content)

	// Launch OpenTTD in background
	go um.launchProfile(profile, updateLogDisplay)
}

// launchProfile launches OpenTTD with the specified profile
func (um *UIManager) launchProfile(profile Profile, updateUI func()) {
	um.LogImportant(fmt.Sprintf("Launching profile %q", profile.Name))
	um.LogVerbose(fmt.Sprintf("Profile config: version=%q savePath=%q server=%q company=%q", profile.Version, profile.SavePath, profile.ServerIpPort, profile.ServerCompanyNumber))

	requested := strings.TrimSpace(profile.Version)
	version := requested
	if requested == "" || strings.EqualFold(requested, "latest") {
		um.LogImportant("Resolving latest JGRPP version")
		version = CheckForNewVersion(um.config)
		if version == "" {
			um.LogImportant("Could not determine latest version from GitHub; trying latest local install.")
			versionFolder := FindLatestFolder(um.config.ParentDir)
			if versionFolder == "" {
				um.LogImportant("No local JGRPP installation found.")
				return
			}
			um.LogVerbose(fmt.Sprintf("Using latest local version folder: %s", versionFolder))
			ExecuteOpenTTD(versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, um.logger, um)
			return
		}
	}
	if requested != "" && !strings.EqualFold(requested, "latest") {
		um.LogImportant(fmt.Sprintf("Using requested JGRPP version %s", version))
	}

	versionFolder := FindVersionFolder(um.config.ParentDir, version)
	if versionFolder == "" {
		um.LogImportant(fmt.Sprintf("Version %s not found locally. Attempting to download.", version))
		if !DownloadAndExtractVersion(version, um.config) {
			um.LogImportant(fmt.Sprintf("Failed to download version %s.", version))
			return
		}
		versionFolder = FindVersionFolder(um.config.ParentDir, version)
		if versionFolder == "" {
			um.LogImportant("Failed to locate downloaded version.")
			return
		}
	}

	um.LogVerbose(fmt.Sprintf("Using version folder: %s", versionFolder))
	ExecuteOpenTTD(versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, um.logger, um)
}

// OnOpenTTDStarted is called when OpenTTD successfully starts
func (um *UIManager) OnOpenTTDStarted() {
	if um.config.AutoCloseOnStart {
		um.app.Quit()
	}
}
