package main

import (
	_ "embed" // required to activate //go:embed directives
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	"os"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fyneadvancedlist "github.com/dweymouth/fyne-advanced-list"
	"github.com/ncruces/zenity"
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

	if um.config.ThemeVariant == "" {
		um.config.ThemeVariant = "dark"
	}
	if um.config.AccentPreset < 0 || um.config.AccentPreset >= len(themePresets) {
		um.config.AccentPreset = 0
	}

	preset := themePresets[um.config.AccentPreset]
	light, _ := parseHexColor(preset.lightHex)
	dark, _ := parseHexColor(preset.darkHex)

	pt := &launcherTheme{
		Theme:       theme.DefaultTheme(),
		accentDark:  dark,
		accentLight: light,
	}
	if um.config.ThemeVariant == "light" {
		v := theme.VariantLight
		pt.overrideVariant = &v
	} else if um.config.ThemeVariant == "dark" {
		v := theme.VariantDark
		pt.overrideVariant = &v
	}

	um.app.Settings().SetTheme(pt)

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
	detailsContainer := container.NewVBox()

	selectionHint := widget.NewLabel("Tip: Press 1-9 to quick launch, or select a profile and press Enter / double-click.")
	selectionHint.Wrapping = fyne.TextWrapWord

	var profileList *fyneadvancedlist.List
	var refreshDetails func()

	var runBtn, editBtn, duplicateBtn, deleteBtn, seeLogsBtn *widget.Button
	var updateButtonStates func()

	runSelected := func() {
		if selectedIdx >= 0 && selectedIdx < len(um.config.Profiles) {
			if um.config.AutoOpenLog {
				um.showLogView(selectedIdx)
			} else {
				// Background launch with feedback
				oldText := runBtn.Text
				runBtn.SetText("Launching...")
				runBtn.Disable()

				profile := um.config.Profiles[selectedIdx]
				um.showToast(fmt.Sprintf("Starting %s...", profile.Name))

				go func() {
					um.launchProfile(profile, nil, func() {
						// On Error: Auto-open logs
						um.showLogView(selectedIdx)
					})

					time.Sleep(1500 * time.Millisecond)
					runBtn.SetText(oldText)
					updateButtonStates() // Re-enables if still selected
				}()
			}
			return
		}
		dialog.ShowError(fmt.Errorf("select a profile to launch"), um.window)
	}

	updateButtonStates = func() {
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

		if seeLogsBtn != nil {
			if len(um.logger.GetAll()) > 0 {
				seeLogsBtn.Enable()
			} else {
				seeLogsBtn.Disable()
			}
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

	addDetail := func(c *fyne.Container, icon fyne.Resource, label, value string, mono bool) {
		if value == "" {
			return
		}

		title := widget.NewLabel(label)
		title.TextStyle = fyne.TextStyle{Bold: true}

		val := widget.NewLabel(value)
		val.Wrapping = fyne.TextWrapWord
		if mono {
			val.TextStyle = fyne.TextStyle{Monospace: true}
		}

		iconObj := widget.NewIcon(icon)

		// Use a border layout to keep the icon fixed on the left
		row := container.NewBorder(nil, nil, iconObj, nil, container.NewVBox(title, val))
		c.Add(container.NewPadded(row))
	}

	refreshDetails = func() {
		updateButtonStates()
		detailsContainer.Objects = nil

		if selectedIdx < 0 || selectedIdx >= len(um.config.Profiles) {
			welcomeTitle := widget.NewLabel("Welcome to JGRPP Launcher")
			welcomeTitle.TextStyle = fyne.TextStyle{Bold: true}
			welcomeTitle.Alignment = fyne.TextAlignCenter

			welcomeBody := widget.NewLabel("Select a profile on the left to see its details and launch the game.")
			welcomeBody.Wrapping = fyne.TextWrapWord
			welcomeBody.Alignment = fyne.TextAlignCenter

			detailsContainer.Add(welcomeTitle)
			detailsContainer.Add(welcomeBody)
			detailsContainer.Refresh()
			return
		}

		profile := um.config.Profiles[selectedIdx]

		// 1. Header: Profile Name
		nameLabel := widget.NewLabel(profile.Name)
		nameLabel.TextStyle = fyne.TextStyle{Bold: true}
		detailsContainer.Add(nameLabel)

		// 2. Action Intent Badge
		intent := "Launch into Main Menu"
		if profile.LaunchMode == "file" {
			intent = fmt.Sprintf("Launch into save: %s", filepath.Base(profile.SavePath))
		} else if profile.LaunchMode == "folder" {
			intent = fmt.Sprintf("Launch newest in: %s", filepath.Base(profile.SavePath))
		} else if profile.LaunchMode == "multiplayer" {
			intent = fmt.Sprintf("Launch and join: %s", valueOrDefault(profile.ServerIpPort, "Server"))
		}

		intentLabel := widget.NewLabel(intent)
		intentLabel.TextStyle = fyne.TextStyle{Bold: true, Italic: true}
		// Add a slight background or styling if possible, otherwise just distinct text
		detailsContainer.Add(container.NewPadded(intentLabel))
		detailsContainer.Add(widget.NewSeparator())

		// 3. Version
		versionText := profile.Version
		if versionText == "" {
			versionText = "latest"
		}
		addDetail(detailsContainer, theme.SettingsIcon(), "JGRPP Version", versionText, false)

		// 4. Situational: Paths and Filters
		if profile.LaunchMode == "file" || profile.LaunchMode == "folder" {
			icon := theme.FolderOpenIcon()
			label := "Folder Path"
			if profile.LaunchMode == "file" {
				icon = theme.FileIcon()
				label = "File Path"
			}
			addDetail(detailsContainer, icon, label, profile.SavePath, false)

			if profile.LaunchMode == "folder" && profile.AutoLatestFilter != "" {
				addDetail(detailsContainer, theme.SearchIcon(), "File Filter", profile.AutoLatestFilter, true)
			}
		}

		// 5. Situational: Multiplayer
		if profile.LaunchMode == "multiplayer" || profile.ServerIpPort != "" {
			addDetail(detailsContainer, theme.ComputerIcon(), "Server Address", profile.ServerIpPort, false)

			if profile.ServerCompanyNumber != "" {
				addDetail(detailsContainer, theme.LoginIcon(), "Company Slot", profile.ServerCompanyNumber, false)
			}
			if profile.ServerPassword != "" || profile.ServerCompanyPassword != "" {
				authInfo := ""
				if profile.ServerPassword != "" {
					authInfo += "Server password set. "
				}
				if profile.ServerCompanyPassword != "" {
					authInfo += "Company password set."
				}
				addDetail(detailsContainer, theme.ConfirmIcon(), "Authentication", authInfo, false)
			}
		}

		// 6. Advanced: Extra Args
		if profile.ExtraArgs != "" {
			addDetail(detailsContainer, theme.InfoIcon(), "Advanced Arguments", profile.ExtraArgs, true)
		}

		detailsContainer.Refresh()
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
	}

	newBtn := widget.NewButtonWithIcon("New Profile", theme.ContentAddIcon(), func() {
		um.showProfileEditor(-1)
	})
	newBtn.Importance = widget.HighImportance

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
			} else {
				dialog.ShowError(fmt.Errorf("cannot delete the last profile"), um.window)
			}

		}
	})

	runBtn = widget.NewButton("Run Selected", runSelected)
	runBtn.Importance = widget.HighImportance

	seeLogsBtn = widget.NewButton("See Logs", func() {
		um.showLogView(-1)
	})

	settingsBtn := widget.NewButton("Settings", func() {
		um.showSettingsView()
	})

	actionsContent := container.NewVBox(
		runBtn,
		container.NewGridWithColumns(3, editBtn, duplicateBtn, deleteBtn),
	)

	leftPanelObj := container.NewBorder(
		widget.NewCard("Profiles", "", widget.NewLabel("Select a profile to edit or run it.")),
		container.NewPadded(container.NewVBox(widget.NewSeparator(), newBtn, widget.NewSeparator(), seeLogsBtn, settingsBtn)),


		nil,
		nil,
		profileList,
	)
	leftPanel := newThemedBox(colorNameSidebar, leftPanelObj)

	detailsContent := container.NewVScroll(container.NewPadded(detailsContainer))

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
	var themeToggleBtn *widget.Button
	themeToggleBtn = widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), func() {
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(themeToggleBtn)
		pos.Y += themeToggleBtn.Size().Height
		um.showThemeCustomizer(pos)
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

func (um *UIManager) showThemeCustomizer(pos fyne.Position) {
	apply := func(v string, presetIdx int) {
		um.config.ThemeVariant = v
		um.config.AccentPreset = presetIdx
		if pt, ok := um.app.Settings().Theme().(*launcherTheme); ok {
			pt.UpdateAccent(presetIdx, v)
		}
		_ = SaveConfig(um.configPath, um.config)
	}

	modeSelect := um.newSegmentedRadio([]string{"Light", "Dark"}, strings.Title(um.config.ThemeVariant), func(s string) {
		apply(strings.ToLower(s), um.config.AccentPreset)
	})

	colorGrid := container.NewGridWithColumns(4)
	colorButtons := make([]*canvas.Rectangle, len(themePresets))

	updateButtons := func() {
		for i, rect := range colorButtons {
			if i == um.config.AccentPreset {
				rect.StrokeColor = theme.PrimaryColor()
				rect.StrokeWidth = 3
			} else {
				rect.StrokeColor = color.Transparent
				rect.StrokeWidth = 0
			}
			rect.Refresh()
		}
	}

	for i, p := range themePresets {
		idx := i
		// Show the color corresponding to the current theme variant in the preview circle
		hex := p.darkHex
		if um.config.ThemeVariant == "light" {
			hex = p.lightHex
		}
		c, _ := parseHexColor(hex)

		rect := canvas.NewRectangle(c)
		rect.SetMinSize(fyne.NewSize(36, 36))
		rect.CornerRadius = 4
		colorButtons[idx] = rect

		btn := widget.NewButton("", func() {
			apply(um.config.ThemeVariant, idx)
			updateButtons()
		})
		btn.Importance = widget.LowImportance

		colorGrid.Add(container.NewStack(rect, btn))
	}

	updateButtons()

	content := container.NewVBox(
		widget.NewLabel("Mode"),
		modeSelect.container,
		widget.NewLabel("Accent Color"),
		colorGrid,
	)

	widget.NewPopUp(container.NewPadded(content), um.window.Canvas()).ShowAtPosition(pos)
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

	versionEntry := widget.NewSelectEntry(um.cachedVersions)
	if len(um.cachedVersions) == 0 {
		versionEntry.SetOptions([]string{"latest"})
		go func() {
			versions, err := FetchAvailableVersions(um.config)
			if err == nil && len(versions) > 0 {
				um.cachedVersions = versions
				versionEntry.SetOptions(versions)
				versionEntry.Refresh()
			}
		}()
	}
	// Fallback if the profile's version isn't in the options yet
	if profile.Version != "" {
		versionEntry.SetText(profile.Version)
	} else {
		versionEntry.SetText("latest")
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

	// Dynamic visibility functions defined early to avoid 'undefined' errors
	folderInstructions := widget.NewLabel("")
	folderInstructions.Wrapping = fyne.TextWrapWord
	updateFolderInstructions := func(filterLabel string) {
		switch filterLabel {
		case "Saves Only":
			folderInstructions.SetText("Select a folder; the launcher will auto-load the most recent save inside it:")
		case "Scenarios Only":
			folderInstructions.SetText("Select a folder; the launcher will auto-load the most recent scenario inside it:")
		default: // Saves & Scenarios
			folderInstructions.SetText("Select a folder; the launcher will auto-load the most recent save or scenario inside it:")
		}
	}

	// Option visibility logic
	fileOption := container.NewVBox()
	folderOption := container.NewVBox()
	multiplayerOption := container.NewVBox()
	pathManualOption := container.NewVBox()
	folderFilterOption := container.NewVBox()
	normalOption := widget.NewLabel("The game will launch normally to the main menu.")
	optionsStack := container.NewVBox(normalOption, fileOption, folderOption, multiplayerOption, pathManualOption, folderFilterOption)

	updateVisibility := func(mode string) {
		normalOption.Hide()
		fileOption.Hide()
		folderOption.Hide()
		multiplayerOption.Hide()
		pathManualOption.Hide()
		folderFilterOption.Hide()
		switch mode {
		case "Main Menu":
			normalOption.Show()
		case "Load File":
			fileOption.Show()
			pathManualOption.Show()
		case "Latest in Folder":
			folderOption.Show()
			pathManualOption.Show()
			folderFilterOption.Show()
		case "Multiplayer":
			multiplayerOption.Show()
		}
		optionsStack.Refresh()
	}

	// Launch Mode selection
	modeMap := map[string]string{
		"Main Menu":        "",
		"Load File":        "file",
		"Latest in Folder": "folder",
		"Multiplayer":      "multiplayer",
	}
	revModeMap := map[string]string{
		"":            "Main Menu",
		"file":        "Load File",
		"folder":      "Latest in Folder",
		"multiplayer": "Multiplayer",
	}

	modeSelect := um.newSegmentedRadio([]string{"Main Menu", "Load File", "Latest in Folder", "Multiplayer"}, revModeMap[profile.LaunchMode], func(s string) {
		updateVisibility(s)
	})

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

	extraArgsEntry := widget.NewMultiLineEntry()
	extraArgsEntry.SetText(profile.ExtraArgs)
	extraArgsEntry.SetPlaceHolder("Example: -d 3 -r 1920x1080")
	extraArgsEntry.Wrapping = fyne.TextWrapWord

	// Auto-Latest Filter selection
	filterLabelMap := map[string]string{
		"both": "Saves & Scenarios",
		"sav":  "Saves Only",
		"scn":  "Scenarios Only",
	}
	revFilterLabelMap := map[string]string{
		"Saves & Scenarios": "both",
		"Saves Only":        "sav",
		"Scenarios Only":    "scn",
	}

	initialFilter := profile.AutoLatestFilter
	if initialFilter == "" {
		initialFilter = "both"
	}

	autoLatestFilterRadio := um.newSegmentedRadio([]string{"Saves & Scenarios", "Saves Only", "Scenarios Only"}, filterLabelMap[initialFilter], func(s string) {
		updateFolderInstructions(s)
	})
	updateFolderInstructions(autoLatestFilterRadio.Selected())

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
					// 1. Try relative to Docs/save (preferred)
					saveBase := filepath.Join(um.config.DocsBasePath, "save")
					rel, err := filepath.Rel(saveBase, path)
					if err == nil && !strings.HasPrefix(rel, "..") {
						path = rel
					} else {
						// 2. Fallback: try relative to Docs Base (e.g. if file is in another subfolder)
						rel, err := filepath.Rel(um.config.DocsBasePath, path)
						if err == nil && !strings.HasPrefix(rel, "..") {
							path = rel
						}
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
					// 1. Try relative to Docs/save (preferred)
					saveBase := filepath.Join(um.config.DocsBasePath, "save")
					rel, err := filepath.Rel(saveBase, path)
					if err == nil && !strings.HasPrefix(rel, "..") {
						path = rel
					} else {
						// 2. Fallback: try relative to Docs Base
						rel, err := filepath.Rel(um.config.DocsBasePath, path)
						if err == nil && !strings.HasPrefix(rel, "..") {
							path = rel
						}
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

	fileOption.Objects = []fyne.CanvasObject{
		widget.NewLabel("Select a specific save or scenario file to load:"),
		browseFileBtn,
	}
	folderOption.Objects = []fyne.CanvasObject{
		folderInstructions,
		browseFolderBtn,
	}
	folderFilterOption.Objects = []fyne.CanvasObject{
		widget.NewSeparator(),
		widget.NewLabel("File Type Filter"),
		container.NewHBox(autoLatestFilterRadio.container),
	}
	pathManualOption.Objects = []fyne.CanvasObject{
		widget.NewSeparator(),
		widget.NewLabel("Path (Relative or Absolute)"),
		savePathEntry,
	}
	multiplayerOption.Objects = []fyne.CanvasObject{
		sectionTitle("Server Connection"),
		widget.NewLabel("Server IP:Port"), ipPortEntry,
		widget.NewLabel("Server Password"), serverPassEntry,
		widget.NewSeparator(),
		sectionTitle("Company Details"),
		container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Company Number"), companyNumEntry),
			container.NewVBox(widget.NewLabel("Company Password"), companyPassEntry),
		),
	}

	updateVisibility(revModeMap[profile.LaunchMode])

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord
	var editDialog *widget.PopUp


	validate := func() (bool, string) {
		name := strings.TrimSpace(nameEntry.Text)
		if name == "" {
			return false, "Profile name is required."
		}

		for i, p := range um.config.Profiles {
			if i != profileIdx && strings.EqualFold(strings.TrimSpace(p.Name), name) {
				return false, "A profile with this name already exists."
			}
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
		if profile.Version == "latest" {
			profile.Version = ""
		}

		rawSavePath := strings.TrimSpace(savePathEntry.Text)
		// Strip leading "save/" or "save\" (case-insensitive)
		for {
			lowered := strings.ToLower(rawSavePath)
			if strings.HasPrefix(lowered, "save/") || strings.HasPrefix(lowered, "save\\") {
				rawSavePath = rawSavePath[5:]
				continue
			}
			break
		}
		profile.SavePath = rawSavePath

		profile.ServerIpPort = strings.TrimSpace(ipPortEntry.Text)

		profile.ServerPassword = serverPassEntry.Text
		profile.ServerCompanyNumber = strings.TrimSpace(companyNumEntry.Text)
		profile.ServerCompanyPassword = companyPassEntry.Text
		profile.LaunchMode = modeMap[modeSelect.Selected()]
		profile.AutoLatestFilter = revFilterLabelMap[autoLatestFilterRadio.Selected()]

		profile.ExtraArgs = strings.TrimSpace(extraArgsEntry.Text)
		switch profile.LaunchMode {
		case "":
			profile.SavePath = ""
			profile.ServerIpPort = ""
		case "multiplayer":
			profile.SavePath = ""
		default:
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

	generalTab := container.NewTabItemWithIcon("General Options", theme.InfoIcon(), container.NewVBox(
		sectionTitle("Identity"),
		widget.NewLabel("Name"), nameEntry,
		widget.NewLabel("JGRPP Version"), versionEntry,
	))

	launchTab := container.NewTabItemWithIcon("Launch Options", theme.MediaPlayIcon(), container.NewVBox(
		sectionTitle("How should the game start?"),
		modeSelect.container,
		widget.NewSeparator(),
		optionsStack,
	))

	advancedTab := container.NewTabItemWithIcon("Advanced Options", theme.SettingsIcon(), container.NewVBox(
		sectionTitle("Custom Command Line Arguments"),
		widget.NewLabel("Specify extra flags to pass to the OpenTTD executable:"),
		extraArgsEntry,
	))

	tabs := container.NewAppTabs(generalTab, launchTab, advancedTab)
	tabs.SetTabLocation(container.TabLocationTop)

	form := container.NewVBox(
		statusLabel,
		tabs,
	)

	launchTab.Content = container.NewVBox(
		sectionTitle("How should the game start?"),
		modeSelect.container,
		widget.NewSeparator(),
		optionsStack,
	)

	saveBtn = widget.NewButton("Save", func() { saveProfile(false) })
	saveAndRunBtn = widget.NewButton("Save & Run", func() { saveProfile(true) })
	if isNew {
		saveAndRunBtn.SetText("Create & Run")
	}
	cancelBtn := widget.NewButton("Cancel", func() {
		editDialog.Hide()
	})

	toolbar := container.NewCenter(container.NewHBox(
		container.NewPadded(cancelBtn), 
		container.NewPadded(saveBtn), 
		container.NewPadded(saveAndRunBtn),
	))


	
	title := widget.NewLabel("Edit Profile")
	title.TextStyle = fyne.TextStyle{Bold: true}

	content := container.NewBorder(
		container.NewPadded(title),
		container.NewPadded(toolbar),
		nil,
		nil,
		container.NewPadded(form),
	)


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

	editDialog = widget.NewModalPopUp(content, um.window.Canvas())
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

	autoOpenLogCheck := widget.NewCheck("Auto-open log panel when game starts", nil)
	autoOpenLogCheck.SetChecked(um.config.AutoOpenLog)

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
		container.NewGridWithColumns(1, autoCloseCheck, autoOpenLogCheck, verboseCheck),
	))

	advancedTab := container.NewTabItemWithIcon("Advanced", theme.SettingsIcon(), container.NewVBox(
		sectionTitle("System Settings"),
		widget.NewLabel("GitHub API URL"), githubApiUrlEntry,
		widget.NewLabel("OS Type (detected automatically)"), osTypeEntry,
	))

	tabs := container.NewAppTabs(pathsTab, behaviorTab, advancedTab)
	tabs.SetTabLocation(container.TabLocationTop)

	var settingsDialog *widget.PopUp


	saveBtn := widget.NewButton("Save Settings", func() {
		um.config.ParentDir = parentDirEntry.Text
		um.config.DocsBasePath = docsBasePathEntry.Text
		um.config.GithubApiUrl = githubApiUrlEntry.Text
		um.config.OSType = osTypeEntry.Text
		um.config.AutoCloseOnStart = autoCloseCheck.Checked
		um.config.AutoOpenLog = autoOpenLogCheck.Checked
		um.config.Verbose = verboseCheck.Checked

		_ = SaveConfig(um.configPath, um.config)
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

	settingsDialog = widget.NewModalPopUp(content, um.window.Canvas())
	settingsDialog.Resize(fyne.NewSize(850, 600))
	settingsDialog.Show()

}

// showLogView shows a window with logs while launching a profile
func (um *UIManager) showLogView(profileIdx int) {
	var profile Profile
	isLaunch := profileIdx >= 0
	if isLaunch {
		profile = um.config.Profiles[profileIdx]
	}

	statusBinding := binding.NewString()
	_ = statusBinding.Set("Preparing launch")

	var summaryObj fyne.CanvasObject
	if isLaunch {
		summary := widget.NewLabel(fmt.Sprintf("Profile: %s\nVersion: %s\nSave path: %s\nServer: %s", profile.Name, valueOrDefault(profile.Version, "latest"), valueOrDefault(profile.SavePath, "(none)"), valueOrDefault(profile.ServerIpPort, "(none)")))
		summary.Wrapping = fyne.TextWrapWord
		summaryObj = widget.NewCard("Launching", "Current launch context", summary)
	}

	statusLabel := widget.NewLabelWithData(statusBinding)
	statusLabel.Wrapping = fyne.TextWrapWord
	statusObj := widget.NewCard("Status", "Background operations", statusLabel)
	if !isLaunch {
		statusObj.Hide()
	}

	// Create log text widget using data binding (thread-safe)
	logBinding := binding.NewString()
	logLabel := widget.NewLabelWithData(logBinding)
	logLabel.Wrapping = fyne.TextWrapWord

	logBox := container.NewVScroll(logLabel)
	logBox.SetMinSize(fyne.NewSize(600, 400))

	// Update the log display whenever logger changes
	updateLogDisplay := func() {
		logs := um.logger.GetAll()
		text := ""
		for _, line := range logs {
			text += line + "\n"
		}
		_ = logBinding.Set(text)
		logBox.ScrollToBottom()
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

	closeBtn := widget.NewButton("Return to Main", func() {
		select {
		case <-done:
		default:
			close(done)
		}
		um.window.SetContent(um.makeMainView())
	})

	copyBtn := widget.NewButtonWithIcon("Copy to Clipboard", theme.ContentCopyIcon(), func() {
		logs := um.logger.GetAll()
		text := ""
		for _, line := range logs {
			text += line + "\n"
		}
		um.window.Clipboard().SetContent(text)
		um.showToast("Logs copied to clipboard!")
	})

	top := container.NewVBox()
	if isLaunch {
		top.Add(summaryObj)
		top.Add(statusObj)
	}

	content := container.NewBorder(
		top,
		container.NewHBox(closeBtn, copyBtn),
		nil,
		nil,
		logBox,
	)

	um.window.SetContent(content)

	// Launch OpenTTD in background if requested
	if isLaunch {
		go um.launchProfile(profile, func(status string) {
			_ = statusBinding.Set(status)
		}, nil)
	}
}

// showToast shows a temporary notification at the bottom of the window
func (um *UIManager) showToast(message string) {
	toast := widget.NewLabel(message)
	toast.Alignment = fyne.TextAlignCenter

	// Use a popup or a dedicated area? For simplicity, we'll use a short-lived dialog-like overlay
	// but Fyne's dialogs are modal. We'll use a custom PopUp.
	content := container.NewPadded(toast)
	pop := widget.NewPopUp(content, um.window.Canvas())

	// Position at bottom center
	size := um.window.Content().Size()
	pop.ShowAtPosition(fyne.NewPos(size.Width/2-100, size.Height-60))

	go func() {
		time.Sleep(3 * time.Second)
		pop.Hide()
	}()
}

// launchProfile launches OpenTTD with the specified profile
func (um *UIManager) launchProfile(profile Profile, updateStatus func(status string), onError func()) {

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
				if onError != nil {
					onError()
				}
				return

			}
			um.LogVerbose(fmt.Sprintf("Using latest local version folder: %s", versionFolder))
			if updateStatus != nil {
				updateStatus("Starting OpenTTD from latest local installation")
			}
			ExecuteOpenTTD(versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, profile.LaunchMode, profile.ExtraArgs, um.logger, um)
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
	ExecuteOpenTTD(versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, profile.LaunchMode, profile.ExtraArgs, um.logger, um)
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

// --- Custom Launcher Theming ---

const (
	colorNameSidebar fyne.ThemeColorName = "launcherSidebar"
	colorNameContent fyne.ThemeColorName = "launcherContent"
	colorNameHeader  fyne.ThemeColorName = "launcherHeader"
)

var themePresets = []struct {
	name     string
	lightHex string
	darkHex  string
}{
	{"Green", "#2D912D", "#3D993D"},
	{"Orange", "#E67300", "#FF8000"},
	{"Red", "#B71C1C", "#D32F2F"},
	{"Blue", "#1565C0", "#1976D2"},
	{"Purple", "#6A1B9A", "#7B1FA2"},
	{"Teal", "#00695C", "#00796B"},
	{"Gold", "#F9A825", "#E6A700"},
	{"Slate", "#37474F", "#455A64"},
}

type launcherTheme struct {
	fyne.Theme
	overrideVariant *fyne.ThemeVariant
	accentDark      color.NRGBA
	accentLight     color.NRGBA
}

func (p *launcherTheme) UpdateAccent(presetIdx int, variant string) {
	preset := themePresets[presetIdx]
	light, _ := parseHexColor(preset.lightHex)
	dark, _ := parseHexColor(preset.darkHex)
	p.accentLight = light
	p.accentDark = dark

	if variant == "light" {
		v := theme.VariantLight
		p.overrideVariant = &v
	} else if variant == "dark" {
		v := theme.VariantDark
		p.overrideVariant = &v
	} else {
		p.overrideVariant = nil
	}
	fyne.CurrentApp().Settings().SetTheme(p)
}

func (p *launcherTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if p.overrideVariant != nil {
		variant = *p.overrideVariant
	}

	accent := p.accentLight
	if variant == theme.VariantDark {
		accent = p.accentDark
	}

	if variant == theme.VariantDark {
		switch name {
		case theme.ColorNameBackground, theme.ColorNameOverlayBackground:
			return color.NRGBA{R: 25, G: 25, B: 25, A: 255} // Neutral Dark BG
		case theme.ColorNameInputBackground:
			return color.NRGBA{R: 32, G: 32, B: 32, A: 255} // Neutral Dark Input
		case colorNameSidebar:
			return color.NRGBA{R: 19, G: 19, B: 19, A: 255} // Neutral Dark Sidebar
		case colorNameContent:
			return color.NRGBA{R: 29, G: 29, B: 29, A: 255} // Neutral Dark Content
		case colorNameHeader:
			return color.NRGBA{R: 35, G: 35, B: 35, A: 255} // Neutral Dark Header

		case theme.ColorNameSelection:
			return withAlpha(accent, 115) // 45% Opacity
		case theme.ColorNameHover:
			return withAlpha(accent, 51) // 20% Opacity
		case theme.ColorNamePrimary:
			return accent
		}
	} else {
		switch name {
		case theme.ColorNameBackground, theme.ColorNameOverlayBackground:
			return color.NRGBA{R: 246, G: 246, B: 246, A: 255}
		case theme.ColorNameInputBackground:
			return color.NRGBA{R: 238, G: 238, B: 238, A: 255}
		case colorNameSidebar:
			return color.NRGBA{R: 236, G: 236, B: 236, A: 255}
		case colorNameContent:
			return color.NRGBA{R: 250, G: 250, B: 250, A: 255}
		case colorNameHeader:
			return color.NRGBA{R: 224, G: 224, B: 224, A: 255}
		case theme.ColorNameSelection:
			return withAlpha(accent, 115) // 45% Opacity
		case theme.ColorNameHover:
			return withAlpha(accent, 51) // 20% Opacity
		case theme.ColorNamePrimary:
			return accent
		}
	}
	return p.Theme.Color(name, variant)
}

func withAlpha(c color.Color, alpha uint8) color.Color {
	if c == nil {
		return color.Transparent
	}
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: alpha}
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

// --- Segmented Radio Button Group ---

type segmentedRadio struct {
	container *fyne.Container
	selected  string
	options   []string
	buttons   []*widget.Button
	onChanged func(string)
}

func (um *UIManager) newSegmentedRadio(options []string, initial string, onChanged func(string)) *segmentedRadio {
	s := &segmentedRadio{
		options:   options,
		selected:  initial,
		onChanged: onChanged,
		buttons:   make([]*widget.Button, len(options)),
	}

	for i, opt := range options {
		label := opt
		btn := widget.NewButton(label, func() {
			s.SetSelected(label)
			if s.onChanged != nil {
				s.onChanged(label)
			}
		})
		if label == initial {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.LowImportance
		}
		s.buttons[i] = btn
	}

	// Create a grid so all blocks are equal width
	s.container = container.NewGridWithColumns(len(options))
	for _, b := range s.buttons {
		s.container.Add(b)
	}

	return s
}

func (s *segmentedRadio) SetSelected(label string) {
	s.selected = label
	for _, b := range s.buttons {
		if b.Text == label {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.LowImportance
		}
		b.Refresh()
	}
}

func (s *segmentedRadio) Selected() string {
	return s.selected
}

func (s *segmentedRadio) Hide() {
	s.container.Hide()
}

func (s *segmentedRadio) Show() {
	s.container.Show()
}

func (s *segmentedRadio) Refresh() {
	s.container.Refresh()
}

// --- Color Helpers ---

func parseHexColor(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("invalid hex length")
	}
	r, err := strconv.ParseUint(s[0:2], 16, 8)
	if err != nil {
		return color.NRGBA{}, err
	}
	g, err := strconv.ParseUint(s[2:4], 16, 8)
	if err != nil {
		return color.NRGBA{}, err
	}
	b, err := strconv.ParseUint(s[4:6], 16, 8)
	if err != nil {
		return color.NRGBA{}, err
	}
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil
}
