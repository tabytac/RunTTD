package fyne

import (
	"context"
	"fmt"
	"image/color"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	fyneadvancedlist "github.com/dweymouth/fyne-advanced-list"

	apppkg "runttd/internal/app"
	"runttd/internal/platform"
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

	detailsContainer *fyne.Container
	profileList      *fyneadvancedlist.List
	launchPhase      *widget.Label
	launchBar        *widget.ProgressBar
	launchSpin       *widget.ProgressBarInfinite
	launchLogsBtn    *widget.Button
	cancelBtn        *widget.Button
	launchBand       *ThemedBox
	runBtn           *widget.Button
	editBtn          *widget.Button
	duplicateBtn     *widget.Button
	deleteBtn        *widget.Button
	seeLogsBtn       *widget.Button
	searchEntry      *searchEntry
	noResults        *fyne.Container
	firstRun         *fyne.Container
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

// launchIndex starts the profile at idx, honoring the AutoOpenLog setting:
// either open the log view or launch in the background with toast + button
// feedback. Used by the Run button, Enter, and the digit quick-launch keys so
// all three behave identically.
func (mv *mainView) launchIndex(idx int) {
	um := mv.um
	if idx < 0 || idx >= len(um.Config.Profiles) {
		um.showError("select a profile to launch")
		return
	}
	if um.Config.AutoOpenLog {
		um.showLogView(idx)
		return
	}
	// um.launchInProgress is the cross-path guard (also checked by showLogView);
	// mv.launchInProgress mirrors it for this view's own button/band state, since
	// AutoOpenLog can be toggled mid-launch, switching which path a later Run
	// takes without the launch itself having finished.
	if um.launchInProgress {
		return
	}
	um.launchInProgress = true
	mv.launchInProgress = true
	mv.launchGen++

	profile := um.Config.Profiles[idx]
	mv.launchLogsIdx = idx

	// Reset the band to a fresh "working" state (marquee until download starts).
	mv.launchLogsBtn.Hide()
	mv.launchBar.Hide()
	mv.launchSpin.Show()
	mv.launchPhase.Importance = widget.MediumImportance
	mv.launchPhase.TextStyle = fyne.TextStyle{}
	mv.launchPhase.SetText("Starting " + profile.Name)
	mv.launchBand.Show()
	mv.launchBand.Refresh()
	mv.runBtn.Disable()

	ctx, cancel := context.WithCancel(context.Background())
	mv.launchCancel = cancel
	mv.cancelBtn.Show()

	failed := false
	lastPct := -1
	go func() {
		defer fyne.Do(func() {
			um.launchInProgress = false
			mv.launchInProgress = false
			cancel()
			mv.launchCancel = nil
			mv.cancelBtn.Hide()
			mv.launchSpin.Hide()
			mv.launchBar.Hide()
			mv.runBtn.Enable()
			mv.updateButtonStates()
			if failed {
				mv.launchPhase.Importance = widget.DangerImportance
				mv.launchPhase.SetText(strings.TrimPrefix(mv.launchPhase.Text, "Failed: "))
				mv.launchPhase.Refresh()
				mv.launchLogsBtn.Show()
				return
			}
			mv.launchPhase.Importance = widget.MediumImportance
			mv.launchPhase.TextStyle = fyne.TextStyle{Bold: true}
			mv.launchPhase.SetText("Launched " + profile.Name)
			mv.launchPhase.Refresh()
			if um.profileListRefresh != nil {
				um.profileListRefresh() // launchProfile already invalidated the disk cache on a fresh download
			}
			gen := mv.launchGen
			go func() {
				time.Sleep(6000 * time.Millisecond)
				fyne.Do(func() {
					if mv.shouldAutoHide(gen) {
						mv.launchBand.Hide()
					}
				})
			}()
		})
		um.launchProfile(ctx, profile,
			func(status string) {
				fyne.Do(func() { mv.launchPhase.SetText(status) })
			},
			func(done, total int64) {
				if total <= 0 {
					return // unknown size: stay on the marquee
				}
				if done >= total {
					fyne.Do(func() {
						mv.launchBar.Hide()
						mv.launchSpin.Show()
						mv.launchPhase.SetText("Extracting")
						mv.cancelBtn.Hide() // extraction isn't cancellable; stop offering to
					})
					return
				}
				pct := int(done * 100 / total)
				if pct == lastPct {
					return // throttle to whole-percent steps
				}
				lastPct = pct
				fyne.Do(func() {
					mv.launchSpin.Hide()
					mv.launchBar.Show()
					mv.launchBar.SetValue(float64(done) / float64(total))
				})
			},
			func() { failed = true },
		)
	}()
}

// shouldAutoHide reports whether a success auto-hide timer started for gen may
// still hide the band: only if no newer launch has bumped launchGen and none is
// in flight.
func (mv *mainView) shouldAutoHide(gen int) bool {
	return gen == mv.launchGen && !mv.launchInProgress
}

func (mv *mainView) updateButtonStates() {
	if mv.selectedIdx >= 0 {
		mv.runBtn.Enable()
		mv.editBtn.Enable()
		mv.duplicateBtn.Enable()
		mv.deleteBtn.Enable()
	} else {
		mv.runBtn.Disable()
		mv.editBtn.Disable()
		mv.duplicateBtn.Disable()
		mv.deleteBtn.Disable()
	}

	if mv.seeLogsBtn != nil {
		if mv.um.Logger.Len() > 0 {
			mv.seeLogsBtn.Enable()
		} else {
			mv.seeLogsBtn.Disable()
		}
	}
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
	mv.detailsContainer.Add(NewThemedBox(ColorNameDetailHeader, container.NewPadded(header)))

	// In-context auto-launch toggle; persists immediately, unlike the save-gated dialog.
	isStartup := profile.Name == um.Config.AutoLaunchProfile && um.Config.AutoLaunchProfile != ""
	autoBtn := widget.NewButton("", nil)
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
	adv.addLongField("Arguments", profile.ExtraArgs, true)
	adv.emit("Advanced", mv.detailsContainer)

	mv.detailsContainer.Refresh()
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

// deleteSelected confirms, then removes the selected profile and reselects a
// neighbour. Refuses to delete the last remaining profile.
func (mv *mainView) deleteSelected() {
	um := mv.um
	if mv.selectedIdx < 0 {
		return
	}
	if len(um.Config.Profiles) <= 1 {
		um.showError("cannot delete the last profile")
		return
	}
	profileName := um.Config.Profiles[mv.selectedIdx].Name
	dialog.NewConfirm(
		"Delete Profile",
		fmt.Sprintf("Are you sure you want to delete profile %q?", profileName),
		func(confirmed bool) {
			if !confirmed {
				return
			}

			um.Config.Profiles = append(um.Config.Profiles[:mv.selectedIdx], um.Config.Profiles[mv.selectedIdx+1:]...)
			um.saveConfigOrWarn()

			nextIdx := mv.selectedIdx
			if nextIdx >= len(um.Config.Profiles) {
				nextIdx = len(um.Config.Profiles) - 1
			}

			mv.selectedIdx = nextIdx
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
		},
		um.Window,
	).Show()
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
			right := row.Objects[2].(*fyne.Container)                       // the HBox
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

// escapeOverlayAction resolves what Escape should do for the given top overlay, or nil
// if there's nothing to dismiss. Scoped overlays (settings, editor, a blocking confirm)
// route through their own dismiss so callbacks still fire; raw top.Hide() would skip them.
func (um *UIManager) escapeOverlayAction(top fyne.CanvasObject) func() {
	switch {
	case top == nil:
		return nil
	case top == um.settingsOverlay && um.settingsOnEscape != nil:
		return um.settingsOnEscape
	case top == um.editorOverlay && um.editorOnEscape != nil:
		return um.editorOnEscape
	case top == um.blockingConfirm && um.blockingConfirmHide != nil:
		return um.blockingConfirmHide
	default:
		return top.Hide
	}
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

	selectionHint := widget.NewLabel("Press 1–9 (0 for 10th) to quick-launch · Enter or double-click to launch selected")
	selectionHint.Importance = widget.LowImportance
	selectionHint.Alignment = fyne.TextAlignCenter
	selectionHint.Wrapping = fyne.TextWrapWord

	// Launch status band: feedback for background launches (log auto-open off).
	// Hidden until a launch runs; kept a constant height (sized to the View logs
	// row) so it never resizes as it moves through phases.
	mv.launchPhase = widget.NewLabel("")
	mv.launchPhase.Wrapping = fyne.TextWrapWord
	mv.launchBar = widget.NewProgressBar()
	mv.launchSpin = widget.NewProgressBarInfinite()
	mv.launchLogsBtn = widget.NewButton("View logs", func() {
		if mv.launchLogsIdx >= 0 {
			um.showLogView(mv.launchLogsIdx)
		}
	})
	mv.launchLogsBtn.Importance = widget.LowImportance
	mv.cancelBtn = widget.NewButton("Cancel", func() {
		if mv.launchCancel != nil {
			mv.launchCancel()
		}
	})
	mv.cancelBtn.Importance = widget.DangerImportance
	mv.cancelBtn.Hide()

	launchBars := container.NewStack(mv.launchSpin, mv.launchBar)
	// Cancel shares the bar's row via Border (bar takes the remaining space, Cancel
	// pins right) rather than another full-width Stack layer, which would draw the
	// button on top of the bar instead of beside it — Border skips a hidden Right
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

	newBtn := widget.NewButtonWithIcon("New", theme.ContentAddIcon(), func() {
		um.showProfileEditor(-1, true)
	})
	newBtn.Importance = widget.HighImportance

	mv.editBtn = widget.NewButton("Edit", func() {
		if mv.selectedIdx >= 0 {
			um.showProfileEditor(mv.selectedIdx, false)
		} else {
			um.showError("select a profile to edit")
		}
	})
	mv.duplicateBtn = widget.NewButton("Duplicate", mv.duplicateSelected)
	mv.deleteBtn = widget.NewButton("Delete", mv.deleteSelected)

	mv.runBtn = widget.NewButton("Run Selected", mv.runSelected)
	mv.runBtn.Importance = widget.HighImportance

	mv.seeLogsBtn = widget.NewButtonWithIcon("Logs", theme.DocumentIcon(), func() {
		um.showLogView(-1)
	})
	mv.seeLogsBtn.Importance = widget.LowImportance

	manageInstallsBtn := widget.NewButtonWithIcon("Installs", theme.StorageIcon(), func() {
		um.showLibraryView()
	})
	manageInstallsBtn.Importance = widget.LowImportance

	settingsBtn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
		um.showSettingsView()
	})
	settingsBtn.Importance = widget.LowImportance

	actionsContent := container.NewVBox(
		mv.runBtn,
		container.NewGridWithColumns(3, mv.editBtn, mv.duplicateBtn, mv.deleteBtn),
	)

	mv.searchEntry = newSearchEntry()
	mv.searchEntry.SetPlaceHolder("Search profiles...")

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
	mv.noResults = container.NewCenter(container.NewVBox(
		centeredLabel("No profiles match your search."),
		mutedCenteredLabel("Press Esc to clear."),
	))
	mv.noResults.Hide()

	// First-run state (no profiles exist yet).
	firstRunBtn := widget.NewButtonWithIcon("New Profile", theme.ContentAddIcon(), func() {
		um.showProfileEditor(-1, true)
	})
	firstRunBtn.Importance = widget.HighImportance
	mv.firstRun = container.NewCenter(container.NewVBox(
		centeredLabel("No profiles yet."),
		mutedCenteredLabel("Create your first profile to get started."),
		container.NewCenter(firstRunBtn),
	))
	mv.firstRun.Hide()

	mv.searchEntry.OnChanged = func(s string) {
		mv.filterText = s
		mv.recomputeVisible()
		// Reordering a filtered subset is ambiguous; only allow drag with no filter.
		mv.profileList.EnableDragging = strings.TrimSpace(s) == ""
		mv.profileList.UnselectAll()
		mv.selectProfile(-1)
		mv.refreshDetails()
		mv.updateEmptyState()
		mv.profileList.Refresh()
	}
	// Esc clears the filter and returns to the full list, then blurs so quick-launch
	// (digits, Enter) works again immediately rather than staying dead until the user
	// clicks elsewhere: Fyne routes keys to the focused widget OR the canvas, never
	// both, so this Entry keeping focus would otherwise swallow every later key.
	mv.searchEntry.onEscape = func() {
		if mv.searchEntry.Text != "" {
			mv.searchEntry.SetText("") // triggers OnChanged, which resets the filter
		}
		um.Window.Canvas().Unfocus()
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
	leftPanelObj := container.NewBorder(top, footer, nil, nil, listArea)
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
	mv.updateButtonStates()
	mv.refreshDetails()

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.35

	headerLabel := widget.NewLabel("RunTTD")
	headerLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleBox := container.NewHBox(headerLabel, mutedLabel(versionCaption(um.Version)))
	var themeToggleBtn *widget.Button
	themeToggleBtn = widget.NewButtonWithIcon("", theme.ColorPaletteIcon(), func() {
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
	um.Window.Canvas().SetOnTypedKey(func(event *fyne.KeyEvent) {
		// Escape dismisses the top modal (profile editor, settings, theme
		// customizer), matching each modal's Cancel button which only hides it.
		if event.Name == fyne.KeyEscape {
			if action := um.escapeOverlayAction(um.Window.Canvas().Overlays().Top()); action != nil {
				action()
			}
			return
		}

		// F5 targets the library view (a separate full-screen view, not mainContent),
		// so it's checked before the mainContent guard below. Bare keys (no modifier,
		// like Escape/Enter/digits) never reach Canvas.AddShortcut, so this and Delete
		// have to live in this same plain-key fallback.
		if event.Name == fyne.KeyF5 {
			if um.libraryRescan != nil && um.Window.Canvas().Overlays().Top() == nil {
				um.libraryRescan()
			}
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
	})

	// Modifier-combo accelerators: Canvas.AddShortcut fires regardless of focus for
	// any focused widget that isn't Shortcutable (Button/Select/Check/List all
	// qualify), unlike the bare-key handler above. While an Entry has focus these
	// stay silent, matching Fyne's own built-in shortcuts (e.g. Ctrl+Z inside an
	// Entry only undoes text) rather than reaching here — expected, not a bug.
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

	// A "Save & Run" from the editor defers its launch to here, so it goes through
	// the normal path (AutoOpenLog + launch band) once this view is live.
	if um.pendingLaunchIdx >= 0 {
		idx := um.pendingLaunchIdx
		um.pendingLaunchIdx = -1
		fyne.Do(func() { mv.launchIndex(idx) })
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

// applyAppearance persists the theme variant + accent preset and applies them live.
// Shared by the header palette popover and the settings Appearance tab so the two
// entry points can't drift.
func (um *UIManager) applyAppearance(variant string, presetIdx int) {
	um.Config.ThemeVariant = variant
	um.Config.AccentPreset = presetIdx
	if pt, ok := um.App.Settings().Theme().(*LauncherTheme); ok {
		pt.UpdateAccent(presetIdx, variant)
	}
	um.saveConfigOrWarn()
}

// setAutoLaunchProfile records the single startup profile (or "" for off), persists,
// and refreshes the list so the marker moves. The writer for the instant main-view
// toggle; the settings dialog writes the field in its batch save then calls profileListRefresh.
func (um *UIManager) setAutoLaunchProfile(name string) {
	um.Config.AutoLaunchProfile = name
	um.saveConfigOrWarn()
	if um.profileListRefresh != nil {
		um.profileListRefresh()
	}
}

// showThemeCustomizer presents the preset accent color circular items and mode toggles
func (um *UIManager) showThemeCustomizer(pos fyne.Position) {
	apply := um.applyAppearance

	var currentMode string
	if um.Config.ThemeVariant == "light" {
		currentMode = "Light"
	} else {
		currentMode = "Dark"
	}

	modeSelect := NewSegmentedRadio([]string{"Light", "Dark"}, currentMode, func(s string) {
		apply(strings.ToLower(s), um.Config.AccentPreset)
	}, nil)

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
		widget.NewLabel("Theme"),
		modeSelect.Container,
		widget.NewLabel("Accent Colour"),
		colorGrid,
	)

	widget.NewPopUp(container.NewPadded(content), um.Window.Canvas()).ShowAtPosition(pos)
}
