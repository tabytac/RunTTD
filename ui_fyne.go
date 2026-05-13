package main

import (
	"fmt"
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
	profileList := widget.NewList(
		func() int { return len(um.config.Profiles) },
		func() fyne.CanvasObject {
			return widget.NewLabel("Profile")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			label := o.(*widget.Label)
			if i < len(um.config.Profiles) {
				label.SetText(um.config.Profiles[i].Name)
			}
		},
	)
	selectedIdx := -1
	profileList.OnSelected = func(id widget.ListItemID) {
		selectedIdx = int(id)
	}

	// Buttons
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
				profileList.Refresh()
			} else {
				dialog.ShowError(fmt.Errorf("cannot delete the last profile"), um.window)
			}
		}
	})

	runBtn := widget.NewButton("Run", func() {
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

	// Layout
	buttonBox := container.NewVBox(
		newBtn,
		editBtn,
		deleteBtn,
		widget.NewSeparator(),
		runBtn,
		settingsBtn,
	)

	mainBox := container.NewBorder(
		widget.NewCard("Profiles", "", profileList),
		nil,
		buttonBox,
		nil,
		profileList,
	)

	return mainBox
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

	versionEntry := widget.NewEntry()
	versionEntry.SetText(profile.Version)

	savePathEntry := widget.NewEntry()
	savePathEntry.SetText(profile.SavePath)

	ipPortEntry := widget.NewEntry()
	ipPortEntry.SetText(profile.ServerIpPort)

	serverPassEntry := widget.NewEntry()
	serverPassEntry.SetText(profile.ServerPassword)
	serverPassEntry.Password = true

	companyNumEntry := widget.NewEntry()
	companyNumEntry.SetText(profile.ServerCompanyNumber)

	companyPassEntry := widget.NewEntry()
	companyPassEntry.SetText(profile.ServerCompanyPassword)
	companyPassEntry.Password = true

	form := container.NewVBox(
		widget.NewCard("Profile Name", "", nameEntry),
		widget.NewCard("JGRPP Version (e.g. 0.71.2 or latest)", "", versionEntry),
		widget.NewCard("Save Path", "", savePathEntry),
		widget.NewCard("Server IP:Port", "", ipPortEntry),
		widget.NewCard("Server Password", "", serverPassEntry),
		widget.NewCard("Company Number", "", companyNumEntry),
		widget.NewCard("Company Password", "", companyPassEntry),
	)

	saveBtn := widget.NewButton("Save", func() {
		profile.Name = nameEntry.Text
		profile.Version = versionEntry.Text
		profile.SavePath = savePathEntry.Text
		profile.ServerIpPort = ipPortEntry.Text
		profile.ServerPassword = serverPassEntry.Text
		profile.ServerCompanyNumber = companyNumEntry.Text
		profile.ServerCompanyPassword = companyPassEntry.Text

		if isNew {
			um.config.Profiles = append(um.config.Profiles, profile)
		} else {
			um.config.Profiles[profileIdx] = profile
		}

		_ = SaveConfig(um.configPath, um.config)
		dialog.ShowInformation("Success", "Profile saved", um.window)
		um.window.SetContent(um.makeMainView())
	})

	scrollBox := container.NewVScroll(form)
	scrollBox.SetMinSize(fyne.NewSize(400, 400))

	content := container.NewBorder(
		nil,
		saveBtn,
		nil,
		nil,
		scrollBox,
	)

	editDialog := dialog.NewCustom("Edit Profile", "Close", content, um.window)
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
