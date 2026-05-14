package main

import (
	_ "embed" // required to activate //go:embed directives
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

//go:embed app_icon.png
var appIconBytes []byte

// UIManager manages the Fyne GUI application
type UIManager struct {
	app                 fyne.App
	window              fyne.Window
	config              *Config
	logger              *Logger
	configPath          string
	selectedProfileName string
	lastListSelectID    int
	lastListSelectAt    time.Time
	themeOverride       *fyne.ThemeVariant
}

// NewUIManager creates a new UI manager
func NewUIManager(config *Config, configPath string) *UIManager {
	fyneApp := app.New()
	// theme is set later
	appIcon := fyne.NewStaticResource("app_icon.png", appIconBytes)
	fyneApp.SetIcon(appIcon)
	window := fyneApp.NewWindow("JGRPP Launcher")
	window.SetIcon(appIcon)
	window.Resize(fyne.NewSize(960, 720))

	um := &UIManager{
		app:              fyneApp,
		window:           window,
		config:           config,
		logger:           NewLogger(config.LogToFile, resolveLogPath(configPath)),
		configPath:       configPath,
		lastListSelectID: -1,
	}
	fyneApp.Settings().SetTheme(&premiumTheme{Theme: theme.DefaultTheme(), overrideVariant: um.themeOverride})
	return um
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
	if um.config.FirstRun {
		um.window.SetContent(um.makeOnboardingView())
	} else {
		um.window.SetContent(um.makeMainView())
	}
	um.window.ShowAndRun()
}

// makeOnboardingView creates the first-run configuration screen
func (um *UIManager) makeOnboardingView() fyne.CanvasObject {
	welcomeLabel := widget.NewLabel("Welcome to JGRPP Launcher!")
	welcomeLabel.TextStyle = fyne.TextStyle{Bold: true, Italic: false}
	welcomeLabel.Alignment = fyne.TextAlignCenter

	instructions := widget.NewLabel("Before we begin, please confirm your installation folders.\nThese default paths are based on your operating system, but you can change them if you have a custom setup.")
	instructions.Wrapping = fyne.TextWrapWord

	parentDirEntry := widget.NewEntry()
	parentDirEntry.SetText(um.config.ParentDir)
	parentDirEntry.SetPlaceHolder("Folder where OpenTTD game files / executables will be automatically installed")

	docsBasePathEntry := widget.NewEntry()
	docsBasePathEntry.SetText(um.config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	validate := func() bool {
		if strings.TrimSpace(parentDirEntry.Text) == "" {
			statusLabel.SetText("Parent Directory cannot be empty.")
			return false
		}
		if strings.TrimSpace(docsBasePathEntry.Text) == "" {
			statusLabel.SetText("Docs Base Path cannot be empty.")
			return false
		}
		statusLabel.SetText("")
		return true
	}

	var continueBtn *widget.Button
	continueBtn = widget.NewButton("Continue", func() {
		if !validate() {
			return
		}

		um.config.ParentDir = strings.TrimSpace(parentDirEntry.Text)
		um.config.DocsBasePath = strings.TrimSpace(docsBasePathEntry.Text)
		um.config.FirstRun = false

		_ = SaveConfig(um.configPath, um.config)

		um.window.SetContent(um.makeMainView())
	})
	continueBtn.Importance = widget.HighImportance

	updateState := func(_ string) {
		if strings.TrimSpace(parentDirEntry.Text) != "" && strings.TrimSpace(docsBasePathEntry.Text) != "" {
			continueBtn.Enable()
		} else {
			continueBtn.Disable()
		}
	}
	parentDirEntry.OnChanged = updateState
	docsBasePathEntry.OnChanged = updateState

	form := container.NewVBox(
		welcomeLabel,
		widget.NewSeparator(),
		instructions,
		widget.NewSeparator(),
		widget.NewLabel("Parent Directory (where game files / executables will be automatically installed)"), parentDirEntry,
		widget.NewLabel("Docs Base Path (Saves & config)"), docsBasePathEntry,
		statusLabel,
	)

	return container.NewBorder(
		nil,
		container.NewHBox(widget.NewLabel(""), continueBtn),
		nil,
		nil,
		container.NewPadded(container.NewVScroll(form)),
	)
}

// makeMainView creates the main profile selection view
func (um *UIManager) makeMainView() fyne.CanvasObject {
	selectedIdx := indexOfProfileByName(um.config.Profiles, um.selectedProfileName)
	selectedLabel := widget.NewLabel("No profile selected")
	selectedLabel.TextStyle = fyne.TextStyle{Bold: true}

	selectedSummary := widget.NewLabel("Choose a profile to see its version, save path, and multiplayer settings.")
	selectedSummary.Wrapping = fyne.TextWrapWord

	selectedConfig := widget.NewLabel("")
	selectedConfig.Wrapping = fyne.TextWrapWord

	selectionHint := widget.NewLabel("Tip: select a profile, then press Enter, double-click the row, or use Run Selected.")
	selectionHint.Wrapping = fyne.TextWrapWord

	var profileList *widget.List
	var refreshDetails func()

	runSelected := func() {
		if selectedIdx >= 0 && selectedIdx < len(um.config.Profiles) {
			um.showLogView(selectedIdx)
			return
		}
		dialog.ShowError(fmt.Errorf("select a profile to launch"), um.window)
	}

	selectProfile := func(idx int) {
		if idx < 0 || idx >= len(um.config.Profiles) {
			selectedIdx = -1
			um.selectedProfileName = ""
			return
		}

		selectedIdx = idx
		um.selectedProfileName = um.config.Profiles[selectedIdx].Name
		refreshDetails()
		selectedSummary.Refresh()
		selectedConfig.Refresh()
		selectedLabel.Refresh()
	}

	handleRowTap := func(idx int) {
		now := time.Now()
		if idx == selectedIdx && idx == um.lastListSelectID && now.Sub(um.lastListSelectAt) < 450*time.Millisecond {
			um.lastListSelectAt = time.Time{}
			runSelected()
			return
		}

		um.lastListSelectID = idx
		um.lastListSelectAt = now
		profileList.Select(widget.ListItemID(idx))
	}

	refreshDetails = func() {
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

	profileList = widget.NewList(
		func() int { return len(um.config.Profiles) },
		func() fyne.CanvasObject {
			btn := widget.NewButton("Profile", nil)
			btn.Importance = widget.LowImportance
			btn.Alignment = widget.ButtonAlignLeading
			return btn
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			button := o.(*widget.Button)
			if i < len(um.config.Profiles) {
				profile := um.config.Profiles[i]
				versionText := profile.Version
				if versionText == "" {
					versionText = "latest"
				}
				button.SetText(fmt.Sprintf("%s   •   %s", profile.Name, versionText))
				idx := int(i)
				button.OnTapped = func() {
					handleRowTap(idx)
				}
			}
		},
	)
	profileList.OnSelected = func(id widget.ListItemID) {
		selectProfile(int(id))
	}
	profileList.OnUnselected = func(_ widget.ListItemID) {
		selectProfile(-1)
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
	duplicateBtn := widget.NewButton("Duplicate", func() {
		if selectedIdx >= 0 {
			dup := um.config.Profiles[selectedIdx]
			dup.Name = dup.Name + " Copy"
			um.config.Profiles = append(um.config.Profiles, dup)
			_ = SaveConfig(um.configPath, um.config)

			selectedIdx = len(um.config.Profiles) - 1
			um.selectedProfileName = um.config.Profiles[selectedIdx].Name
			profileList.Refresh()
			profileList.Select(widget.ListItemID(selectedIdx))
			refreshDetails()
			selectedSummary.Refresh()
			selectedConfig.Refresh()
			selectedLabel.Refresh()
		} else {
			dialog.ShowError(fmt.Errorf("select a profile to duplicate"), um.window)
		}
	})
	deleteBtn := widget.NewButton("Delete", func() {
		if selectedIdx >= 0 {
			if len(um.config.Profiles) > 1 {
				um.config.Profiles = append(um.config.Profiles[:selectedIdx], um.config.Profiles[selectedIdx+1:]...)
				_ = SaveConfig(um.configPath, um.config)

				nextIdx := selectedIdx
				if nextIdx >= len(um.config.Profiles) {
					nextIdx = len(um.config.Profiles) - 1
				}

				selectedIdx = nextIdx
				um.selectedProfileName = um.config.Profiles[selectedIdx].Name
				profileList.Refresh()
				profileList.Select(widget.ListItemID(selectedIdx))
				refreshDetails()
				selectedSummary.Refresh()
				selectedConfig.Refresh()
				selectedLabel.Refresh()
			} else {
				dialog.ShowError(fmt.Errorf("cannot delete the last profile"), um.window)
			}
		}
	})

	runBtn := widget.NewButton("Run Selected", runSelected)
	runBtn.Importance = widget.HighImportance

	settingsBtn := widget.NewButton("Settings", func() {
		um.showSettingsView()
	})

	actionsContent := container.NewVBox(
		runBtn,
		container.NewGridWithColumns(3, editBtn, duplicateBtn, deleteBtn),
	)

	leftPanelObj := container.NewBorder(
		widget.NewCard("Profiles", "", widget.NewLabel("Select a profile to edit or run it.")),
		container.NewPadded(container.NewVBox(widget.NewSeparator(), newBtn, settingsBtn)),
		nil,
		nil,
		profileList,
	)
	leftPanel := newThemedBox(colorNameSidebar, leftPanelObj)

	detailsSection := container.NewVBox(
		widget.NewCard("Profile Details", "", container.NewVBox(selectedLabel, selectedSummary, widget.NewSeparator(), selectedConfig)),
		widget.NewSeparator(),
		widget.NewCard("Actions", "", actionsContent),
		container.NewPadded(selectionHint),
	)

	rightPanelObj := container.NewVScroll(detailsSection)
	rightPanelObj.SetMinSize(fyne.NewSize(320, 0))
	rightPanel := newThemedBox(colorNameContent, rightPanelObj)

	if selectedIdx >= 0 && selectedIdx < len(um.config.Profiles) {
		profileList.Select(widget.ListItemID(selectedIdx))
	}
	refreshDetails()

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.42

	headerLabel := widget.NewLabel("JGRPP Launcher")
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}
	themeToggleBtn := widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), func() {
		current := fyne.CurrentApp().Settings().ThemeVariant()
		if um.themeOverride != nil {
			current = *um.themeOverride
		}
		var next fyne.ThemeVariant
		if current == theme.VariantDark {
			next = theme.VariantLight
		} else {
			next = theme.VariantDark
		}
		um.themeOverride = &next
		fyne.CurrentApp().Settings().SetTheme(&premiumTheme{Theme: theme.DefaultTheme(), overrideVariant: um.themeOverride})
	})
	themeToggleBtn.Importance = widget.LowImportance

	headerContent := container.NewBorder(nil, nil, nil, themeToggleBtn, headerLabel)
	header := newThemedBox(colorNameHeader, container.NewPadded(headerContent))

	mainContent := container.NewBorder(header, nil, nil, nil, split)
	um.window.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		if um.window.Content() != mainContent {
			return
		}

		if event.Name != fyne.KeyReturn && event.Name != fyne.KeyEnter {
			return
		}

		if selectedIdx >= 0 {
			runSelected()
		}
	})

	return mainContent
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

func indexOfProfileByName(profiles []Profile, name string) int {
	needle := strings.TrimSpace(name)
	if needle == "" {
		return -1
	}
	for i, p := range profiles {
		if strings.EqualFold(strings.TrimSpace(p.Name), needle) {
			return i
		}
	}
	return -1
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
		um.selectedProfileName = profile.Name

		_ = SaveConfig(um.configPath, um.config)
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

	leftColumn := container.NewVBox(
		identitySection,
		storageSection,
	)
	rightColumn := container.NewVBox(
		multiplayerSection,
	)
	columns := container.NewGridWithColumns(2, leftColumn, rightColumn)

	form := container.NewVBox(
		statusLabel,
		columns,
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
	editDialog.Resize(fyne.NewSize(750, 450))
	editDialog.Show()
}

// showSettingsView shows a dialog to edit global settings
func (um *UIManager) showSettingsView() {
	// scrollBox is assigned later; forwardScroll is captured by closure so entries
	// always forward to the real scroll container once it is created.
	var scrollBox *container.Scroll
	forwardScroll := func(ev *fyne.ScrollEvent) {
		if scrollBox != nil {
			scrollBox.Scrolled(ev)
		}
	}

	parentDirEntry := newScrollForwardingEntry(forwardScroll)
	parentDirEntry.SetText(um.config.ParentDir)
	parentDirEntry.SetPlaceHolder("Folder where game files / executables will be automatically installed")

	docsBasePathEntry := newScrollForwardingEntry(forwardScroll)
	docsBasePathEntry.SetText(um.config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	githubApiUrlEntry := newScrollForwardingEntry(forwardScroll)
	githubApiUrlEntry.SetText(um.config.GithubApiUrl)

	osTypeEntry := newScrollForwardingEntry(forwardScroll)
	osTypeEntry.SetText(um.config.OSType)

	autoCloseCheck := widget.NewCheck("Auto-close launcher when OpenTTD starts", nil)
	autoCloseCheck.SetChecked(um.config.AutoCloseOnStart)

	verboseCheck := widget.NewCheck("Verbose logging (show all messages)", nil)
	verboseCheck.SetChecked(um.config.Verbose)

	sectionTitle := func(title string) *widget.Label {
		label := widget.NewLabel(title)
		label.TextStyle = fyne.TextStyle{Bold: true}
		return label
	}

	basicSection := widget.NewCard(
		"Basic",
		"Commonly changed settings",
		container.NewVBox(
			sectionTitle("Install Paths"),
			widget.NewLabel("Parent Directory (where game files / executables will be automatically installed)"), parentDirEntry,
			widget.NewLabel("Docs Base Path (Saves & config)"), docsBasePathEntry,
		),
	)

	behaviorSection := widget.NewCard(
		"Behavior",
		"Launch behavior and UI logging",
		container.NewVBox(autoCloseCheck, verboseCheck),
	)

	advancedSection := widget.NewCard(
		"Advanced",
		"Only change if you know what you are doing",
		container.NewVBox(
			widget.NewLabel("GitHub API URL"), githubApiUrlEntry,
			widget.NewLabel("OS Type"), osTypeEntry,
		),
	)

	form := container.NewVBox(basicSection, behaviorSection, advancedSection)

	var settingsDialog dialog.Dialog

	saveBtn := widget.NewButton("Save Settings", func() {
		um.config.ParentDir = parentDirEntry.Text
		um.config.DocsBasePath = docsBasePathEntry.Text
		um.config.GithubApiUrl = githubApiUrlEntry.Text
		um.config.OSType = osTypeEntry.Text
		um.config.AutoCloseOnStart = autoCloseCheck.Checked
		um.config.Verbose = verboseCheck.Checked

		_ = SaveConfig(um.configPath, um.config)
		if settingsDialog != nil {
			settingsDialog.Hide()
		}
	})

	scrollBox = container.NewVScroll(form)

	content := container.NewBorder(
		nil,
		saveBtn,
		nil,
		nil,
		scrollBox,
	)

	settingsDialog = dialog.NewCustom("Settings", "Close", content, um.window)
	settingsDialog.Resize(fyne.NewSize(750, 450))
	settingsDialog.Show()
}

// showLogView shows a window with logs while launching a profile
func (um *UIManager) showLogView(profileIdx int) {
	profile := um.config.Profiles[profileIdx]
	statusBinding := binding.NewString()
	_ = statusBinding.Set("Preparing launch")

	summary := widget.NewLabel(fmt.Sprintf("Profile: %s\nVersion: %s\nSave path: %s\nServer: %s", profile.Name, valueOrDefault(profile.Version, "latest"), valueOrDefault(profile.SavePath, "(none)"), valueOrDefault(profile.ServerIpPort, "(none)")))
	summary.Wrapping = fyne.TextWrapWord
	statusLabel := widget.NewLabelWithData(statusBinding)
	statusLabel.Wrapping = fyne.TextWrapWord

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

	logBox := container.NewVScroll(logLabel)
	logBox.SetMinSize(fyne.NewSize(600, 400))

	closeBtn := widget.NewButton("Return to Main", func() {
		select {
		case <-done:
		default:
			close(done)
		}
		um.window.SetContent(um.makeMainView())
	})

	refreshBtn := widget.NewButton("Refresh Logs", func() {
		updateLogDisplay()
	})

	top := container.NewVBox(
		widget.NewCard("Launching", "Current launch context", summary),
		widget.NewCard("Status", "Background operations", statusLabel),
	)

	content := container.NewBorder(
		top,
		container.NewHBox(closeBtn, refreshBtn),
		nil,
		nil,
		logBox,
	)

	um.window.SetContent(content)

	// Launch OpenTTD in background
	go um.launchProfile(profile, func(status string) {
		_ = statusBinding.Set(status)
	})
}

// launchProfile launches OpenTTD with the specified profile
func (um *UIManager) launchProfile(profile Profile, updateStatus func(status string)) {
	if updateStatus != nil {
		updateStatus("Resolving profile and version")
	}
	um.LogImportant(fmt.Sprintf("Launching profile %q", profile.Name))
	um.LogVerbose(fmt.Sprintf("Profile config: version=%q savePath=%q server=%q company=%q", profile.Version, profile.SavePath, profile.ServerIpPort, profile.ServerCompanyNumber))

	requested := strings.TrimSpace(profile.Version)
	version := requested
	if requested == "" || strings.EqualFold(requested, "latest") {
		if updateStatus != nil {
			updateStatus("Resolving latest JGRPP version")
		}
		um.LogImportant("Resolving latest JGRPP version")
		version = CheckForNewVersion(um.config)
		if version == "" {
			um.LogImportant("Could not determine latest version from GitHub; trying latest local install.")
			if updateStatus != nil {
				updateStatus("Latest version lookup failed, using latest local install")
			}
			versionFolder := FindLatestFolder(um.config.ParentDir)
			if versionFolder == "" {
				if updateStatus != nil {
					updateStatus("Failed: no local JGRPP installation found")
				}
				um.LogImportant("No local JGRPP installation found.")
				return
			}
			um.LogVerbose(fmt.Sprintf("Using latest local version folder: %s", versionFolder))
			if updateStatus != nil {
				updateStatus("Starting OpenTTD from latest local installation")
			}
			ExecuteOpenTTD(versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, um.logger, um)
			if updateStatus != nil {
				updateStatus("Launch command sent")
			}
			return
		}
	}
	if requested != "" && !strings.EqualFold(requested, "latest") {
		if updateStatus != nil {
			updateStatus(fmt.Sprintf("Using requested JGRPP version %s", version))
		}
		um.LogImportant(fmt.Sprintf("Using requested JGRPP version %s", version))
	}

	if updateStatus != nil {
		updateStatus("Looking for local version folder")
	}
	versionFolder := FindVersionFolder(um.config.ParentDir, version)
	if versionFolder == "" {
		if updateStatus != nil {
			updateStatus("Version not found locally, downloading")
		}
		um.LogImportant(fmt.Sprintf("Version %s not found locally. Attempting to download.", version))
		if !DownloadAndExtractVersion(version, um.config) {
			if updateStatus != nil {
				updateStatus(fmt.Sprintf("Failed: download of version %s did not complete", version))
			}
			um.LogImportant(fmt.Sprintf("Failed to download version %s.", version))
			return
		}
		if updateStatus != nil {
			updateStatus("Download complete, resolving extracted folder")
		}
		versionFolder = FindVersionFolder(um.config.ParentDir, version)
		if versionFolder == "" {
			if updateStatus != nil {
				updateStatus("Failed: downloaded version folder could not be located")
			}
			um.LogImportant("Failed to locate downloaded version.")
			return
		}
	}

	um.LogVerbose(fmt.Sprintf("Using version folder: %s", versionFolder))
	if updateStatus != nil {
		updateStatus("Starting OpenTTD")
	}
	ExecuteOpenTTD(versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, um.logger, um)
	if updateStatus != nil {
		updateStatus("Launch command sent")
	}
}

// OnOpenTTDStarted is called when OpenTTD successfully starts
func (um *UIManager) OnOpenTTDStarted() {
	if um.config.AutoCloseOnStart {
		um.app.Quit()
	}
}

// --- Scroll Forwarding Entry ---

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

// --- Custom Premium Theming ---

const (
	colorNameSidebar fyne.ThemeColorName = "premiumSidebar"
	colorNameContent fyne.ThemeColorName = "premiumContent"
	colorNameHeader  fyne.ThemeColorName = "premiumHeader"
)

type premiumTheme struct {
	fyne.Theme
	overrideVariant *fyne.ThemeVariant
}

func (p *premiumTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if p.overrideVariant != nil {
		variant = *p.overrideVariant
	}
	if variant == theme.VariantDark {
		switch name {
		case theme.ColorNameBackground:
			return color.NRGBA{R: 26, G: 26, B: 29, A: 255} // Base app bg
		case colorNameSidebar:
			return color.NRGBA{R: 20, G: 20, B: 22, A: 255} // Deep dark sidebar
		case colorNameContent:
			return color.NRGBA{R: 30, G: 30, B: 34, A: 255} // Slightly elevated main content
		case colorNameHeader:
			return color.NRGBA{R: 36, G: 36, B: 42, A: 255} // Distinct header
		case theme.ColorNamePrimary:
			return color.NRGBA{R: 98, G: 114, B: 164, A: 255} // Premium indigo
		}
	} else {
		switch name {
		case theme.ColorNameBackground:
			return color.NRGBA{R: 245, G: 245, B: 247, A: 255}
		case colorNameSidebar:
			return color.NRGBA{R: 235, G: 235, B: 238, A: 255} // Slightly darker gray for contrast
		case colorNameContent:
			return color.NRGBA{R: 250, G: 250, B: 252, A: 255} // Bright clean content
		case colorNameHeader:
			return color.NRGBA{R: 220, G: 220, B: 225, A: 255} // Sleek gray header
		case theme.ColorNamePrimary:
			return color.NRGBA{R: 65, G: 88, B: 208, A: 255} // Vibrant blue
		}
	}
	return p.Theme.Color(name, variant)
}

type themedBox struct {
	widget.BaseWidget
	content   fyne.CanvasObject
	colorName fyne.ThemeColorName
}

func newThemedBox(colorName fyne.ThemeColorName, content fyne.CanvasObject) *themedBox {
	b := &themedBox{content: content, colorName: colorName}
	b.ExtendBaseWidget(b)
	return b
}

func (b *themedBox) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(theme.Color(b.colorName))
	return &themedBoxRenderer{rect: rect, content: b.content, b: b}
}

type themedBoxRenderer struct {
	rect    *canvas.Rectangle
	content fyne.CanvasObject
	b       *themedBox
}

func (r *themedBoxRenderer) Layout(size fyne.Size) {
	r.rect.Resize(size)
	r.content.Resize(size)
}

func (r *themedBoxRenderer) MinSize() fyne.Size {
	return r.content.MinSize()
}

func (r *themedBoxRenderer) Refresh() {
	r.rect.FillColor = theme.Color(r.b.colorName)
	r.rect.Refresh()
	r.content.Refresh()
}

func (r *themedBoxRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.rect, r.content}
}

func (r *themedBoxRenderer) Destroy() {}
