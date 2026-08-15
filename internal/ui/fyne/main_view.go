package fyne

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fyneadvancedlist "github.com/dweymouth/fyne-advanced-list"

	apppkg "runttd/internal/app"
	"runttd/internal/domain"
)

// mainView owns one profile-view instance's state and widgets; a fresh one per makeMainView call keeps launch/selection/filter state isolated.
type mainView struct {
	um *UIManager

	// selectedIdx is the real Config.Profiles index of the selection, or -1.
	selectedIdx int
	// visibleIdx maps a displayed row to the real Config.Profiles index (identity
	// when unfiltered); quick-launch always uses the real index, so filtering is
	// visual. filterText is the current search needle.
	visibleIdx []int
	filterText string

	// Launch-band state.
	launchLogsIdx    int
	launchInProgress bool
	launchGen        int    // bumped per launch; guards the stale success auto-hide
	launchCancel     func() // cancels the in-flight launch's download context, if any

	// Undo-banner state. undoProfile is nil unless a restore is pending; only the
	// newest deletion is recoverable, so a second one supersedes any older offer.
	undoProfile *domain.Profile
	undoIdx     int
	undoGen     int // bumped per deletion; guards the stale banner auto-hide

	detailsContainer *fyne.Container
	profileList      *fyneadvancedlist.List
	launchPhase      *widget.Label
	launchBar        *widget.ProgressBar
	launchSpin       *widget.ProgressBarInfinite
	launchLogsBtn    *viewButton
	cancelBtn        *viewButton
	launchBand       *ThemedBox
	runBtn           *viewButton
	editBtn          *viewButton
	duplicateBtn     *viewButton
	deleteBtn        *viewButton
	seeLogsBtn       *viewButton
	searchEntry      *searchEntry
	noResults        *fyne.Container
	firstRun         *fyne.Container
	undoBand         *ThemedBox
	undoLabel        *widget.Label
}

// displayPos returns the filtered row position of a real index, or -1 if hidden.
func (mv *mainView) displayPos(real int) int {
	for d, r := range mv.visibleIdx {
		if r == real {
			return d
		}
	}
	return -1
}

// selectionSurvivesFilter reports whether the current selection is still visible
// under the active filter, so typing a search needle doesn't clear a selection
// that still matches.
func (mv *mainView) selectionSurvivesFilter() bool {
	return mv.selectedIdx >= 0 && mv.displayPos(mv.selectedIdx) >= 0
}

// digitLaunchIndex maps a quick-launch key ('1'-'9', '0' for the 10th) to a
// profile index, or -1 if there's no such profile or it's hidden by the
// active search filter (its badge digit isn't shown on any visible row, so
// launching it would silently start a profile the user can't currently see).
func (mv *mainView) digitLaunchIndex(digit byte) int {
	idx := int(digit - '1')
	if digit == '0' {
		idx = 9
	}
	if idx < 0 || idx >= len(mv.um.Config.Profiles) {
		return -1
	}
	if mv.displayPos(idx) < 0 {
		return -1
	}
	return idx
}

func (mv *mainView) recomputeVisible() {
	needle := strings.ToLower(strings.TrimSpace(mv.filterText))
	mv.visibleIdx = mv.visibleIdx[:0]
	for i, p := range mv.um.Config.Profiles {
		if needle == "" || strings.Contains(strings.ToLower(p.Name), needle) {
			mv.visibleIdx = append(mv.visibleIdx, i)
		}
	}
}

func (mv *mainView) selectProfile(idx int) {
	um := mv.um
	if idx < 0 || idx >= len(um.Config.Profiles) {
		mv.selectedIdx = -1
		um.SelectedProfileName = ""
		return
	}

	mv.selectedIdx = idx
	um.SelectedProfileName = um.Config.Profiles[mv.selectedIdx].Name
	mv.refreshDetails()
}

func (mv *mainView) handleRowTap(idx int) {
	um := mv.um
	now := time.Now()
	if idx == mv.selectedIdx && idx == um.LastListSelectID && now.Sub(um.LastListSelectAt) < 450*time.Millisecond {
		um.LastListSelectAt = time.Time{}
		mv.runSelected()
		return
	}

	um.LastListSelectID = idx
	um.LastListSelectAt = now
	if d := mv.displayPos(idx); d >= 0 {
		mv.profileList.Select(widget.ListItemID(d))
	} else {
		mv.profileList.UnselectAll()
	}
}

func (mv *mainView) runSelected() {
	mv.um.suppressAutoCloseOnce = false
	mv.launchIndex(mv.selectedIdx)
}

// clearSearch drops the filter and returns to the full list, then blurs so
// quick-launch (digits, Enter) works again immediately rather than staying dead
// until the user clicks elsewhere: Fyne routes keys to the focused widget OR the
// canvas, never both, so the search Entry keeping focus would swallow every
// later key. It answers Escape both from the Entry itself and, via viewEscape,
// from anywhere else on the profile view, which is what the empty state promises.
func (mv *mainView) clearSearch() {
	if mv.searchEntry.Text != "" {
		mv.searchEntry.SetText("") // triggers OnChanged, which resets the filter
	}
	mv.um.Window.Canvas().Unfocus()
}

func (mv *mainView) updateButtonStates() {
	if mv.selectedIdx >= 0 {
		if mv.launchInProgress {
			mv.runBtn.Disable() // stay disabled mid-launch no matter what else triggers this refresh
		} else {
			mv.runBtn.Enable()
		}
		mv.editBtn.Enable()
		mv.duplicateBtn.Enable()
		if len(mv.um.Config.Profiles) <= 1 {
			mv.deleteBtn.Disable() // the last profile can't be deleted; don't even let the click happen
		} else {
			mv.deleteBtn.Enable()
		}
	} else {
		mv.runBtn.Disable()
		mv.editBtn.Disable()
		mv.duplicateBtn.Disable()
		mv.deleteBtn.Disable()
	}

	// Always enabled: an empty log view is still a valid view (e.g. right after
	// Clear Logs), and disabling it there would lock the user out of ever
	// reopening it, since nothing else re-enables it once Logger.Len() hits 0.
}

func (mv *mainView) refreshDetails() {
	um := mv.um
	mv.updateButtonStates()
	mv.detailsContainer.Objects = nil

	if mv.selectedIdx < 0 || mv.selectedIdx >= len(um.Config.Profiles) {
		mv.detailsContainer.Add(mutedCenteredLabel("Select a profile to view its details and launch."))
		mv.detailsContainer.Refresh()
		return
	}

	profile := um.Config.Profiles[mv.selectedIdx]

	name := widget.NewRichText(&widget.TextSegment{
		Text: profile.Name,
		Style: widget.RichTextStyle{
			SizeName:  theme.SizeNameHeadingText,
			TextStyle: fyne.TextStyle{Bold: true},
			ColorName: theme.ColorNameForeground,
		},
	})
	// Unwrapped, this reports its full width as a minimum, and a vertical Scroll
	// passes that straight out: a long name would push the whole window past the
	// screen rather than being given two lines.
	name.Wrapping = fyne.TextWrapWord
	// Text-only: the + icon already means "new profile" in this view, and the
	// auto-launch button directly below is text-only too.
	shortcutBtn := um.newViewButton("Create shortcut", func() {
		um.showShortcutDialog(profile)
	})
	shortcutBtn.Importance = widget.LowImportance

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
	// Border, not VBox: the button keeps its own width at the top right while the
	// wrapping name and intent take the rest of the row.
	header := container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewVBox(shortcutBtn), name),
		intent,
	)
	mv.detailsContainer.Add(NewThemedBox(ColorNameDetailHeader, container.NewPadded(header)))

	// In-context auto-launch toggle; persists immediately, unlike the save-gated dialog.
	isStartup := profile.Name == um.Config.AutoLaunchProfile && um.Config.AutoLaunchProfile != ""
	autoBtn := um.newViewButton("", nil)
	if isStartup {
		autoBtn.SetText("★ Launches on startup · click to turn off")
		autoBtn.Importance = widget.HighImportance
	} else {
		autoBtn.SetText("☆ Launch on startup")
		autoBtn.Importance = widget.MediumImportance
	}
	autoBtn.OnTapped = func() {
		if isStartup {
			um.setAutoLaunchProfile("")
		} else {
			um.setAutoLaunchProfile(profile.Name)
		}
		mv.refreshDetails() // flip this button; the list marker is refreshed by the helper
	}
	mv.detailsContainer.Add(autoBtn)

	// Echo of the row's warning, from the async cache (setupIssueFor); no disk I/O on this path.
	if issue := um.setupIssueFor(profile); issue != "" {
		wl := widget.NewLabel(issue)
		wl.Importance = widget.WarningImportance
		mv.detailsContainer.Add(container.NewHBox(newSetupWarning(), wl))
	}

	launch := newSection()
	if profile.LaunchMode == "file" {
		um.addPathField(launch, "Save File", profile.SavePath, true)
	} else if profile.LaunchMode == "folder" {
		um.addPathField(launch, "Save Folder", profile.SavePath, false)
		label, value := filterDisplay(profile.AutoLatestFilter)
		if profile.SaveSearchSubfolders {
			value += " (including subfolders)"
		}
		launch.addField(label, value, false)
	}
	if profile.LaunchMode == "multiplayer" || profile.ServerIpPort != "" {
		launch.addField("Server", profile.ServerIpPort, false)
		launch.addField("Company Number", profile.ServerCompanyNumber, false)
		launch.addReveal("Server Password", profile.ServerPassword)
		effClient := apppkg.EffectiveClient(profile.Client, um.Config.DefaultClient)
		if apppkg.ClientSupportsCompanyPassword(effClient) {
			launch.addReveal("Company Password", profile.ServerCompanyPassword)
		}
	}
	launch.emit("Launch", mv.detailsContainer)

	client := newSection()
	if profile.Client == "custom" {
		um.addPathField(client, "Executable Folder", strings.TrimSpace(profile.CustomExecutablePath), false)
	} else {
		client.addField("Version", valueOrDefault(profile.Version, "latest"), false)
	}
	client.emit("Client", mv.detailsContainer)

	adv := newSection()
	if profile.NoConfigSave {
		adv.addField("No config save", "Enabled", false)
	}
	adv.addField("NewGRF Loading", newGRFDesc(profile.NewGRFScanMode), false)
	um.addPathField(adv, "Config", profile.ConfigFilePath, true)
	if profile.ExtraArgsDisabled {
		adv.addMutedLongField("Arguments (disabled)", profile.ExtraArgs, true)
	} else {
		adv.addLongField("Arguments", profile.ExtraArgs, true)
	}
	adv.emit("Advanced", mv.detailsContainer)

	mv.detailsContainer.Refresh()
}

// newEmptyState stacks placeholder widgets down the middle of the list panel.
// Spacers rather than NewCenter: a word-wrapping label's MinSize is only its own
// padding, so a MinSize-based layout wraps the message one character wide.
func newEmptyState(objects ...fyne.CanvasObject) *fyne.Container {
	return container.NewVBox(
		layout.NewSpacer(),
		container.NewPadded(container.NewVBox(objects...)),
		layout.NewSpacer(),
	)
}

func (mv *mainView) updateEmptyState() {
	switch {
	case len(mv.um.Config.Profiles) == 0:
		mv.firstRun.Show()
		mv.noResults.Hide()
		mv.searchEntry.Hide()
	case len(mv.visibleIdx) == 0 && strings.TrimSpace(mv.filterText) != "":
		mv.noResults.Show()
		mv.firstRun.Hide()
		mv.searchEntry.Show()
	default:
		mv.firstRun.Hide()
		mv.noResults.Hide()
		mv.searchEntry.Show()
	}
}

// duplicateSelected copies the selected profile, selects the copy, and refreshes.
func (mv *mainView) duplicateSelected() {
	um := mv.um
	if mv.selectedIdx < 0 {
		um.showError("select a profile to duplicate")
		return
	}
	dup := um.Config.Profiles[mv.selectedIdx]
	dup.Name = uniqueProfileName(um.Config.Profiles, dup.Name)
	um.Config.Profiles = append(um.Config.Profiles, dup)
	um.saveConfigOrWarn()

	mv.selectedIdx = len(um.Config.Profiles) - 1
	um.SelectedProfileName = um.Config.Profiles[mv.selectedIdx].Name
	mv.recomputeVisible()
	mv.profileList.Refresh()
	if d := mv.displayPos(mv.selectedIdx); d >= 0 {
		mv.profileList.Select(widget.ListItemID(d))
	} else {
		mv.profileList.UnselectAll()
	}
	mv.refreshDetails()
	mv.updateEmptyState()
}

// A drawn SVG, not a "▶" label: Windows renders that glyph via the emoji font.
var startupMarkerSVG = fyne.NewStaticResource("startup-marker.svg",
	[]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path fill="#1E88E5" d="M1 0 L9 5 L1 10 Z"/></svg>`))

// newStartupMarker returns the blue ▶ flag shown on the auto-launch profile's row.
func newStartupMarker() *canvas.Image {
	img := canvas.NewImageFromResource(startupMarkerSVG)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(dotDiameter, dotDiameter))
	return img
}

// Amber triangle with an exclamation cutout; drawn, not a "⚠" glyph (emoji font fallback).
var setupWarningSVG = fyne.NewStaticResource("setup-warning.svg",
	[]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path fill="#E6A700" d="M5 0 L10 10 L0 10 Z"/><rect x="4.35" y="3.4" width="1.3" height="3.3" fill="#20242C"/><rect x="4.35" y="7.8" width="1.3" height="1.3" fill="#20242C"/></svg>`))

// newSetupWarning returns the ⚠ flag shown on rows whose launch would silently
// miss the profile's configured intent (see app.ProfileSetupIssue).
func newSetupWarning() *canvas.Image {
	img := canvas.NewImageFromResource(setupWarningSVG)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(dotDiameter, dotDiameter))
	return img
}

// buildProfileList creates the profile list widget and wires its row template,
// row binding, drag-reorder, and selection callbacks.
func (mv *mainView) buildProfileList() {
	um := mv.um
	mv.profileList = fyneadvancedlist.NewList(
		func() int { return len(mv.visibleIdx) },
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
			warn := container.New(layout.NewCustomPaddedLayout(0, 0, 0, pad/2), newSetupWarning())
			warn.Hide() // shown only on rows with a setup issue
			marker := container.New(layout.NewCustomPaddedLayout(0, 0, 0, pad/2), newStartupMarker())
			marker.Hide() // shown only on the startup row
			dot := newStatusDot()
			dotWrap := container.New(layout.NewCustomPaddedLayout(0, 0, 0, pad), dot)
			right := container.NewHBox(warn, marker, dotWrap)
			row := container.NewBorder(nil, nil, badge, right, text)
			return container.NewStack(btn, container.New(layout.NewCustomPaddedLayout(0, 0, pad, pad), row))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			stack := o.(*fyne.Container)
			btn := stack.Objects[0].(*rightClickButton)
			padding := stack.Objects[1].(*fyne.Container)
			row := padding.Objects[0].(*fyne.Container)
			text := row.Objects[0].(*fyne.Container)
			badge := row.Objects[1].(*widget.Label)
			right := row.Objects[2].(*fyne.Container) // the HBox
			warn := right.Objects[0].(*fyne.Container)
			marker := right.Objects[1].(*fyne.Container)
			dot := right.Objects[2].(*fyne.Container).Objects[0].(*statusDot) // dotWrap -> dot
			nameLabel := text.Objects[0].(*widget.Label)
			versionLabel := text.Objects[1].(*widget.Label)

			if int(i) < len(mv.visibleIdx) {
				real := mv.visibleIdx[i]
				profile := um.Config.Profiles[real]
				clientTag := shortClientLabel(profile.Client, um.Config.DefaultClient)
				var versionText string
				if profile.Client == "custom" {
					versionText = "Custom Client"
				} else {
					version := profile.Version
					if version == "" {
						version = "latest" // matches the details pane's Version field default
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
				dot.SetState(um.resolveDotState(profile))
				if um.setupIssueFor(profile) != "" {
					warn.Show()
				} else {
					warn.Hide()
				}
				if profile.Name == um.Config.AutoLaunchProfile && um.Config.AutoLaunchProfile != "" {
					marker.Show()
				} else {
					marker.Hide()
				}
				idx := real
				btn.OnTapped = func() {
					mv.handleRowTap(idx)
				}
				btn.onSecondaryTapped = func() {
					um.showProfileEditor(idx, false)
				}
				btn.onLaunch = func() {
					if d := mv.displayPos(idx); d >= 0 {
						mv.profileList.Select(widget.ListItemID(d))
					}
					mv.um.suppressAutoCloseOnce = false // a manual launch must clear a stale startup-suppression flag
					mv.launchIndex(idx)
				}
			}
		},
	)

	mv.profileList.EnableDragging = true
	mv.profileList.OnDragEnd = func(draggedFrom, draggedTo widget.ListItemID) {
		from := int(draggedFrom)
		to := int(draggedTo)

		if from == to {
			return
		}

		um.Config.Profiles = reorderProfiles(um.Config.Profiles, from, to)
		um.saveConfigOrWarn()

		// Re-resolve by name: the splice shifts mv.selectedIdx onto a different
		// profile whenever the drag crosses it, even if the dragged row wasn't the selection.
		mv.selectedIdx = indexOfProfileByName(um.Config.Profiles, um.SelectedProfileName)
		mv.profileList.Refresh()
		if d := mv.displayPos(mv.selectedIdx); d >= 0 {
			mv.profileList.Select(widget.ListItemID(d))
		} else {
			mv.profileList.UnselectAll()
		}
		mv.refreshDetails()
	}
	mv.profileList.OnSelected = func(id widget.ListItemID) {
		if int(id) < len(mv.visibleIdx) {
			mv.selectProfile(mv.visibleIdx[id])
		}
	}
	mv.profileList.OnUnselected = func(_ widget.ListItemID) {
		mv.selectProfile(-1)
		mv.refreshDetails()
	}
	um.profileListRefresh = func() { mv.profileList.Refresh() }
	um.detailsRefresh = func() { mv.refreshDetails() }
}

// makeMainView creates the main profile selection view
func (um *UIManager) makeMainView() fyne.CanvasObject {
	mv := &mainView{
		um:               um,
		selectedIdx:      indexOfProfileByName(um.Config.Profiles, um.SelectedProfileName),
		launchLogsIdx:    -1,
		detailsContainer: container.NewVBox(),
	}

	mv.visibleIdx = make([]int, len(um.Config.Profiles))
	for i := range mv.visibleIdx {
		mv.visibleIdx[i] = i
	}

	selectionHint := widget.NewLabel("Press 1–9 (0 for 10th) to quick-launch · Enter or double-click to launch selected · Right-click to edit")
	selectionHint.Importance = widget.LowImportance
	selectionHint.Alignment = fyne.TextAlignCenter
	selectionHint.Wrapping = fyne.TextWrapWord

	// Launch status band: feedback for background launches (log auto-open off).
	// Hidden until a launch runs; kept a constant height (sized to the View Logs
	// row) so it never resizes as it moves through phases.
	mv.launchPhase = widget.NewLabel("")
	mv.launchPhase.Wrapping = fyne.TextWrapWord
	mv.launchBar = widget.NewProgressBar()
	mv.launchSpin = widget.NewProgressBarInfinite()
	mv.launchLogsBtn = um.newViewButton("View logs", func() {
		if mv.launchLogsIdx >= 0 {
			um.showLogView(mv.launchLogsIdx)
		}
	})
	mv.launchLogsBtn.Importance = widget.LowImportance
	mv.cancelBtn = um.newViewButton("Cancel", func() {
		if mv.launchCancel != nil {
			mv.launchCancel()
		}
	})
	mv.cancelBtn.Importance = widget.DangerImportance
	mv.cancelBtn.Hide()

	launchBars := container.NewStack(mv.launchSpin, mv.launchBar)
	// Cancel shares the bar's row via Border (bar takes the remaining space, Cancel
	// pins right) rather than another full-width Stack layer, which would draw the
	// button on top of the bar instead of beside it: Border skips a hidden Right
	// widget's space entirely, so the bar still spans the full row once Cancel hides.
	barsWithCancel := container.NewBorder(nil, nil, nil, mv.cancelBtn, launchBars)
	barsCentered := container.NewVBox(layout.NewSpacer(), barsWithCancel, layout.NewSpacer())
	logsRow := container.NewHBox(layout.NewSpacer(), mv.launchLogsBtn)
	rowPin := canvas.NewRectangle(color.Transparent)
	rowPin.SetMinSize(fyne.NewSize(1, mv.launchLogsBtn.MinSize().Height))
	launchSecondRow := container.NewStack(rowPin, barsCentered, logsRow)
	mv.launchBand = NewThemedBox(ColorNameDetailHeader, container.NewPadded(container.NewVBox(
		mv.launchPhase,
		launchSecondRow,
	)))
	mv.launchBand.Hide()

	mv.recomputeVisible()
	mv.buildProfileList()

	newBtn := um.newViewIconButton("New", theme.ContentAddIcon(), func() {
		um.showProfileEditor(-1, true)
	})
	newBtn.Importance = widget.HighImportance

	mv.editBtn = um.newViewButton("Edit", func() {
		if mv.selectedIdx >= 0 {
			um.showProfileEditor(mv.selectedIdx, false)
		} else {
			um.showError("select a profile to edit")
		}
	})
	mv.duplicateBtn = um.newViewButton("Duplicate", mv.duplicateSelected)
	mv.deleteBtn = um.newViewButton("Delete", mv.deleteSelected)

	mv.runBtn = um.newViewButton("Run selected", mv.runSelected)
	mv.runBtn.Importance = widget.HighImportance

	mv.seeLogsBtn = um.newViewIconButton("Logs", theme.DocumentIcon(), func() {
		um.showLogView(-1)
	})
	mv.seeLogsBtn.Importance = widget.LowImportance

	manageInstallsBtn := um.newViewIconButton("Installs", theme.StorageIcon(), func() {
		um.showLibraryView()
	})
	manageInstallsBtn.Importance = widget.LowImportance

	settingsBtn := um.newViewIconButton("Settings", theme.SettingsIcon(), func() {
		um.showSettingsView()
	})
	settingsBtn.Importance = widget.LowImportance

	actionsContent := container.NewVBox(
		mv.runBtn,
		container.NewGridWithColumns(3, mv.editBtn, mv.duplicateBtn, mv.deleteBtn),
	)

	mv.searchEntry = newSearchEntry()
	mv.searchEntry.SetPlaceHolder("Search profiles…")

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

	mv.noResults = newEmptyState(
		centeredLabel("No profiles match your search."),
		mutedCenteredLabel("Press Esc to clear."),
	)
	mv.noResults.Hide()

	// First-run state (no profiles exist yet).
	firstRunBtn := um.newViewIconButton("New profile", theme.ContentAddIcon(), func() {
		um.showProfileEditor(-1, true)
	})
	firstRunBtn.Importance = widget.HighImportance
	mv.firstRun = newEmptyState(
		centeredLabel("No profiles yet."),
		mutedCenteredLabel("Create your first profile to get started."),
		container.NewCenter(firstRunBtn),
	)
	mv.firstRun.Hide()

	// Undo affordance: an in-panel band, not a toast, since an overlay would
	// swallow the click that has to reach Undo.
	mv.undoLabel = widget.NewLabel("")
	mv.undoLabel.Wrapping = fyne.TextWrapWord
	undoBtn := um.newViewButton("Undo", mv.undoDelete)
	undoBtn.Importance = widget.HighImportance
	mv.undoBand = NewThemedBox(ColorNameDetailHeader, container.NewPadded(
		container.NewBorder(nil, nil, nil, undoBtn, mv.undoLabel),
	))
	mv.undoBand.Hide()

	mv.searchEntry.OnChanged = func(s string) {
		mv.filterText = s
		mv.recomputeVisible()
		// Reordering a filtered subset is ambiguous; only allow drag with no filter.
		mv.profileList.EnableDragging = strings.TrimSpace(s) == ""
		mv.profileList.Refresh()
		if mv.selectionSurvivesFilter() {
			// Still visible under the new filter: keep it selected instead of clearing on every keystroke.
			mv.profileList.Select(widget.ListItemID(mv.displayPos(mv.selectedIdx)))
		} else {
			mv.profileList.UnselectAll()
			mv.selectProfile(-1)
			mv.refreshDetails()
		}
		mv.updateEmptyState()
	}
	// Esc clears the filter and returns to the full list, then blurs so quick-launch
	// (digits, Enter) works again immediately rather than staying dead until the user
	// clicks elsewhere: Fyne routes keys to the focused widget OR the canvas, never
	// both, so this Entry keeping focus would otherwise swallow every later key.
	mv.searchEntry.onEscape = mv.clearSearch
	// The hint under the list promises Enter launches the selection; typing a
	// filter should not be the one place that stops being true.
	mv.searchEntry.onEnter = func() {
		if mv.selectionSurvivesFilter() {
			mv.runSelected()
		}
	}
	mv.updateEmptyState()

	dragHint := widget.NewLabel("Drag rows to reorder")
	dragHint.Importance = widget.LowImportance
	dragHint.Alignment = fyne.TextAlignCenter

	legendItem := func(s DotState, label string) fyne.CanvasObject {
		d := newStatusDot()
		d.SetState(s)
		txt := widget.NewLabel(label)
		txt.Importance = widget.LowImportance
		txt.SizeName = theme.SizeNameCaptionText
		return container.New(layout.NewCustomPaddedHBoxLayout(0), d, txt) // no dot-to-label gap; the label's own padding spaces them
	}
	// Marker entries are glyphs, not status dots, so they get their own swatches.
	glyphLegend := func(glyph fyne.CanvasObject, label string) fyne.CanvasObject {
		txt := widget.NewLabel(label)
		txt.Importance = widget.LowImportance
		txt.SizeName = theme.SizeNameCaptionText
		return container.New(layout.NewCustomPaddedHBoxLayout(0), glyph, txt)
	}
	// Content-sized columns packed tight and centered: small gaps, spare space at the ends, dots aligned per column.
	legendCol := func(top, bottom fyne.CanvasObject) fyne.CanvasObject {
		return container.New(layout.NewCustomPaddedVBoxLayout(-theme.Padding()), top, bottom)
	}
	legend := container.NewCenter(container.New(layout.NewCustomPaddedHBoxLayout(theme.Padding()*2),
		legendCol(legendItem(DotGreen, "Ready"), legendItem(DotGrey, "Checking")),
		legendCol(legendItem(DotOrange, "Update"), glyphLegend(newStartupMarker(), "Startup")),
		legendCol(legendItem(DotRed, "Uninstalled"), glyphLegend(newSetupWarning(), "Invalid")),
	))

	// Separator marks the list/footer boundary; the top group is tightened so it lines up with the one above Run Selected.
	footerTop := container.New(layout.NewCustomPaddedVBoxLayout(-theme.Padding()),
		widget.NewSeparator(),
		container.New(layout.NewCustomPaddedVBoxLayout(-theme.Padding()-2), dragHint, legend),
	)
	footer := container.NewPadded(container.NewVBox(
		footerTop,
		container.NewGridWithColumns(3, mv.seeLogsBtn, manageInstallsBtn, settingsBtn),
	))

	listArea := container.NewStack(mv.profileList, mv.noResults, mv.firstRun)
	top := container.NewVBox(headerBand, container.NewPadded(mv.searchEntry))
	leftPanelObj := container.NewBorder(top, container.NewVBox(mv.undoBand, footer), nil, nil, listArea)
	leftPanel := NewThemedBox(ColorNameSidebar, leftPanelObj)

	detailsContent := container.NewVScroll(container.NewPadded(mv.detailsContainer))

	rightPanelObj := container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), mv.launchBand, actionsContent, container.NewPadded(selectionHint)),
		nil,
		nil,
		detailsContent,
	)
	rightPanel := NewThemedBox(ColorNameContent, rightPanelObj)

	if mv.selectedIdx >= 0 && mv.selectedIdx < len(um.Config.Profiles) {
		if d := mv.displayPos(mv.selectedIdx); d >= 0 {
			mv.profileList.Select(widget.ListItemID(d))
		}
	}
	// A launch outlives the view that started it, so the newest view is always the
	// one that renders and settles it. This has to follow the widgets it touches.
	um.mainView = mv
	um.viewEscape = mv.clearSearch
	if um.launchInProgress {
		mv.adoptLaunch()
	}
	mv.updateButtonStates()
	mv.refreshDetails()

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.35

	headerLabel := widget.NewLabel("RunTTD")
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleBox := container.NewHBox(headerLabel, mutedLabel(versionCaption(um.Version)))
	var themeToggleBtn *viewButton
	themeToggleBtn = um.newViewIconButton("", theme.ColorPaletteIcon(), func() {
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(themeToggleBtn)
		pos.Y += themeToggleBtn.Size().Height
		um.showThemeCustomizer(pos)
	})

	themeToggleBtn.Importance = widget.LowImportance

	headerRight := container.NewHBox(themeToggleBtn)
	headerContent := container.NewBorder(nil, nil, nil, headerRight, titleBox)
	header := NewThemedBox(ColorNameHeader, container.NewPadded(headerContent))

	um.startUpdateCheck(headerRight)

	mainContent := container.NewBorder(header, nil, nil, nil, split)
	handleKey := func(event *fyne.KeyEvent) {
		if event.Name == fyne.KeyEscape {
			if action := um.escapeAction(); action != nil {
				action()
			}
			return
		}

		// Fyne's confirm dialogs build plain buttons that ignore Enter and take no
		// focus, so the default action has to be answered from here.
		if event.Name == fyne.KeyReturn || event.Name == fyne.KeyEnter {
			if um.confirmAction != nil && um.Window.Canvas().Overlays().Top() != nil {
				um.confirmAction()
				return
			}
		}

		// F5 targets the library view (a separate full-screen view, not mainContent),
		// so it's checked before the mainContent guard below. Bare keys (no modifier,
		// like Escape/Enter/digits) never reach Canvas.AddShortcut, so this and Delete
		// have to live in this same plain-key fallback.
		if event.Name == fyne.KeyF5 {
			um.runLibraryRescan()
			return
		}

		if um.Window.Content() != mainContent || um.Window.Canvas().Overlays().Top() != nil {
			return
		}

		if event.Name == fyne.KeyDelete {
			mv.deleteSelected()
			return
		}

		if len(event.Name) == 1 && event.Name[0] >= '0' && event.Name[0] <= '9' {
			if idx := mv.digitLaunchIndex(event.Name[0]); idx >= 0 {
				mv.profileList.Select(widget.ListItemID(mv.displayPos(idx)))
				mv.um.suppressAutoCloseOnce = false
				mv.launchIndex(idx)
				return
			}
		}

		if event.Name == fyne.KeyReturn || event.Name == fyne.KeyEnter {
			if mv.selectedIdx >= 0 {
				mv.runSelected()
			}
		}
	}
	um.Window.Canvas().SetOnTypedKey(handleKey)
	um.viewKeys = handleKey // a focused viewButton hands back what it did not use

	// Modifier-combo accelerators: Canvas.AddShortcut fires regardless of focus for
	// any focused widget that isn't Shortcutable (Button/Select/Check/List all
	// qualify), unlike the bare-key handler above. While an Entry has focus these
	// stay silent, matching Fyne's own built-in shortcuts (e.g. Ctrl+Z inside an
	// Entry only undoes text) rather than reaching here: expected, not a bug.
	mainViewGuard := func() bool {
		return um.Window.Content() == mainContent && um.Window.Canvas().Overlays().Top() == nil
	}
	um.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) {
		if mainViewGuard() {
			um.showProfileEditor(-1, true)
		}
	})
	um.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) {
		if mainViewGuard() {
			um.Window.Canvas().Focus(mv.searchEntry)
		}
	})
	um.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyD, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) {
		// Duplicate's button is disabled with nothing selected; guard the accelerator
		// the same way so it doesn't bypass that into an unexpected error dialog.
		if mainViewGuard() && mv.selectedIdx >= 0 {
			mv.duplicateSelected()
		}
	})
	um.Window.Canvas().AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierShortcutDefault}, func(fyne.Shortcut) {
		if mainViewGuard() {
			um.showSettingsView()
		}
	})

	// A "Save & run" from the editor defers its launch to here, so it goes through
	// the normal path (AutoOpenLog + launch band) once this view is live.
	if um.pendingLaunchIdx >= 0 {
		idx := um.pendingLaunchIdx
		um.pendingLaunchIdx = -1
		fyne.Do(func() { mv.launchIndex(idx) })
	}

	return mainContent
}
