package fyne

import (
	"context"
	"fmt"
	"image/color"
	neturl "net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/platform"
)

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

// addUpdatePill prepends an accent-coloured "update available" pill to the header
// (left of the theme button) that opens the release page when clicked. Must be
// called on the main goroutine (inside fyne.Do or a UI callback).
func (um *UIManager) addUpdatePill(headerRight *fyne.Container, tag, releaseURL string) {
	pill := um.newViewButton("↻  Update to "+tag, func() {
		if u, perr := neturl.Parse(releaseURL); perr == nil {
			if err := fyne.CurrentApp().OpenURL(u); err != nil {
				um.Logger.Append(fmt.Sprintf("could not open the release page for %s: %v", tag, err))
			}
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

// showThemeCustomizer presents the preset accent colour circular items and mode toggles
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

		btn := um.newViewButton("", func() {
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
