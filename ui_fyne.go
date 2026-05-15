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
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/dweymouth/fyne-advanced-list"
	"github.com/ncruces/zenity"
	"os"
	"path/filepath"
	"runtime"
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
	cachedVersions      []string
}

// rightClickButton is a button that also handles right-clicks
type rightClickButton struct {
	widget.Button
	onSecondaryTapped func()
}

func newRightClickButton(tapped func(), secondaryTapped func()) *rightClickButton {
	b := &rightClickButton{onSecondaryTapped: secondaryTapped}
	b.OnTapped = tapped
	b.Importance = widget.LowImportance
	b.ExtendBaseWidget(b)
	return b
}

func (b *rightClickButton) TappedSecondary(_ *fyne.PointEvent) {
	if b.onSecondaryTapped != nil {
		b.onSecondaryTapped()
	}
}

// NewUIManager creates a new UI manager
func NewUIManager(config *Config, configPath string) *UIManager {
	fyneApp := app.New()
	// theme is set later
	appIcon := fyne.NewStaticResource("app_icon.png", appIconBytes)
	fyneApp.SetIcon(appIcon)
	window := fyneApp.NewWindow("JGRPP Launcher")
	window.SetIcon(appIcon)
	window.Resize(fyne.NewSize(1024, 768))

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

	parentDirBtn := widget.NewButton("Browse...", func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer func() {
				if r := recover(); r != nil {
					um.logger.Append(fmt.Sprintf("CRITICAL: Zenity panicked: %v", r))
				}
			}()
			um.logger.Append("Opening Parent Directory picker...")
			directory, err := zenity.SelectFile(
				zenity.Directory(),
				zenity.Title("Select Parent Directory"),
				zenity.Filename(parentDirEntry.Text),
			)
			um.logger.Append(fmt.Sprintf("Picker closed. Err: %v", err))
			if err == nil && directory != "" {
				parentDirEntry.SetText(directory)
			}
		}()
	})

	docsBasePathEntry := widget.NewEntry()
	docsBasePathEntry.SetText(um.config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	validationIcon := widget.NewIcon(theme.CancelIcon())
	validationIcon.Hide()

	docsBasePathBtn := widget.NewButton("Browse...", func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer func() {
				if r := recover(); r != nil {
					um.logger.Append(fmt.Sprintf("CRITICAL: Zenity panicked: %v", r))
				}
			}()
			um.logger.Append("Opening Docs Base Path picker...")
			directory, err := zenity.SelectFile(
				zenity.Directory(),
				zenity.Title("Select Docs Base Path"),
				zenity.Filename(docsBasePathEntry.Text),
			)
			um.logger.Append(fmt.Sprintf("Picker closed. Err: %v", err))
			if err == nil && directory != "" {
				docsBasePathEntry.SetText(directory)
			}
		}()
	})

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

	updateState := func(_ string) {
		if strings.TrimSpace(parentDirEntry.Text) != "" && strings.TrimSpace(docsBasePathEntry.Text) != "" {
			continueBtn.Enable()
		} else {
			continueBtn.Disable()
		}
	}
	parentDirEntry.OnChanged = updateState
	docsBasePathEntry.OnChanged = func(s string) {
		updateState(s)
		updateDocsValidation(s)
	}
	updateDocsValidation(docsBasePathEntry.Text)

	form := container.NewVBox(
		welcomeLabel,
		widget.NewSeparator(),
		instructions,
		widget.NewSeparator(),
		widget.NewLabel("Parent Directory (where game files / executables will be automatically installed)"),
		container.NewBorder(nil, nil, nil, parentDirBtn, parentDirEntry),
		widget.NewLabel("Docs Base Path (Saves & config)"),
		container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, docsBasePathBtn), docsBasePathEntry),
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

	selectionHint := widget.NewLabel("Tip: Press 1-9 to quick launch, or select a profile and press Enter / double-click.")
	selectionHint.Wrapping = fyne.TextWrapWord

	var profileList *fyneadvancedlist.List
	var refreshDetails func()

	var runBtn, editBtn, duplicateBtn, deleteBtn *widget.Button

	runSelected := func() {
		if selectedIdx >= 0 && selectedIdx < len(um.config.Profiles) {
			um.showLogView(selectedIdx)
			return
		}
		dialog.ShowError(fmt.Errorf("select a profile to launch"), um.window)
	}

	updateButtonStates := func() {
		if selectedIdx >= 0 {
			runBtn.Enable()
			editBtn.Enable()
			duplicateBtn.Enable()
			deleteBtn.Enable()
		} else {
			runBtn.Disable()
			editBtn.Disable()
			duplicateBtn.Disable()
			deleteBtn.Disable()
		}
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
		updateButtonStates()
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

	profileList = fyneadvancedlist.NewList(
		func() int { return len(um.config.Profiles) },
		func() fyne.CanvasObject {
			btn := newRightClickButton(nil, nil)


			nameLabel := widget.NewLabel("")
			versionLabel := widget.NewLabel("")
			versionLabel.Alignment = fyne.TextAlignTrailing
			versionLabel.TextStyle = fyne.TextStyle{Italic: true}

			dragIcon := widget.NewIcon(theme.MenuIcon())

			// Border layout: name on left, [version + drag] on right
			rightSide := container.NewHBox(versionLabel, dragIcon)
			layout := container.NewBorder(nil, nil, nameLabel, rightSide, nil)
			return container.NewStack(btn, container.NewPadded(layout))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			stack := o.(*fyne.Container)
			btn := stack.Objects[0].(*rightClickButton)
			padding := stack.Objects[1].(*fyne.Container)
			layout := padding.Objects[0].(*fyne.Container)
			nameLabel := layout.Objects[0].(*widget.Label)
			rightSide := layout.Objects[1].(*fyne.Container)
			versionLabel := rightSide.Objects[0].(*widget.Label)

			if i < len(um.config.Profiles) {
				profile := um.config.Profiles[i]
				versionText := profile.Version
				if versionText == "" {
					versionText = "latest"
				}
				nameLabel.SetText(fmt.Sprintf("%d. %s", i+1, profile.Name))
				versionLabel.SetText(versionText)
				idx := int(i)
				btn.OnTapped = func() {
					handleRowTap(idx)
				}
				btn.onSecondaryTapped = func() {
					um.showProfileEditor(idx)
				}
			}
		},
	)

	profileList.EnableDragging = true
	profileList.OnDragEnd = func(draggedFrom, draggedTo widget.ListItemID) {
		from := int(draggedFrom)
		to := int(draggedTo)

		if from == to {
			return
		}

		// Adjust 'to' because advanced-list provides the insertion index
		if to > from {
			to--
		}

		profile := um.config.Profiles[from]
		// Remove from old position
		um.config.Profiles = append(um.config.Profiles[:from], um.config.Profiles[from+1:]...)
		// Insert into new position
		newProfiles := make([]Profile, 0, len(um.config.Profiles)+1)
		newProfiles = append(newProfiles, um.config.Profiles[:to]...)
		newProfiles = append(newProfiles, profile)
		newProfiles = append(newProfiles, um.config.Profiles[to:]...)
		um.config.Profiles = newProfiles

		_ = SaveConfig(um.configPath, um.config)
		profileList.Refresh()

		// Re-select if the dragged profile was selected
		if from == selectedIdx {
			profileList.Select(widget.ListItemID(to))
		}
	}
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
	editBtn = widget.NewButton("Edit", func() {
		if selectedIdx >= 0 {
			um.showProfileEditor(selectedIdx)
		} else {
			dialog.ShowError(fmt.Errorf("select a profile to edit"), um.window)
		}
	})
	duplicateBtn = widget.NewButton("Duplicate", func() {
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
	deleteBtn = widget.NewButton("Delete", func() {
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

	runBtn = widget.NewButton("Run Selected", runSelected)
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

	detailsContent := container.NewVScroll(container.NewPadded(
		container.NewVBox(selectedLabel, selectedSummary, widget.NewSeparator(), selectedConfig),
	))

	rightPanelObj := container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), actionsContent, container.NewPadded(selectionHint)),
		nil,
		nil,
		detailsContent,
	)
	rightPanel := newThemedBox(colorNameContent, rightPanelObj)

	if selectedIdx >= 0 && selectedIdx < len(um.config.Profiles) {
		profileList.Select(widget.ListItemID(selectedIdx))
	}
	updateButtonStates()
	refreshDetails()

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.35

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
		if um.window.Content() != mainContent || um.window.Canvas().Overlays().Top() != nil {
			return
		}

		// Handle number keys 1-9 for quick launch
		if len(event.Name) == 1 && event.Name[0] >= '1' && event.Name[0] <= '9' {
			idx := int(event.Name[0] - '1')
			if idx < len(um.config.Profiles) {
				um.showLogView(idx)
				return
			}
		}

		if event.Name == fyne.KeyReturn || event.Name == fyne.KeyEnter {
			if selectedIdx >= 0 {
				runSelected()
			}
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

	versionEntry := widget.NewSelect(um.cachedVersions, nil)
	if len(um.cachedVersions) == 0 {
		versionEntry.Options = []string{"latest"}
		go func() {
			versions, err := FetchAvailableVersions(um.config)
			if err == nil && len(versions) > 0 {
				um.cachedVersions = versions
				versionEntry.Options = versions
				versionEntry.Refresh()
			}
		}()
	}
	// Fallback if the profile's version isn't in the options yet
	if profile.Version != "" {
		versionEntry.SetSelected(profile.Version)
	} else {
		versionEntry.SetSelected("latest")
	}
	versionEntry.PlaceHolder = "latest or 0.71.2"
	
	// Auto-detect mode for legacy configs or unsaved changes
	if profile.LaunchMode == "" {
		if profile.ServerIpPort != "" {
			profile.LaunchMode = "multiplayer"
		} else if profile.SavePath != "" {
			// Check if SavePath is a directory
			abs := profile.SavePath
			if !filepath.IsAbs(abs) && um.config.DocsBasePath != "" {
				abs = filepath.Join(um.config.DocsBasePath, "save", abs)
			}
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				profile.LaunchMode = "folder"
			} else {
				profile.LaunchMode = "file"
			}
		}
	}

	// Launch Mode selection
	modeMap := map[string]string{
		"Normal":             "",
		"Single File":        "file",
		"Auto-Latest Folder": "folder",
		"Multiplayer":        "multiplayer",
	}
	revModeMap := map[string]string{
		"":            "Normal",
		"file":        "Single File",
		"folder":      "Auto-Latest Folder",
		"multiplayer": "Multiplayer",
	}

	modeSelect := widget.NewRadioGroup([]string{"Normal", "Single File", "Auto-Latest Folder", "Multiplayer"}, nil)
	modeSelect.Horizontal = true
	modeSelect.SetSelected(revModeMap[profile.LaunchMode])

	// Multiplayer fields
	ipPortEntry := widget.NewEntry()
	ipPortEntry.SetText(profile.ServerIpPort)
	ipPortEntry.SetPlaceHolder("host:port")

	serverPassEntry := widget.NewPasswordEntry()
	serverPassEntry.SetText(profile.ServerPassword)
	serverPassEntry.SetPlaceHolder("Optional password")

	companyPassEntry := widget.NewPasswordEntry()
	companyPassEntry.SetText(profile.ServerCompanyPassword)
	companyPassEntry.SetPlaceHolder("Optional company password")

	companyNumEntry := widget.NewEntry()
	companyNumEntry.SetText(profile.ServerCompanyNumber)
	companyNumEntry.SetPlaceHolder("Optional company number")

	// Save fields
	savePathEntry := widget.NewEntry()
	savePathEntry.SetText(profile.SavePath)
	savePathEntry.SetPlaceHolder("Path to file or folder")

	browseFileBtn := widget.NewButtonWithIcon("Browse File...", theme.FileIcon(), func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			startPath := savePathEntry.Text
			if startPath == "" {
				if um.config.DocsBasePath != "" {
					startPath = filepath.Join(um.config.DocsBasePath, "save")
				}
			} else if !filepath.IsAbs(startPath) && um.config.DocsBasePath != "" {
				startPath = filepath.Join(um.config.DocsBasePath, "save", startPath)
			}

			file, err := zenity.SelectFile(
				zenity.Title("Select Save or Scenario"),
				zenity.FileFilters{
					{Name: "OpenTTD Saves/Scenarios", Patterns: []string{"*.sav", "*.scn"}},
				},
				zenity.Filename(startPath),
			)
			if err == nil && file != "" {
				path := file
				if um.config.DocsBasePath != "" {
					rel, err := filepath.Rel(um.config.DocsBasePath, path)
					if err == nil && !strings.HasPrefix(rel, "..") {
						path = rel
					}
				}
				savePathEntry.SetText(path)
			}
		}()
	})

	browseFolderBtn := widget.NewButtonWithIcon("Browse Folder...", theme.FolderIcon(), func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			startPath := savePathEntry.Text
			if startPath == "" {
				if um.config.DocsBasePath != "" {
					startPath = filepath.Join(um.config.DocsBasePath, "save")
				}
			} else if !filepath.IsAbs(startPath) && um.config.DocsBasePath != "" {
				startPath = filepath.Join(um.config.DocsBasePath, "save", startPath)
			}

			directory, err := zenity.SelectFile(
				zenity.Directory(),
				zenity.Title("Select Save Folder"),
				zenity.Filename(startPath),
			)
			if err == nil && directory != "" {
				path := directory
				if um.config.DocsBasePath != "" {
					rel, err := filepath.Rel(um.config.DocsBasePath, path)
					if err == nil && !strings.HasPrefix(rel, "..") {
						path = rel
					}
				}
				savePathEntry.SetText(path)
			}
		}()
	})

	sectionTitle := func(title string) *widget.Label {
		label := widget.NewLabel(title)
		label.TextStyle = fyne.TextStyle{Bold: true}
		return label
	}

	// Visibility Containers
	fileOption := container.NewVBox(
		widget.NewLabel("Select a specific save or scenario file to load:"),
		container.NewBorder(nil, nil, nil, browseFileBtn, savePathEntry),
	)
	folderOption := container.NewVBox(
		widget.NewLabel("Select a folder; the launcher will auto-load the most recent save inside it:"),
		container.NewBorder(nil, nil, nil, browseFolderBtn, savePathEntry),
	)
	multiplayerOption := container.NewVBox(
		sectionTitle("Server Connection"),
		widget.NewLabel("Server IP:Port"), ipPortEntry,
		widget.NewLabel("Server Password"), serverPassEntry,
		widget.NewSeparator(),
		sectionTitle("Company Details"),
		container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Company Number"), companyNumEntry),
			container.NewVBox(widget.NewLabel("Company Password"), companyPassEntry),
		),
	)
	normalOption := widget.NewLabel("The game will launch normally to the main menu.")

	optionsStack := container.NewStack(normalOption, fileOption, folderOption, multiplayerOption)

	updateVisibility := func(mode string) {
		normalOption.Hide()
		fileOption.Hide()
		folderOption.Hide()
		multiplayerOption.Hide()
		switch mode {
		case "Normal":
			normalOption.Show()
		case "Single File":
			fileOption.Show()
		case "Auto-Latest Folder":
			folderOption.Show()
		case "Multiplayer":
			multiplayerOption.Show()
		}
		optionsStack.Refresh()
	}

	modeSelect.OnChanged = updateVisibility
	updateVisibility(modeSelect.Selected)

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord
	var editDialog dialog.Dialog



	validate := func() (bool, string) {
		if strings.TrimSpace(nameEntry.Text) == "" {
			return false, "Profile name is required."
		}

		if strings.TrimSpace(versionEntry.Selected) == "" {
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
		profile.Version = strings.TrimSpace(versionEntry.Selected)
		if profile.Version == "" {
			profile.Version = "latest"
		}
		profile.SavePath = strings.TrimSpace(savePathEntry.Text)
		profile.ServerIpPort = strings.TrimSpace(ipPortEntry.Text)
		profile.ServerPassword = serverPassEntry.Text
		profile.ServerCompanyNumber = strings.TrimSpace(companyNumEntry.Text)
		profile.ServerCompanyPassword = companyPassEntry.Text
		profile.LaunchMode = modeMap[modeSelect.Selected]
		if profile.LaunchMode == "" {
			profile.SavePath = ""
			profile.ServerIpPort = ""
		} else if profile.LaunchMode == "multiplayer" {
			profile.SavePath = ""
		} else {
			profile.ServerIpPort = ""
		}

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

	generalTab := container.NewTabItemWithIcon("General", theme.SettingsIcon(), container.NewVBox(
		sectionTitle("Identity"),
		widget.NewLabel("Name"), nameEntry,
		widget.NewLabel("JGRPP Version"), versionEntry,
	))

	launchTab := container.NewTabItemWithIcon("Launch Mode", theme.MediaPlayIcon(), container.NewVBox(
		sectionTitle("How should the game start?"),
		modeSelect,
		widget.NewSeparator(),
		optionsStack,
	))

	filesTab := container.NewTabItemWithIcon("Paths", theme.FolderIcon(), container.NewVBox(
		sectionTitle("Advanced Folder Management"),
		widget.NewLabel("You can manually specify an override here, though the Launch Mode tab is usually preferred."),
		savePathEntry, // Still allow manual editing if needed
	))

	tabs := container.NewAppTabs(generalTab, launchTab, filesTab)
	tabs.SetTabLocation(container.TabLocationTop)

	form := container.NewVBox(
		statusLabel,
		tabs,
	)

	saveBtn = widget.NewButton("Save", func() { saveProfile(false) })
	saveAndRunBtn = widget.NewButton("Save & Run", func() { saveProfile(true) })
	if isNew {
		saveAndRunBtn.SetText("Create & Run")
	}
	cancelBtn := widget.NewButton("Cancel", func() {
		editDialog.Hide()
	})

	toolbar := container.NewHBox(cancelBtn, layout.NewSpacer(), saveBtn, saveAndRunBtn)
	content := container.NewBorder(nil, container.NewPadded(toolbar), nil, nil, container.NewPadded(form))

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
	for _, entry := range []*widget.Entry{nameEntry, savePathEntry, ipPortEntry, serverPassEntry, companyNumEntry, companyPassEntry} {
		entry.OnChanged = func(string) {
			updateState()
		}
	}
	versionEntry.OnChanged = func(string) {
		updateState()
	}
	updateState()

	editDialog = dialog.NewCustom("Edit Profile", "Close", content, um.window)
	editDialog.Resize(fyne.NewSize(850, 600))
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

	parentDirBtn := widget.NewButton("Browse...", func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer func() {
				if r := recover(); r != nil {
					um.logger.Append(fmt.Sprintf("CRITICAL: Zenity panicked: %v", r))
				}
			}()
			um.logger.Append("Opening Parent Directory picker (Settings)...")
			directory, err := zenity.SelectFile(
				zenity.Directory(),
				zenity.Title("Select Parent Directory"),
				zenity.Filename(parentDirEntry.Text),
			)
			um.logger.Append(fmt.Sprintf("Picker closed (Settings). Err: %v", err))
			if err == nil && directory != "" {
				parentDirEntry.SetText(directory)
			}
		}()
	})

	docsBasePathEntry := newScrollForwardingEntry(forwardScroll)
	docsBasePathEntry.SetText(um.config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	validationIcon := widget.NewIcon(theme.CancelIcon())
	validationIcon.Hide()

	docsBasePathBtn := widget.NewButton("Browse...", func() {
		go func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			defer func() {
				if r := recover(); r != nil {
					um.logger.Append(fmt.Sprintf("CRITICAL: Zenity panicked: %v", r))
				}
			}()
			um.logger.Append("Opening Docs Base Path picker (Settings)...")
			directory, err := zenity.SelectFile(
				zenity.Directory(),
				zenity.Title("Select Docs Base Path"),
				zenity.Filename(docsBasePathEntry.Text),
			)
			um.logger.Append(fmt.Sprintf("Picker closed (Settings). Err: %v", err))
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

	pathsTab := container.NewTabItemWithIcon("Paths", theme.FolderIcon(), container.NewVBox(
		sectionTitle("Install Locations"),
		widget.NewLabel("Parent Directory (where game files / executables will be automatically installed)"),
		container.NewBorder(nil, nil, nil, parentDirBtn, parentDirEntry),
		widget.NewLabel("Docs Base Path (Saves & config)"),
		container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, docsBasePathBtn), docsBasePathEntry),
	))

	behaviorTab := container.NewTabItemWithIcon("Behavior", theme.ConfirmIcon(), container.NewVBox(
		sectionTitle("Launch Behavior"),
		container.NewGridWithColumns(2, autoCloseCheck, verboseCheck),
	))

	advancedTab := container.NewTabItemWithIcon("Advanced", theme.SettingsIcon(), container.NewVBox(
		sectionTitle("System Settings"),
		widget.NewLabel("GitHub API URL"), githubApiUrlEntry,
		widget.NewLabel("OS Type (detected automatically)"), osTypeEntry,
	))

	tabs := container.NewAppTabs(pathsTab, behaviorTab, advancedTab)
	tabs.SetTabLocation(container.TabLocationTop)

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



	content := container.NewBorder(
		nil,
		container.NewPadded(saveBtn),
		nil,
		nil,
		container.NewPadded(tabs),
	)

	settingsDialog = dialog.NewCustom("Settings", "Close", content, um.window)
	settingsDialog.Resize(fyne.NewSize(850, 600))
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
			ExecuteOpenTTD(versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, profile.LaunchMode, um.logger, um)
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
	ExecuteOpenTTD(versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, profile.LaunchMode, um.logger, um)
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
