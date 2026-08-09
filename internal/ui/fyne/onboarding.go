package fyne

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
)

// makeOnboardingView creates the first-run configuration screen
func (um *UIManager) makeOnboardingView() fyne.CanvasObject {
	welcomeLabel := widget.NewLabel("Set up RunTTD - " + versionCaption(um.Version))
	welcomeLabel.TextStyle = fyne.TextStyle{Bold: true, Italic: false}
	welcomeLabel.Alignment = fyne.TextAlignCenter

	subtitle := NewSectionDescription("Confirm your installation folders and preferences to get started.")
	subtitle.Alignment = fyne.TextAlignCenter

	// Every field's Enter routes here; the action itself is defined once the
	// widgets it reads exist.
	var tryContinue func()
	onEnter := func() { tryContinue() }

	// --- Installation paths ---
	parentDirEntry := newDialogEntry(nil, onEnter)
	parentDirEntry.SetText(um.Config.ParentDir)
	parentDirEntry.SetPlaceHolder("Folder where OpenTTD game files / executables will be automatically installed")

	var parentDirBtn *dialogButton
	parentDirBtn = newDialogButton("Browse…", func() {
		parentDirBtn.Disable()
		um.browseDirectory(&parentDirEntry.Entry, "Select Parent Directory", "Parent Directory", parentDirBtn.Enable)
	}, nil)

	docsBasePathEntry := newDialogEntry(nil, onEnter)
	docsBasePathEntry.SetText(um.Config.DocsBasePath)
	docsBasePathEntry.SetPlaceHolder("Folder where your saves and configuration (openttd.cfg) are stored")

	validationIcon := widget.NewIcon(theme.CancelIcon())
	validationIcon.Hide()
	validationLabel := mutedLabel("")
	validationLabel.Hide()

	var docsBasePathBtn *dialogButton
	docsBasePathBtn = newDialogButton("Browse…", func() {
		docsBasePathBtn.Disable()
		um.browseDirectory(&docsBasePathEntry.Entry, "Select Docs Base Path", "Docs Base Path", docsBasePathBtn.Enable)
	}, nil)

	// --- Preferences ---
	// Default Client is required: starts unselected (unless config has one) and gates Continue below.
	defaultClientSelect := newDialogSelect(defaultClientOptions, func(string) {}, nil, onEnter)
	if label, ok := revDefaultClientMap[um.Config.DefaultClient]; ok {
		defaultClientSelect.SetSelected(label)
	}

	subfolderCheck, subfolderGroup := NewLabeledCheckWithDescription(
		"Organise downloaded clients into per-client subfolders",
		"Keeps each client's downloaded files in a separate folder, instead of all sharing the parent folder. "+
			"Easiest to choose now, before anything is downloaded; you can change it later in Settings.",
		um.Config.SubfolderPerClient, nil, nil,
	)

	autoCloseCheck, autoCloseGroup := NewLabeledCheckWithDescription(
		"Auto-close launcher when OpenTTD starts",
		"Hides the launcher once the game opens. You can change it later in Settings.",
		um.Config.AutoCloseOnStart, nil, nil,
	)

	statusLabel := widget.NewLabel("")
	statusLabel.Wrapping = fyne.TextWrapWord

	// validate is the Continue handler's backstop; the disabled button + live hint (updateState) are the primary guard.
	validate := func() bool {
		return strings.TrimSpace(parentDirEntry.Text) != "" &&
			strings.TrimSpace(docsBasePathEntry.Text) != "" &&
			defaultClientSelect.Selected != ""
	}

	continueBtn := newDialogButton("Continue", func() {
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
		// First write of the config; a failure here means setup runs again next launch.
		if err := domain.SaveConfig(um.ConfigPath, um.Config); err != nil {
			// Stay on setup, with FirstRun intact: clearing it before a failed
			// save would let a later window-close write skip setup for good.
			um.showErrorf("could not save your settings to %s: %w", um.ConfigPath, err)
			return
		}
		um.Config.FirstRun = false

		um.Window.SetContent(um.makeMainView())
	}, nil)
	continueBtn.Importance = widget.HighImportance
	// Enter from anywhere on the screen, including with nothing focused, which is
	// how the view opens: there is no other view to inherit a key handler from.
	tryContinue = func() {
		if !continueBtn.Disabled() {
			continueBtn.OnTapped()
		}
	}
	um.Window.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		// A dialog on top (the failed-save error) owns the keys: Enter must
		// answer it, not re-run the save and stack a second dialog over it.
		if top := um.Window.Canvas().Overlays().Top(); top != nil {
			switch ev.Name {
			case fyne.KeyReturn, fyne.KeyEnter:
				if um.confirmAction != nil {
					um.confirmAction()
				}
			case fyne.KeyEscape:
				um.confirmAction = nil
				top.Hide()
			}
			return
		}
		if ev.Name == fyne.KeyReturn || ev.Name == fyne.KeyEnter {
			tryContinue()
		}
	})

	// Debounced: os.Stat runs 300ms after typing pauses, off the UI thread, since
	// the path can be an unreachable network share (cancel-and-reset, not just a
	// discard-if-stale check, so a burst of keystrokes triggers exactly one stat).
	// docsCheckGuard additionally covers Timer.Stop()'s documented gap (it can't
	// cancel a callback that already started): a slow, superseded check is
	// dropped by generation rather than allowed to overwrite a newer result.
	var docsCheckTimer *time.Timer
	var docsCheckGuard debounceGuard
	updateDocsValidation := func(path string) {
		if docsCheckTimer != nil {
			docsCheckTimer.Stop()
		}
		if path == "" {
			docsCheckGuard.next() // supersede any in-flight check; there's nothing to show
			validationIcon.Hide()
			validationLabel.Hide()
			return
		}
		gen := docsCheckGuard.next()
		docsCheckTimer = time.AfterFunc(300*time.Millisecond, func() {
			cfgPath := filepath.Join(path, "openttd.cfg")
			_, statErr := os.Stat(cfgPath)
			fyne.Do(func() {
				if !docsCheckGuard.current(gen) {
					return // superseded by a newer check
				}
				if statErr == nil {
					validationIcon.SetResource(theme.ConfirmIcon())
					validationLabel.SetText("openttd.cfg found")
				} else {
					validationIcon.SetResource(theme.CancelIcon())
					validationLabel.SetText("No openttd.cfg here")
				}
				validationIcon.Show()
				validationLabel.Show()
			})
		})
	}

	// updateState gates Continue on the required inputs, with the hint naming what's missing rather than leaving the button inertly greyed out.
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
			container.NewBorder(nil, nil, nil, container.NewHBox(validationIcon, validationLabel, docsBasePathBtn), docsBasePathEntry),
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
