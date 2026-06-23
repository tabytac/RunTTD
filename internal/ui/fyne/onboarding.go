package fyne

import (
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
)

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
