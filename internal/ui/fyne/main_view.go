package fyne

import (
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
	fyneadvancedlist "github.com/dweymouth/fyne-advanced-list"

	"runttd/internal/domain"
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

// makeOnboardingView creates the first-run configuration screen
func (um *UIManager) makeOnboardingView() fyne.CanvasObject {
	welcomeLabel := widget.NewLabel("Welcome to RunTTD!")
	welcomeLabel.TextStyle = fyne.TextStyle{Bold: true, Italic: false}
	welcomeLabel.Alignment = fyne.TextAlignCenter

	instructions := NewSectionDescription("Before we begin, please confirm your installation folders.\nThese default paths are based on your operating system, but you can change them if you have a custom setup.")

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

	subfolderCheck, subfolderGroup := NewLabeledCheckWithDescription(
		"Organize downloaded clients into per-client subfolders",
		"Keeps each client's downloaded files in a separate folder, instead of all sharing the parent folder. "+
			"Easiest to choose now, before anything is downloaded; you can change it later in Settings.",
		um.Config.SubfolderPerClient,
	)

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

	continueBtn := widget.NewButton("Continue", func() {
		if !validate() {
			return
		}

		um.Config.ParentDir = strings.TrimSpace(parentDirEntry.Text)
		um.Config.DocsBasePath = strings.TrimSpace(docsBasePathEntry.Text)
		um.Config.SubfolderPerClient = subfolderCheck.Checked
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
		instructions,
		NewSectionHeader("Installation Paths"),
		widget.NewLabel("Parent Directory (where game files / executables will be automatically installed)"),
		container.NewBorder(nil, nil, nil, parentDirBtn, parentDirEntry),
		widget.NewLabel("Docs Base Path (Saves & config)"),
		container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, docsBasePathBtn), docsBasePathEntry),
		NewSectionHeader("Optional"),
		subfolderGroup,
		statusLabel,
	)

	onboardingScroll := container.NewVScroll(form)
	onboardingScroll.SetMinSize(fyne.NewSize(0, 300))
	return container.NewBorder(
		nil,
		container.NewHBox(widget.NewLabel(""), continueBtn),
		nil,
		nil,
		container.NewPadded(onboardingScroll),
	)
}

// makeMainView creates the main profile selection view
func (um *UIManager) makeMainView() fyne.CanvasObject {
	selectedIdx := indexOfProfileByName(um.Config.Profiles, um.SelectedProfileName)
	detailsContainer := container.NewVBox()

	selectionHint := widget.NewLabel("Tip: Press 1-9, or 0 to quick launch. Select a profile and press Enter / double-click.")
	selectionHint.Wrapping = fyne.TextWrapWord

	var profileList *fyneadvancedlist.List
	var refreshDetails func()

	var runBtn, editBtn, duplicateBtn, deleteBtn, seeLogsBtn *widget.Button
	var updateButtonStates func()

	runSelected := func() {
		if selectedIdx >= 0 && selectedIdx < len(um.Config.Profiles) {
			if um.Config.AutoOpenLog {
				um.showLogView(selectedIdx)
			} else {
				// Background launch with feedback
				oldText := runBtn.Text
				runBtn.SetText("Launching...")
				runBtn.Disable()

				profile := um.Config.Profiles[selectedIdx]
				um.showToast(fmt.Sprintf("Starting %s...", profile.Name))

				go func() {
					um.launchProfile(profile, nil, func() {
						um.showLogView(selectedIdx)
					})

					time.Sleep(1500 * time.Millisecond)
					runBtn.SetText(oldText)
					updateButtonStates()
				}()
			}
			return
		}
		dialog.ShowError(fmt.Errorf("select a profile to launch"), um.Window)
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
			if len(um.Logger.GetAll()) > 0 {
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

	profileList = fyneadvancedlist.NewList(
		func() int { return len(um.Config.Profiles) },
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

			if i < len(um.Config.Profiles) {
				profile := um.Config.Profiles[i]
				var versionText string
				if profile.Client == "custom" {
					versionText = "custom"
				} else {
					versionText = profile.Version
					if versionText == "" {
						versionText = "latest"
					}
				}
				nameLabel.SetText(fmt.Sprintf("%d. %s", i+1, profile.Name))
				versionLabel.SetText(versionText)
				idx := int(i)
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
		selectProfile(int(id))
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
			dup.Name = dup.Name + " Copy"
			um.Config.Profiles = append(um.Config.Profiles, dup)
			_ = domain.SaveConfig(um.ConfigPath, um.Config)

			selectedIdx = len(um.Config.Profiles) - 1
			um.SelectedProfileName = um.Config.Profiles[selectedIdx].Name
			profileList.Refresh()
			profileList.Select(widget.ListItemID(selectedIdx))
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
						profileList.Refresh()
						profileList.Select(widget.ListItemID(selectedIdx))
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
		profileList.Select(widget.ListItemID(selectedIdx))
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

	headerContent := container.NewBorder(nil, nil, nil, themeToggleBtn, headerLabel)
	header := NewThemedBox(ColorNameHeader, container.NewPadded(headerContent))

	mainContent := container.NewBorder(header, nil, nil, nil, split)
	um.Window.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		if um.Window.Content() != mainContent || um.Window.Canvas().Overlays().Top() != nil {
			return
		}

		if len(event.Name) == 1 && event.Name[0] >= '0' && event.Name[0] <= '9' {
			idx := int(event.Name[0] - '1')
			if event.Name[0] == '0' {
				idx = 9
			}
			if idx < len(um.Config.Profiles) {
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
