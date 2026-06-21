package fyne

import (
	"context"
	"fmt"
	"image/color"
	neturl "net/url"
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
	fyneadvancedlist "github.com/dweymouth/fyne-advanced-list"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

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

// searchEntry is an Entry that invokes onEscape when Escape is pressed.
type searchEntry struct {
	widget.Entry
	onEscape func()
}

func newSearchEntry() *searchEntry {
	e := &searchEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *searchEntry) TypedKey(key *fyne.KeyEvent) {
	if key.Name == fyne.KeyEscape && e.onEscape != nil {
		e.onEscape()
		return
	}
	e.Entry.TypedKey(key)
}

// makeOnboardingView creates the first-run configuration screen
func (um *UIManager) makeOnboardingView() fyne.CanvasObject {
	welcomeLabel := widget.NewLabel("Set up RunTTD")
	welcomeLabel.TextStyle = fyne.TextStyle{Bold: true, Italic: false}
	welcomeLabel.Alignment = fyne.TextAlignCenter

	subtitle := NewSectionDescription("Confirm your installation folders and preferences to get started.")
	subtitle.Alignment = fyne.TextAlignCenter

	// --- Installation paths ---
	parentDirEntry := widget.NewEntry()
	parentDirEntry.SetText(um.Config.ParentDir)
	parentDirEntry.SetPlaceHolder("Folder where OpenTTD game files / executables will be automatically installed")

	parentDirBtn := widget.NewButton("Browse...", func() {
		um.browseDirectory(parentDirEntry, "Select Parent Directory", "Parent Directory")
	})

	docsBasePathEntry := widget.NewEntry()
	docsBasePathEntry.SetText(um.Config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	validationIcon := widget.NewIcon(theme.CancelIcon())
	validationIcon.Hide()

	docsBasePathBtn := widget.NewButton("Browse...", func() {
		um.browseDirectory(docsBasePathEntry, "Select Docs Base Path", "Docs Base Path")
	})

	// --- Preferences ---
	// Default Client is required: the dropdown starts unselected unless the
	// config already has a value, and Continue is gated on a selection below.
	defaultClientSelect := widget.NewSelect(defaultClientOptions, func(string) {})
	if label, ok := revDefaultClientMap[um.Config.DefaultClient]; ok {
		defaultClientSelect.SetSelected(label)
	}

	subfolderCheck, subfolderGroup := NewLabeledCheckWithDescription(
		"Organize downloaded clients into per-client subfolders",
		"Keeps each client's downloaded files in a separate folder, instead of all sharing the parent folder. "+
			"Easiest to choose now, before anything is downloaded; you can change it later in Settings.",
		um.Config.SubfolderPerClient,
	)

	autoCloseCheck, autoCloseGroup := NewLabeledCheckWithDescription(
		"Auto-close launcher when OpenTTD starts",
		"Hides the launcher once the game opens. You can change it later in Settings.",
		um.Config.AutoCloseOnStart,
	)

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	// validate is a backstop for the Continue handler; the live hint and the
	// disabled button (see updateState) are the primary guard. All three inputs
	// (both paths and a default client) are required.
	validate := func() bool {
		return strings.TrimSpace(parentDirEntry.Text) != "" &&
			strings.TrimSpace(docsBasePathEntry.Text) != "" &&
			defaultClientSelect.Selected != ""
	}

	continueBtn := widget.NewButton("Continue", func() {
		if !validate() {
			return
		}

		um.Config.ParentDir = strings.TrimSpace(parentDirEntry.Text)
		um.Config.DocsBasePath = strings.TrimSpace(docsBasePathEntry.Text)
		um.Config.SubfolderPerClient = subfolderCheck.Checked
		um.Config.AutoCloseOnStart = autoCloseCheck.Checked
		// validate() guarantees a selection, so this lookup always hits.
		if mapped, ok := defaultClientMap[defaultClientSelect.Selected]; ok {
			um.Config.DefaultClient = mapped
		}
		um.Config.FirstRun = false

		_ = domain.SaveConfig(um.ConfigPath, um.Config)

		um.Window.SetContent(um.makeMainView())
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

	// updateState keeps the Continue button and the status hint in sync with the
	// required inputs (both paths and a default client). Continue stays disabled
	// until all are set, so the hint explains what is still needed rather than
	// leaving the button inertly greyed out.
	updateState := func(_ string) {
		switch {
		case strings.TrimSpace(parentDirEntry.Text) == "":
			statusLabel.SetText("Enter a Parent Directory to continue.")
			continueBtn.Disable()
		case strings.TrimSpace(docsBasePathEntry.Text) == "":
			statusLabel.SetText("Enter a Docs Base Path to continue.")
			continueBtn.Disable()
		case defaultClientSelect.Selected == "":
			statusLabel.SetText("Choose a default client to continue.")
			continueBtn.Disable()
		default:
			statusLabel.SetText("")
			continueBtn.Enable()
		}
	}
	parentDirEntry.OnChanged = updateState
	docsBasePathEntry.OnChanged = func(s string) {
		updateState(s)
		updateDocsValidation(s)
	}
	defaultClientSelect.OnChanged = updateState
	updateState("")
	updateDocsValidation(docsBasePathEntry.Text)

	form := container.NewVBox(
		welcomeLabel,
		subtitle,
		NewSectionHeader("Installation Paths"),
		NewLabeledField(
			"Parent Directory (required)",
			"RunTTD downloads, installs, and removes game clients here.",
			container.NewBorder(nil, nil, nil, parentDirBtn, parentDirEntry),
		),
		NewLabeledField(
			"Docs Base Path (required)",
			"Where your saves and configuration (openttd.cfg) live. RunTTD reads from here but never modifies your files.",
			container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, docsBasePathBtn), docsBasePathEntry),
		),
		NewSectionHeader("Preferences"),
		NewLabeledField(
			"Default Client (required)",
			"The client new profiles use by default. You can change it per profile.",
			defaultClientSelect,
		),
		subfolderGroup,
		autoCloseGroup,
	)

	onboardingScroll := container.NewVScroll(form)
	return container.NewBorder(
		nil,
		container.NewPadded(container.NewVBox(statusLabel, container.NewHBox(continueBtn))),
		nil,
		nil,
		container.NewPadded(onboardingScroll),
	)
}

// makeMainView creates the main profile selection view
func (um *UIManager) makeMainView() fyne.CanvasObject {
	selectedIdx := indexOfProfileByName(um.Config.Profiles, um.SelectedProfileName)
	detailsContainer := container.NewVBox()

	// visibleIdx maps a displayed row to the real Config.Profiles index (identity
	// when unfiltered); quick-launch always uses the real index, so filtering is visual.
	visibleIdx := make([]int, len(um.Config.Profiles))
	for i := range visibleIdx {
		visibleIdx[i] = i
	}
	filterText := ""

	// displayPos is forward-declared so handleRowTap can use it; assigned below.
	var displayPos func(real int) int

	selectionHint := widget.NewLabel("Tip: Press 1-9, or 0 to quick launch. Select a profile and press Enter / double-click.")
	selectionHint.Wrapping = fyne.TextWrapWord

	var profileList *fyneadvancedlist.List
	var refreshDetails func()

	var runBtn, editBtn, duplicateBtn, deleteBtn, seeLogsBtn *widget.Button
	var updateButtonStates func()

	// launchIndex starts the profile at idx, honoring the AutoOpenLog setting:
	// either open the log view or launch in the background with toast + button
	// feedback. Used by the Run button, Enter, and the digit quick-launch keys so
	// all three behave identically.
	launchIndex := func(idx int) {
		if idx < 0 || idx >= len(um.Config.Profiles) {
			dialog.ShowError(fmt.Errorf("select a profile to launch"), um.Window)
			return
		}
		if um.Config.AutoOpenLog {
			um.showLogView(idx)
			return
		}
		// Background launch with feedback
		oldText := runBtn.Text
		runBtn.SetText("Launching...")
		runBtn.Disable()

		profile := um.Config.Profiles[idx]
		um.showToast(fmt.Sprintf("Starting %s...", profile.Name))

		go func() {
			um.launchProfile(profile, nil, func() {
				um.showLogView(idx)
			})

			time.Sleep(1500 * time.Millisecond)
			fyne.Do(func() {
				runBtn.SetText(oldText)
				updateButtonStates()
			})
		}()
	}

	runSelected := func() {
		launchIndex(selectedIdx)
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
			if um.Logger.Len() > 0 {
				seeLogsBtn.Enable()
			} else {
				seeLogsBtn.Disable()
			}
		}
	}

	selectProfile := func(idx int) {
		if idx < 0 || idx >= len(um.Config.Profiles) {
			selectedIdx = -1
			um.SelectedProfileName = ""
			return
		}

		selectedIdx = idx
		um.SelectedProfileName = um.Config.Profiles[selectedIdx].Name
		refreshDetails()
	}

	handleRowTap := func(idx int) {
		now := time.Now()
		if idx == selectedIdx && idx == um.LastListSelectID && now.Sub(um.LastListSelectAt) < 450*time.Millisecond {
			um.LastListSelectAt = time.Time{}
			runSelected()
			return
		}

		um.LastListSelectID = idx
		um.LastListSelectAt = now
		if d := displayPos(idx); d >= 0 {
			profileList.Select(widget.ListItemID(d))
		} else {
			profileList.UnselectAll()
		}
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

		row := container.NewBorder(nil, nil, iconObj, nil, container.NewVBox(title, val))
		c.Add(container.NewPadded(row))
	}

	refreshDetails = func() {
		updateButtonStates()
		detailsContainer.Objects = nil

		if selectedIdx < 0 || selectedIdx >= len(um.Config.Profiles) {
			welcomeTitle := widget.NewLabel("Welcome to RunTTD")
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

		profile := um.Config.Profiles[selectedIdx]

		nameLabel := widget.NewLabel(profile.Name)
		nameLabel.TextStyle = fyne.TextStyle{Bold: true}
		detailsContainer.Add(nameLabel)

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
		detailsContainer.Add(container.NewPadded(intentLabel))
		detailsContainer.Add(widget.NewSeparator())

		if profile.Client == "custom" {
			folder := strings.TrimSpace(profile.CustomExecutablePath)
			if folder == "" {
				folder = "(not set)"
			}
			addDetail(detailsContainer, theme.FolderIcon(), "Executable Folder", folder, true)
		} else {
			versionText := profile.Version
			if versionText == "" {
				versionText = "latest"
			}
			addDetail(detailsContainer, theme.SettingsIcon(), "Version", versionText, false)
		}

		if strings.TrimSpace(profile.ConfigFilePath) != "" {
			addDetail(detailsContainer, theme.FileIcon(), "Config Override", profile.ConfigFilePath, true)
		}
		if profile.NoConfigSave {
			addDetail(detailsContainer, theme.ConfirmIcon(), "No Config Save", "Enabled", false)
		}

		if profile.NewGRFScanMode != "" {
			var desc string
			switch strings.ToUpper(profile.NewGRFScanMode) {
			case "Q":
				desc = "Skip NewGRF loading at startup"
			case "QQ":
				desc = "Disable all NewGRF scanning/loading (session-wide)"
			}
			if desc != "" {
				addDetail(detailsContainer, theme.InfoIcon(), "NewGRF Scan", desc, false)
			}
		}

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

		if profile.ExtraArgs != "" {
			addDetail(detailsContainer, theme.InfoIcon(), "Advanced Arguments", profile.ExtraArgs, true)
		}

		detailsContainer.Refresh()
	}

	recomputeVisible := func() {
		needle := strings.ToLower(strings.TrimSpace(filterText))
		visibleIdx = visibleIdx[:0]
		for i, p := range um.Config.Profiles {
			if needle == "" || strings.Contains(strings.ToLower(p.Name), needle) {
				visibleIdx = append(visibleIdx, i)
			}
		}
	}
	recomputeVisible()

	// displayPos returns the filtered row position of a real index, or -1 if hidden.
	displayPos = func(real int) int {
		for d, r := range visibleIdx {
			if r == real {
				return d
			}
		}
		return -1
	}

	profileList = fyneadvancedlist.NewList(
		func() int { return len(visibleIdx) },
		func() fyne.CanvasObject {
			btn := newRightClickButton(nil, nil)

			nameLabel := widget.NewLabel("")
			versionLabel := widget.NewLabel("")
			versionLabel.Alignment = fyne.TextAlignTrailing
			versionLabel.TextStyle = fyne.TextStyle{Italic: true}

			dragIcon := widget.NewIcon(theme.MenuIcon())

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

			if int(i) < len(visibleIdx) {
				real := visibleIdx[i]
				profile := um.Config.Profiles[real]
				clientTag := shortClientLabel(profile.Client, um.Config.DefaultClient)
				var versionText string
				if profile.Client == "custom" {
					versionText = "Custom"
				} else {
					version := profile.Version
					if version == "" {
						version = "latest"
					}
					versionText = clientTag + " · " + version
				}
				nameLabel.SetText(fmt.Sprintf("%d. %s", real+1, profile.Name))
				versionLabel.SetText(versionText)
				idx := real
				btn.OnTapped = func() {
					handleRowTap(idx)
				}
				btn.onSecondaryTapped = func() {
					um.showProfileEditor(idx, false)
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

		if to > from {
			to--
		}

		profile := um.Config.Profiles[from]
		um.Config.Profiles = append(um.Config.Profiles[:from], um.Config.Profiles[from+1:]...)

		newProfiles := make([]domain.Profile, 0, len(um.Config.Profiles)+1)
		newProfiles = append(newProfiles, um.Config.Profiles[:to]...)
		newProfiles = append(newProfiles, profile)
		newProfiles = append(newProfiles, um.Config.Profiles[to:]...)
		um.Config.Profiles = newProfiles

		_ = domain.SaveConfig(um.ConfigPath, um.Config)
		profileList.Refresh()

		if from == selectedIdx {
			profileList.Select(widget.ListItemID(to))
		}
	}
	profileList.OnSelected = func(id widget.ListItemID) {
		if int(id) < len(visibleIdx) {
			selectProfile(visibleIdx[id])
		}
	}
	profileList.OnUnselected = func(_ widget.ListItemID) {
		selectProfile(-1)
		refreshDetails()
	}

	newBtn := widget.NewButtonWithIcon("New Profile", theme.ContentAddIcon(), func() {
		um.showProfileEditor(-1, true)
	})
	newBtn.Importance = widget.HighImportance

	editBtn = widget.NewButton("Edit", func() {
		if selectedIdx >= 0 {
			um.showProfileEditor(selectedIdx, false)
		} else {
			dialog.ShowError(fmt.Errorf("select a profile to edit"), um.Window)
		}
	})
	duplicateBtn = widget.NewButton("Duplicate", func() {
		if selectedIdx >= 0 {
			dup := um.Config.Profiles[selectedIdx]
			dup.Name = uniqueProfileName(um.Config.Profiles, dup.Name)
			um.Config.Profiles = append(um.Config.Profiles, dup)
			_ = domain.SaveConfig(um.ConfigPath, um.Config)

			selectedIdx = len(um.Config.Profiles) - 1
			um.SelectedProfileName = um.Config.Profiles[selectedIdx].Name
			recomputeVisible()
			profileList.Refresh()
			if d := displayPos(selectedIdx); d >= 0 {
				profileList.Select(widget.ListItemID(d))
			} else {
				profileList.UnselectAll()
			}
			refreshDetails()
		} else {
			dialog.ShowError(fmt.Errorf("select a profile to duplicate"), um.Window)
		}
	})
	deleteBtn = widget.NewButton("Delete", func() {
		if selectedIdx >= 0 {
			if len(um.Config.Profiles) > 1 {
				profileName := um.Config.Profiles[selectedIdx].Name
				dialog.NewConfirm(
					"Delete Profile",
					fmt.Sprintf("Are you sure you want to delete profile %q?", profileName),
					func(confirmed bool) {
						if !confirmed {
							return
						}

						um.Config.Profiles = append(um.Config.Profiles[:selectedIdx], um.Config.Profiles[selectedIdx+1:]...)
						_ = domain.SaveConfig(um.ConfigPath, um.Config)

						nextIdx := selectedIdx
						if nextIdx >= len(um.Config.Profiles) {
							nextIdx = len(um.Config.Profiles) - 1
						}

						selectedIdx = nextIdx
						um.SelectedProfileName = um.Config.Profiles[selectedIdx].Name
						recomputeVisible()
						profileList.Refresh()
						if d := displayPos(selectedIdx); d >= 0 {
							profileList.Select(widget.ListItemID(d))
						} else {
							profileList.UnselectAll()
						}
						refreshDetails()
					},
					um.Window,
				).Show()
			} else {
				dialog.ShowError(fmt.Errorf("cannot delete the last profile"), um.Window)
			}
		}
	})

	runBtn = widget.NewButton("Run Selected", runSelected)
	runBtn.Importance = widget.HighImportance

	seeLogsBtn = widget.NewButton("See Logs", func() {
		um.showLogView(-1)
	})

	settingsBtn := widget.NewButton("Global Settings", func() {
		um.showSettingsView()
	})

	manageInstallsBtn := widget.NewButton("Manage Installs", func() {
		um.showLibraryView()
	})

	actionsContent := container.NewVBox(
		runBtn,
		container.NewGridWithColumns(3, editBtn, duplicateBtn, deleteBtn),
	)

	emptyState := widget.NewLabel("No profiles match your search.")
	emptyState.Alignment = fyne.TextAlignCenter
	emptyState.Wrapping = fyne.TextWrapWord
	emptyState.Hide()

	updateEmptyState := func() {
		if len(visibleIdx) == 0 && strings.TrimSpace(filterText) != "" {
			emptyState.Show()
		} else {
			emptyState.Hide()
		}
	}

	searchEntry := newSearchEntry()
	searchEntry.SetPlaceHolder("Search profiles by name...")
	searchEntry.OnChanged = func(s string) {
		filterText = s
		recomputeVisible()
		// Reordering a filtered subset is ambiguous; only allow drag with no filter.
		profileList.EnableDragging = strings.TrimSpace(s) == ""
		profileList.UnselectAll()
		selectProfile(-1)
		refreshDetails()
		updateEmptyState()
		profileList.Refresh()
	}
	// Esc clears the filter and returns to the full list.
	searchEntry.onEscape = func() {
		if searchEntry.Text != "" {
			searchEntry.SetText("") // triggers OnChanged, which resets the filter
		}
	}

	leftPanelObj := container.NewBorder(
		widget.NewCard("Profiles", "", widget.NewLabel("Select a profile to edit or run it.")),
		container.NewPadded(container.NewVBox(widget.NewSeparator(), newBtn, widget.NewSeparator(), seeLogsBtn, manageInstallsBtn, settingsBtn)),
		nil,
		nil,
		container.NewBorder(container.NewPadded(searchEntry), nil, nil, nil, container.NewStack(profileList, emptyState)),
	)
	leftPanel := NewThemedBox(ColorNameSidebar, leftPanelObj)

	detailsContent := container.NewVScroll(container.NewPadded(detailsContainer))

	rightPanelObj := container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), actionsContent, container.NewPadded(selectionHint)),
		nil,
		nil,
		detailsContent,
	)
	rightPanel := NewThemedBox(ColorNameContent, rightPanelObj)

	if selectedIdx >= 0 && selectedIdx < len(um.Config.Profiles) {
		if d := displayPos(selectedIdx); d >= 0 {
			profileList.Select(widget.ListItemID(d))
		}
	}
	updateButtonStates()
	refreshDetails()

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.35

	headerLabel := widget.NewLabel("RunTTD")
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}
	var themeToggleBtn *widget.Button
	themeToggleBtn = widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), func() {
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(themeToggleBtn)
		pos.Y += themeToggleBtn.Size().Height
		um.showThemeCustomizer(pos)
	})

	themeToggleBtn.Importance = widget.LowImportance

	headerRight := container.NewHBox(themeToggleBtn)
	headerContent := container.NewBorder(nil, nil, nil, headerRight, headerLabel)
	header := NewThemedBox(ColorNameHeader, container.NewPadded(headerContent))

	um.startUpdateCheck(headerRight)

	mainContent := container.NewBorder(header, nil, nil, nil, split)
	um.Window.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		// Escape dismisses the top modal (profile editor, settings, theme
		// customizer), matching each modal's Cancel button which only hides it.
		if event.Name == fyne.KeyEscape {
			if top := um.Window.Canvas().Overlays().Top(); top != nil {
				top.Hide()
			}
			return
		}

		if um.Window.Content() != mainContent || um.Window.Canvas().Overlays().Top() != nil {
			return
		}

		if len(event.Name) == 1 && event.Name[0] >= '0' && event.Name[0] <= '9' {
			idx := int(event.Name[0] - '1')
			if event.Name[0] == '0' {
				idx = 9
			}
			if idx >= 0 && idx < len(um.Config.Profiles) {
				if d := displayPos(idx); d >= 0 {
					profileList.Select(widget.ListItemID(d))
				} else {
					profileList.UnselectAll()
				}
				launchIndex(idx)
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

// startUpdateCheck shows the update pill if a newer RunTTD release exists.
// The GitHub check runs at most once per app run; the result is cached on the
// UIManager and reused on later view constructions, so navigating back to the
// main view does not re-hit the API or make the pill flicker. On any error, no
// update, or a dev build, the header is left unchanged.
func (um *UIManager) startUpdateCheck(headerRight *fyne.Container) {
	if um.updateChecked {
		if um.updateTag != "" {
			um.addUpdatePill(headerRight, um.updateTag, um.updateURL)
		}
		return
	}
	go func() {
		tag, releaseURL, err := platform.LatestRunTTDRelease(context.Background())
		fyne.Do(func() {
			um.updateChecked = true
			if err != nil || !platform.IsNewerVersion(um.Version, tag) {
				return
			}
			um.updateTag = tag
			um.updateURL = releaseURL
			um.addUpdatePill(headerRight, tag, releaseURL)
		})
	}()
}

// addUpdatePill prepends an accent-colored "update available" pill to the header
// (left of the theme button) that opens the release page when clicked. Must be
// called on the main goroutine (inside fyne.Do or a UI callback).
func (um *UIManager) addUpdatePill(headerRight *fyne.Container, tag, releaseURL string) {
	pill := widget.NewButton("↻  Update to "+tag, func() {
		if u, perr := neturl.Parse(releaseURL); perr == nil {
			_ = fyne.CurrentApp().OpenURL(u)
		}
	})
	pill.Importance = widget.HighImportance
	// Prepend so the pill sits left of the theme toggle button.
	headerRight.Objects = append([]fyne.CanvasObject{pill}, headerRight.Objects...)
	headerRight.Refresh()
}

// showThemeCustomizer presents the preset accent color circular items and mode toggles
func (um *UIManager) showThemeCustomizer(pos fyne.Position) {
	apply := func(v string, presetIdx int) {
		um.Config.ThemeVariant = v
		um.Config.AccentPreset = presetIdx
		if pt, ok := um.App.Settings().Theme().(*LauncherTheme); ok {
			pt.UpdateAccent(presetIdx, v)
		}
		_ = domain.SaveConfig(um.ConfigPath, um.Config)
	}

	var currentMode string
	if um.Config.ThemeVariant == "light" {
		currentMode = "Light"
	} else {
		currentMode = "Dark"
	}

	modeSelect := NewSegmentedRadio([]string{"Light", "Dark"}, currentMode, func(s string) {
		apply(strings.ToLower(s), um.Config.AccentPreset)
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
			apply(um.Config.ThemeVariant, idx)
			updateButtons()
		})
		btn.Importance = widget.LowImportance

		colorGrid.Add(container.NewStack(rect, btn))
	}

	updateButtons()

	content := container.NewVBox(
		widget.NewLabel("Mode"),
		modeSelect.Container,
		widget.NewLabel("Accent Color"),
		colorGrid,
	)

	widget.NewPopUp(container.NewPadded(content), um.Window.Canvas()).ShowAtPosition(pos)
}

func indexOfProfileByName(profiles []domain.Profile, name string) int {
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

// uniqueProfileName returns "base Copy", or "base Copy (2)", "base Copy (3)" ...
// if earlier candidates collide (case-insensitively) with existing profiles.
func uniqueProfileName(profiles []domain.Profile, base string) string {
	candidate := base + " Copy"
	for n := 2; indexOfProfileByName(profiles, candidate) >= 0; n++ {
		candidate = fmt.Sprintf("%s Copy (%d)", base, n)
	}
	return candidate
}
