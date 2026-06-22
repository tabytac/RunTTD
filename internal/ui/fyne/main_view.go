package fyne

import (
	"context"
	"fmt"
	"image/color"
	neturl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
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

	// Launch status band: feedback for background launches (log auto-open off).
	// Hidden until a launch runs; kept a constant height (sized to the View logs
	// row) so it never resizes as it moves through phases.
	launchLogsIdx := -1
	launchInProgress := false
	launchPhase := widget.NewLabel("")
	launchPhase.Wrapping = fyne.TextWrapWord
	launchBar := widget.NewProgressBar()
	launchSpin := widget.NewProgressBarInfinite()
	launchLogsBtn := widget.NewButton("View logs", func() {
		if launchLogsIdx >= 0 {
			um.showLogView(launchLogsIdx)
		}
	})
	launchLogsBtn.Importance = widget.LowImportance

	launchBars := container.NewStack(launchSpin, launchBar)
	barsCentered := container.NewVBox(layout.NewSpacer(), launchBars, layout.NewSpacer())
	logsRow := container.NewHBox(layout.NewSpacer(), launchLogsBtn)
	rowPin := canvas.NewRectangle(color.Transparent)
	rowPin.SetMinSize(fyne.NewSize(1, launchLogsBtn.MinSize().Height))
	launchSecondRow := container.NewStack(rowPin, barsCentered, logsRow)
	launchBand := NewThemedBox(ColorNameDetailHeader, container.NewPadded(container.NewVBox(
		launchPhase,
		launchSecondRow,
	)))
	launchBand.Hide()

	var profileList *fyneadvancedlist.List
	var refreshDetails func()
	var updateEmptyState func()

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
		if launchInProgress {
			return
		}
		launchInProgress = true

		profile := um.Config.Profiles[idx]
		launchLogsIdx = idx

		// Reset the band to a fresh "working" state (marquee until download starts).
		launchLogsBtn.Hide()
		launchBar.Hide()
		launchSpin.Show()
		launchPhase.Importance = widget.MediumImportance
		launchPhase.TextStyle = fyne.TextStyle{}
		launchPhase.SetText("Starting " + profile.Name)
		launchBand.Show()
		launchBand.Refresh()
		runBtn.Disable()

		failed := false
		lastPct := -1
		go func() {
			um.launchProfile(profile,
				func(status string) {
					fyne.Do(func() { launchPhase.SetText(status) })
				},
				func(done, total int64) {
					if total <= 0 {
						return // unknown size: stay on the marquee
					}
					if done >= total {
						fyne.Do(func() {
							launchBar.Hide()
							launchSpin.Show()
							launchPhase.SetText("Extracting")
						})
						return
					}
					pct := int(done * 100 / total)
					if pct == lastPct {
						return // throttle to whole-percent steps
					}
					lastPct = pct
					fyne.Do(func() {
						launchSpin.Hide()
						launchBar.Show()
						launchBar.SetValue(float64(done) / float64(total))
					})
				},
				func() { failed = true },
			)

			fyne.Do(func() {
				launchInProgress = false
				launchSpin.Hide()
				launchBar.Hide()
				runBtn.Enable()
				updateButtonStates()
				if failed {
					launchPhase.Importance = widget.DangerImportance
					launchPhase.SetText(strings.TrimPrefix(launchPhase.Text, "Failed: "))
					launchPhase.Refresh()
					launchLogsBtn.Show()
					return
				}
				launchPhase.Importance = widget.MediumImportance
				launchPhase.TextStyle = fyne.TextStyle{Bold: true}
				launchPhase.SetText("Launched " + profile.Name)
				launchPhase.Refresh()
				go func() {
					time.Sleep(6000 * time.Millisecond)
					fyne.Do(launchBand.Hide)
				}()
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

	// Short pairs align in form; long values stack full-width in extra so wrapping
	// can't clip (FormLayout sizes rows from the unwrapped value).
	type section struct {
		form  *fyne.Container
		extra []fyne.CanvasObject
		count int
	}
	newSection := func() *section {
		return &section{form: container.New(layout.NewFormLayout())}
	}
	mutedLabel := func(text string) *widget.Label {
		l := widget.NewLabel(text)
		l.Importance = widget.LowImportance
		return l
	}
	addField := func(s *section, label, value string, mono bool) {
		if value == "" {
			return
		}
		val := widget.NewLabel(value)
		val.Wrapping = fyne.TextWrapOff
		val.Selectable = true
		if mono {
			val.TextStyle = fyne.TextStyle{Monospace: true}
		}
		s.form.Add(mutedLabel(label))
		s.form.Add(val)
		s.count++
	}
	addLongField := func(s *section, label, value string, mono bool) {
		if strings.TrimSpace(value) == "" {
			return
		}
		val := widget.NewLabel(value)
		val.Wrapping = fyne.TextWrapWord
		val.Selectable = true
		if mono {
			val.TextStyle = fyne.TextStyle{Monospace: true}
		}
		s.extra = append(s.extra, mutedLabel(label), val)
		s.count++
	}
	// addReveal adds a masked value with an eye button that toggles plaintext.
	addReveal := func(s *section, label, value string) {
		if value == "" {
			return
		}
		val := widget.NewLabel("••••••••")
		val.Selectable = true
		shown := false
		var btn *widget.Button
		btn = widget.NewButtonWithIcon("", theme.VisibilityIcon(), func() {
			shown = !shown
			if shown {
				val.SetText(value)
				btn.SetIcon(theme.VisibilityOffIcon())
			} else {
				val.SetText("••••••••")
				btn.SetIcon(theme.VisibilityIcon())
			}
		})
		btn.Importance = widget.LowImportance
		s.form.Add(mutedLabel(label))
		s.form.Add(container.NewBorder(nil, nil, nil, btn, val))
		s.count++
	}
	// addPathField renders a full-width path value (in s.extra, like addLongField)
	// with a reveal-in-file-browser button. isFile selects the file in its folder;
	// otherwise the folder is opened. Empty values render nothing.
	addPathField := func(s *section, label, value string, isFile bool) {
		if strings.TrimSpace(value) == "" {
			return
		}
		val := widget.NewLabel(value)
		val.Wrapping = fyne.TextWrapWord
		val.Selectable = true
		val.TextStyle = fyne.TextStyle{Monospace: true}
		btn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
			var err error
			if isFile {
				err = platform.RevealFileInFileManager(value)
			} else {
				err = platform.RevealInFileManager(value)
			}
			if err != nil {
				um.Logger.Append(fmt.Sprintf("Reveal failed for %s: %v", value, err))
				dialog.ShowError(fmt.Errorf("couldn't open the location: %w", err), um.Window)
			}
		})
		btn.Importance = widget.LowImportance
		row := container.NewBorder(nil, nil, nil, btn, val)
		s.extra = append(s.extra, mutedLabel(label), row)
		s.count++
	}
	emit := func(title string, s *section) {
		if s.count == 0 {
			return
		}
		body := container.NewVBox(NewSectionHeader(title), s.form)
		for _, o := range s.extra {
			body.Add(o)
		}
		detailsContainer.Add(NewThemedBox(ColorNameContent, container.NewPadded(body)))
	}

	refreshDetails = func() {
		updateButtonStates()
		detailsContainer.Objects = nil

		if selectedIdx < 0 || selectedIdx >= len(um.Config.Profiles) {
			detailsContainer.Add(mutedCenteredLabel("Select a profile to view its details and launch."))
			detailsContainer.Refresh()
			return
		}

		profile := um.Config.Profiles[selectedIdx]

		name := widget.NewRichText(&widget.TextSegment{
			Text: profile.Name,
			Style: widget.RichTextStyle{
				SizeName:  theme.SizeNameHeadingText,
				TextStyle: fyne.TextStyle{Bold: true},
				ColorName: theme.ColorNameForeground,
			},
		})
		verb, target := intentParts(profile)
		intentSegs := []widget.RichTextSegment{&widget.TextSegment{
			Text:  verb,
			Style: widget.RichTextStyle{Inline: true, ColorName: theme.ColorNamePlaceHolder},
		}}
		if target != "" {
			intentSegs = append(intentSegs, &widget.TextSegment{
				Text: "  " + target,
				Style: widget.RichTextStyle{
					Inline:    true,
					ColorName: theme.ColorNameForeground,
					TextStyle: fyne.TextStyle{Bold: true},
				},
			})
		}
		intent := widget.NewRichText(intentSegs...)
		intent.Wrapping = fyne.TextWrapWord
		header := container.NewVBox(name, intent)
		detailsContainer.Add(NewThemedBox(ColorNameDetailHeader, container.NewPadded(header)))

		launch := newSection()
		if profile.LaunchMode == "file" {
			addPathField(launch, "Save file", profile.SavePath, true)
		} else if profile.LaunchMode == "folder" {
			addPathField(launch, "Save Folder", profile.SavePath, false)
			label, value := filterDisplay(profile.AutoLatestFilter)
			addField(launch, label, value, false)
		}
		if profile.LaunchMode == "multiplayer" || profile.ServerIpPort != "" {
			addField(launch, "Server", profile.ServerIpPort, false)
			addField(launch, "Company Number", profile.ServerCompanyNumber, false)
			addReveal(launch, "Server Password", profile.ServerPassword)
			addReveal(launch, "Company Password", profile.ServerCompanyPassword)
		}
		emit("Launch", launch)

		client := newSection()
		if profile.Client == "custom" {
			addPathField(client, "Executable Folder", strings.TrimSpace(profile.CustomExecutablePath), false)
		} else {
			addField(client, "Version", valueOrDefault(profile.Version, "latest"), false)
		}
		emit("Client", client)

		adv := newSection()
		if profile.NoConfigSave {
			addField(adv, "No config save", "Enabled", false)
		}
		addField(adv, "NewGRF Loading", newGRFDesc(profile.NewGRFScanMode), false)
		addPathField(adv, "Config", profile.ConfigFilePath, true)
		addLongField(adv, "Arguments", profile.ExtraArgs, true)
		emit("Advanced", adv)

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

			badge := widget.NewLabel("")
			badge.TextStyle = fyne.TextStyle{Monospace: true}
			badge.Importance = widget.LowImportance

			nameLabel := widget.NewLabel("")
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			nameLabel.Truncation = fyne.TextTruncateEllipsis

			versionLabel := widget.NewLabel("")
			versionLabel.Importance = widget.LowImportance
			versionLabel.SizeName = theme.SizeNameCaptionText
			versionLabel.Truncation = fyne.TextTruncateEllipsis

			pad := theme.Padding()
			text := container.New(layout.NewCustomPaddedVBoxLayout(-pad/2), nameLabel, versionLabel)
			row := container.NewBorder(nil, nil, badge, nil, text)
			return container.NewStack(btn, container.New(layout.NewCustomPaddedLayout(0, 0, pad, pad), row))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			stack := o.(*fyne.Container)
			btn := stack.Objects[0].(*rightClickButton)
			padding := stack.Objects[1].(*fyne.Container)
			row := padding.Objects[0].(*fyne.Container)
			text := row.Objects[0].(*fyne.Container)
			badge := row.Objects[1].(*widget.Label)
			nameLabel := text.Objects[0].(*widget.Label)
			versionLabel := text.Objects[1].(*widget.Label)

			if int(i) < len(visibleIdx) {
				real := visibleIdx[i]
				profile := um.Config.Profiles[real]
				clientTag := shortClientLabel(profile.Client, um.Config.DefaultClient)
				var versionText string
				if profile.Client == "custom" {
					versionText = "Custom Client"
				} else {
					version := profile.Version
					if version == "" {
						version = "Latest"
					}
					versionText = clientTag + " · " + version
				}
				if real < 10 {
					badge.SetText(strconv.Itoa((real + 1) % 10))
				} else {
					badge.SetText(" ")
				}
				nameLabel.SetText(profile.Name)
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

	newBtn := widget.NewButtonWithIcon("New", theme.ContentAddIcon(), func() {
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
			updateEmptyState()
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
						updateEmptyState()
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

	seeLogsBtn = widget.NewButtonWithIcon("Logs", theme.DocumentIcon(), func() {
		um.showLogView(-1)
	})
	seeLogsBtn.Importance = widget.LowImportance

	manageInstallsBtn := widget.NewButtonWithIcon("Installs", theme.StorageIcon(), func() {
		um.showLibraryView()
	})
	manageInstallsBtn.Importance = widget.LowImportance

	settingsBtn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
		um.showSettingsView()
	})
	settingsBtn.Importance = widget.LowImportance

	actionsContent := container.NewVBox(
		runBtn,
		container.NewGridWithColumns(3, editBtn, duplicateBtn, deleteBtn),
	)

	searchEntry := newSearchEntry()
	searchEntry.SetPlaceHolder("Search profiles...")

	// Header band: title, live total count, and the primary New action.
	title := widget.NewRichText(&widget.TextSegment{
		Text: "Profiles",
		Style: widget.RichTextStyle{
			SizeName:  theme.SizeNameSubHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
			ColorName: theme.ColorNameForeground,
		},
	})
	headerRow := container.NewBorder(nil, nil, title, newBtn)
	headerBand := NewThemedBox(ColorNameDetailHeader, container.NewPadded(headerRow))

	// No-results state (search matched nothing).
	noResults := container.NewCenter(container.NewVBox(
		centeredLabel("No profiles match your search."),
		mutedCenteredLabel("Press Esc to clear."),
	))
	noResults.Hide()

	// First-run state (no profiles exist yet).
	firstRunBtn := widget.NewButtonWithIcon("New Profile", theme.ContentAddIcon(), func() {
		um.showProfileEditor(-1, true)
	})
	firstRunBtn.Importance = widget.HighImportance
	firstRun := container.NewCenter(container.NewVBox(
		centeredLabel("No profiles yet."),
		mutedCenteredLabel("Create your first profile to get started."),
		container.NewCenter(firstRunBtn),
	))
	firstRun.Hide()

	updateEmptyState = func() {
		switch {
		case len(um.Config.Profiles) == 0:
			firstRun.Show()
			noResults.Hide()
			searchEntry.Hide()
		case len(visibleIdx) == 0 && strings.TrimSpace(filterText) != "":
			noResults.Show()
			firstRun.Hide()
			searchEntry.Show()
		default:
			firstRun.Hide()
			noResults.Hide()
			searchEntry.Show()
		}
	}

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
	updateEmptyState()

	dragHint := widget.NewLabel("Drag rows to reorder")
	dragHint.Importance = widget.LowImportance
	dragHint.SizeName = theme.SizeNameCaptionText
	dragHint.Alignment = fyne.TextAlignCenter

	footer := container.NewPadded(container.NewVBox(
		dragHint,
		container.NewGridWithColumns(3, seeLogsBtn, manageInstallsBtn, settingsBtn),
	))

	listArea := container.NewStack(profileList, noResults, firstRun)
	top := container.NewVBox(headerBand, container.NewPadded(searchEntry))
	leftPanelObj := container.NewBorder(top, footer, nil, nil, listArea)
	leftPanel := NewThemedBox(ColorNameSidebar, leftPanelObj)

	detailsContent := container.NewVScroll(container.NewPadded(detailsContainer))

	rightPanelObj := container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), launchBand, actionsContent, container.NewPadded(selectionHint)),
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

	// A "Save & Run" from the editor defers its launch to here, so it goes through
	// the normal path (AutoOpenLog + launch band) once this view is live.
	if um.pendingLaunchIdx >= 0 {
		idx := um.pendingLaunchIdx
		um.pendingLaunchIdx = -1
		fyne.Do(func() { launchIndex(idx) })
	}

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

func centeredLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Alignment = fyne.TextAlignCenter
	l.Wrapping = fyne.TextWrapWord
	return l
}

func mutedCenteredLabel(text string) *widget.Label {
	l := centeredLabel(text)
	l.Importance = widget.LowImportance
	return l
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

// intentParts splits the launch intent into a muted verb and an accent-colored target.
func intentParts(p domain.Profile) (verb, target string) {
	switch p.LaunchMode {
	case "file":
		return "Load the selected file", filepath.Base(p.SavePath)
	case "folder":
		return "Load the most recent " + folderItemNoun(p.AutoLatestFilter) + " in", filepath.Base(p.SavePath)
	case "multiplayer":
		return "Launch and join the server at", valueOrDefault(p.ServerIpPort, "Server")
	default:
		return "Launch straight into the Main Menu", ""
	}
}

// folderItemNoun names what the folder filter picks, for the launch intent line.
func folderItemNoun(filter string) string {
	switch filter {
	case "sav":
		return "save"
	case "scn":
		return "scenario"
	default:
		return "save or scenario"
	}
}

// filterDisplay maps a stored auto-latest filter to a grammar-matched label and
// full-name value: "File types"/"Saves & Scenarios" for both, "File type"/singular otherwise.
func filterDisplay(filter string) (label, value string) {
	switch filter {
	case "sav":
		return "File type", "Saves only"
	case "scn":
		return "File type", "Scenarios only"
	default:
		return "File types", "Saves & Scenarios"
	}
}

func newGRFDesc(mode string) string {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "Q":
		return "Skip NewGRF loading at startup"
	case "QQ":
		return "Disable all NewGRF scanning/loading (session-wide)"
	default:
		return ""
	}
}
