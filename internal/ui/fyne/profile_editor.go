package fyne

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/app"
	"runttd/internal/domain"
)

// valueOrDefault returns def if val is empty or whitespace
func valueOrDefault(val, def string) string {
	if strings.TrimSpace(val) == "" {
		return def
	}
	return val
}

// trimSavePrefix strips any leading save/ or save\ segments; the launch path
// re-joins SavePath against docs/save, so a stored "save/" prefix would double up.
func trimSavePrefix(p string) string {
	for {
		lowered := strings.ToLower(p)
		if strings.HasPrefix(lowered, "save/") || strings.HasPrefix(lowered, "save\\") {
			p = p[5:]
			continue
		}
		return p
	}
}

// pathExistsWarning returns a "<label> not found" warning if raw resolves to a
// missing path, or "" when empty or present. Relative paths resolve against base
// (the same resolution the launch path uses), so the warning matches what would
// actually happen at launch.
func pathExistsWarning(label, raw, base string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) && base != "" {
		p = filepath.Join(base, p)
	}
	if _, err := os.Stat(p); err != nil {
		return fmt.Sprintf("Warning: %s not found: %s", label, p)
	}
	return ""
}

// versionTrackHintText returns the per-client note about what "latest" resolves
// to, or "" for nightly/custom. JGRPP "latest" is stable-only by design.
func versionTrackHintText(clientID string) string {
	switch clientID {
	case "vanilla":
		return "\"latest (Stable)\" tracks final releases only.\n" +
			"\"latest (Testing)\" also includes betas and release candidates."
	case "jgrpp":
		return "\"latest\" installs the newest stable release.\n" +
			"To play a pre-release, manually pick the version."
	default:
		return ""
	}
}

// defaultVersionOptions returns the version dropdown presets for a client track.
func defaultVersionOptions(clientID string) []string {
	switch clientID {
	case "vanilla":
		return []string{"latest (Stable)", "latest (Testing)"}
	case "vanilla-nightly":
		return []string{"latest"}
	default:
		return []string{"latest"}
	}
}

// displayVersion turns a stored version into the field's display string, normalizing the "latest" aliases per track.
func displayVersion(clientID, stored string) string {
	s := strings.TrimSpace(stored)
	lower := strings.ToLower(s)
	switch clientID {
	case "vanilla", "vanilla-nightly":
		if clientID == "vanilla-nightly" {
			switch lower {
			case "", "latest", "latest-stable", "latest-testing", "latest (stable)", "latest (testing)":
				return "latest"
			default:
				return s
			}
		}
		switch lower {
		case "", "latest", "latest-stable", "latest (stable)":
			return "latest (Stable)"
		case "latest-testing", "latest (testing)":
			return "latest (Testing)"
		default:
			return s
		}
	default:
		if s == "" {
			return "latest"
		}
		return s
	}
}

// storedVersion is the inverse of displayVersion: field text to the canonical value persisted on the profile.
func storedVersion(clientID, entered string) string {
	s := strings.TrimSpace(entered)
	lower := strings.ToLower(s)
	switch clientID {
	case "vanilla", "vanilla-nightly":
		if clientID == "vanilla-nightly" {
			if lower == "" || lower == "latest" || lower == "latest-stable" || lower == "latest-testing" || lower == "latest (stable)" || lower == "latest (testing)" {
				return ""
			}
			return s
		}
		switch lower {
		case "", "latest", "latest-stable", "latest (stable)":
			return "latest-stable"
		case "latest-testing", "latest (testing)":
			return "latest-testing"
		default:
			return s
		}
	default:
		if lower == "" || lower == "latest" {
			return ""
		}
		return s
	}
}

// showProfileEditor displays the edit modal popup for creating or updating profiles
func (um *UIManager) showProfileEditor(profileIdx int, isNew bool) {
	var profile domain.Profile
	if !isNew {
		profile = um.Config.Profiles[profileIdx]
	}

	// Form fields
	nameEntry := widget.NewEntry()
	nameEntry.SetText(profile.Name)
	nameEntry.SetPlaceHolder("Profile name")

	versionEntry := widget.NewSelectEntry(um.CachedVersions)
	versionEntry.SetOptions([]string{"latest (Stable)", "latest (Testing)"})
	versionEntry.PlaceHolder = "latest (Stable), latest (Testing), or 15.3"

	customFolderEntry := widget.NewEntry()
	customFolderEntry.SetText(profile.CustomExecutablePath)
	customFolderEntry.SetPlaceHolder("Folder containing openttd executable")

	customFolderBtn := widget.NewButton("Browse...", func() {
		um.browseDirectory(customFolderEntry, "Select Custom Executable Folder", "Custom Executable Folder")
	})
	customFolderRow := container.NewBorder(nil, nil, nil, customFolderBtn, customFolderEntry)

	// Text set per client in updateClientFields via versionTrackHintText.
	versionTrackHint := NewSectionDescription("")
	versionTrackHint.Hide()

	versionGroup := container.NewVBox(widget.NewLabel("Version"), versionEntry, versionTrackHint)
	customFolderGroup := container.NewVBox(widget.NewLabel("Executable Folder"), customFolderRow)

	updateClientFields := func(clientID string) {
		if clientID == "custom" {
			versionGroup.Hide()
			customFolderGroup.Show()
		} else {
			versionGroup.Show()
			customFolderGroup.Hide()
		}
		if hint := versionTrackHintText(clientID); hint != "" {
			versionTrackHint.SetText(hint)
			versionTrackHint.Show()
		} else {
			versionTrackHint.Hide()
		}
	}

	// Client selection (Vanilla OpenTTD Stable/Nightly, JGRPP, Custom Executable).
	fetchVersionsForClient := func(clientID string) {
		go func() {
			versions, err := app.ClientFetchVersions(context.Background(), clientID, um.Config)
			if err == nil && len(versions) > 0 {
				fyne.Do(func() {
					um.CachedVersions = versions
					versionEntry.SetOptions(versions)
					versionEntry.Refresh()
				})
			}
		}()
	}
	// updateState is defined later, but the client radio callback may need to call it.
	var updateState func()
	initializingClientSelection := true
	clientSelect := NewSegmentedRadio(defaultClientOptions, "", func(s string) {
		cli := defaultClientMap[s]
		updateClientFields(cli)
		if initializingClientSelection {
			versionEntry.SetOptions(defaultVersionOptions(cli))
			versionEntry.Refresh()
			fetchVersionsForClient(cli)
			return
		}
		versionEntry.SetText(displayVersion(cli, ""))
		versionEntry.SetOptions(defaultVersionOptions(cli))
		versionEntry.Refresh()
		fetchVersionsForClient(cli)
		if updateState != nil {
			updateState()
		}
	})
	// Preselect client and version display for new and existing profiles.
	currentClient := strings.TrimSpace(profile.Client)
	if currentClient == "" {
		currentClient = strings.TrimSpace(um.Config.DefaultClient)
		if currentClient == "" {
			currentClient = "jgrpp"
		}
	}
	versionEntry.SetOptions(defaultVersionOptions(currentClient))
	versionEntry.SetText(displayVersion(currentClient, profile.Version))
	if sel, ok := revDefaultClientMap[currentClient]; ok {
		clientSelect.SetSelected(sel)
	}
	updateClientFields(currentClient)
	initializingClientSelection = false
	fetchVersionsForClient(currentClient)

	// Auto-detect mode for legacy configs or unsaved changes
	if profile.LaunchMode == "" {
		if profile.ServerIpPort != "" {
			profile.LaunchMode = "multiplayer"
		} else if profile.SavePath != "" {
			abs := profile.SavePath
			if !filepath.IsAbs(abs) && um.Config.DocsBasePath != "" {
				abs = filepath.Join(um.Config.DocsBasePath, "save", abs)
			}
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				profile.LaunchMode = "folder"
			} else {
				profile.LaunchMode = "file"
			}
		}
	}

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

	modeSelect := NewSegmentedRadio([]string{"Main Menu", "Load File", "Latest in Folder", "Multiplayer"}, revModeMap[profile.LaunchMode], func(s string) {
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

	configFileEntry := widget.NewEntry()
	configFileEntry.SetText(profile.ConfigFilePath)
	configFileEntry.SetPlaceHolder("Optional path to config file (openttd.cfg)")

	noConfigSaveCheck := widget.NewCheck("Do not save config changes on exit", nil)
	noConfigSaveCheck.SetChecked(profile.NoConfigSave)

	browseConfigBtn := widget.NewButtonWithIcon("Browse...", theme.FileIcon(), func() {
		go func() {
			startPath := strings.TrimSpace(configFileEntry.Text)
			if startPath == "" && um.Config.DocsBasePath != "" {
				startPath = filepath.Join(um.Config.DocsBasePath, "openttd.cfg")
			} else if startPath != "" && !filepath.IsAbs(startPath) && um.Config.DocsBasePath != "" {
				startPath = filepath.Join(um.Config.DocsBasePath, startPath)
			}

			path, err := um.browseConfigPath(startPath)
			if err != nil {
				fyne.Do(func() {
					um.showErrorf("could not open config picker: %w", err)
				})
				return
			}
			if path == "" {
				return
			}

			fyne.Do(func() {
				configFileEntry.SetText(path)
			})
		}()
	})

	// NewGRF scan mode selection
	newgrfLabelMap := map[string]string{
		"":   "Default",
		"Q":  "Skip Startup",
		"QQ": "Disable All",
	}
	revNewgrfLabelMap := map[string]string{
		"Default":      "",
		"Skip Startup": "Q",
		"Disable All":  "QQ",
	}
	initialNewgrf := profile.NewGRFScanMode
	newgrfRadio := NewSegmentedRadio([]string{newgrfLabelMap[""], newgrfLabelMap["Q"], newgrfLabelMap["QQ"]}, newgrfLabelMap[initialNewgrf], func(s string) {
		// no-op; selection read on save
	})

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

	autoLatestFilterRadio := NewSegmentedRadio([]string{"Saves & Scenarios", "Saves Only", "Scenarios Only"}, filterLabelMap[initialFilter], func(s string) {
		updateFolderInstructions(s)
	})
	updateFolderInstructions(autoLatestFilterRadio.Selected)

	browseFileBtn := widget.NewButtonWithIcon("Browse File...", theme.FileIcon(), func() {
		go func() {
			startPath := savePathEntry.Text
			if startPath == "" {
				if um.Config.DocsBasePath != "" {
					startPath = filepath.Join(um.Config.DocsBasePath, "save")
				}
			} else if !filepath.IsAbs(startPath) && um.Config.DocsBasePath != "" {
				startPath = filepath.Join(um.Config.DocsBasePath, "save", startPath)
			}

			path, err := um.browseSavePath(startPath, "Select Save or Scenario", false)
			if err != nil {
				fyne.Do(func() {
					um.showErrorf("could not open file picker: %w", err)
				})
				return
			}
			if path == "" {
				return
			}
			fyne.Do(func() {
				savePathEntry.SetText(path)
			})
		}()
	})

	browseFolderBtn := widget.NewButtonWithIcon("Browse Folder...", theme.FolderIcon(), func() {
		go func() {
			startPath := savePathEntry.Text
			if startPath == "" {
				if um.Config.DocsBasePath != "" {
					startPath = filepath.Join(um.Config.DocsBasePath, "save")
				}
			} else if !filepath.IsAbs(startPath) && um.Config.DocsBasePath != "" {
				startPath = filepath.Join(um.Config.DocsBasePath, "save", startPath)
			}

			path, err := um.browseSavePath(startPath, "Select Save Folder", true)
			if err != nil {
				fyne.Do(func() {
					um.showErrorf("could not open folder picker: %w", err)
				})
				return
			}
			if path == "" {
				return
			}
			fyne.Do(func() {
				savePathEntry.SetText(path)
			})
		}()
	})

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
		container.NewHBox(autoLatestFilterRadio.Container),
	}
	pathManualOption.Objects = []fyne.CanvasObject{
		widget.NewSeparator(),
		widget.NewLabel("Path (Relative or Absolute)"),
		savePathEntry,
	}
	multiplayerOption.Objects = []fyne.CanvasObject{
		NewSectionTitle("Server Connection"),
		widget.NewLabel("Server IP:Port"), ipPortEntry,
		widget.NewLabel("Server Password"), serverPassEntry,
		widget.NewSeparator(),
		NewSectionTitle("Company Details"),
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

		for i, p := range um.Config.Profiles {
			if i != profileIdx && strings.EqualFold(strings.TrimSpace(p.Name), name) {
				return false, "A profile with this name already exists."
			}
		}

		if strings.TrimSpace(clientSelect.Selected) == "" {
			return false, "Client selection is required."
		}

		if defaultClientMap[clientSelect.Selected] == "custom" {
			if strings.TrimSpace(customFolderEntry.Text) == "" {
				return false, "Executable folder is required for custom client."
			}
		} else if strings.TrimSpace(versionEntry.Text) == "" {
			return false, "Enter a version number, or use the dropdown to choose the latest or a specific release."
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
		if !ok {
			statusLabel.SetText(message)
			statusLabel.Refresh()
			return
		}

		if warn := pathExistsWarning("config file", configFileEntry.Text, um.Config.DocsBasePath); warn != "" {
			statusLabel.SetText(warn)
			statusLabel.Refresh()
			return
		}

		// Save path is only used in file/folder mode; resolve it the way launch
		// does (relative to docs/save, after stripping a leading save/ prefix).
		mode := modeMap[modeSelect.Selected]
		if mode == "file" || mode == "folder" {
			rawSave := trimSavePrefix(strings.TrimSpace(savePathEntry.Text))
			saveBase := filepath.Join(um.Config.DocsBasePath, "save")
			if warn := pathExistsWarning("save path", rawSave, saveBase); warn != "" {
				statusLabel.SetText(warn)
				statusLabel.Refresh()
				return
			}
		}

		statusLabel.SetText("")
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

		selectedClient := "jgrpp"
		if en := strings.TrimSpace(clientSelect.Selected); en != "" {
			if mapped, ok := defaultClientMap[en]; ok {
				selectedClient = mapped
			}
		}
		profile.Client = selectedClient
		if selectedClient == "custom" {
			profile.Version = ""
			profile.CustomExecutablePath = strings.TrimSpace(customFolderEntry.Text)
		} else {
			profile.Version = storedVersion(selectedClient, versionEntry.Text)
			profile.CustomExecutablePath = ""
		}

		profile.SavePath = trimSavePrefix(strings.TrimSpace(savePathEntry.Text))
		profile.ServerIpPort = strings.TrimSpace(ipPortEntry.Text)
		profile.ServerPassword = serverPassEntry.Text
		profile.ServerCompanyNumber = strings.TrimSpace(companyNumEntry.Text)
		profile.ServerCompanyPassword = companyPassEntry.Text
		profile.LaunchMode = modeMap[modeSelect.Selected]
		profile.AutoLatestFilter = revFilterLabelMap[autoLatestFilterRadio.Selected]
		profile.ExtraArgs = strings.TrimSpace(extraArgsEntry.Text)

		// Persist NewGRF scan mode
		if sel := newgrfRadio.Selected; sel != "" {
			if v, ok := revNewgrfLabelMap[sel]; ok {
				profile.NewGRFScanMode = v
			} else {
				profile.NewGRFScanMode = ""
			}
		} else {
			profile.NewGRFScanMode = ""
		}

		configPath := strings.TrimSpace(configFileEntry.Text)
		if configPath != "" && um.Config.DocsBasePath != "" && filepath.IsAbs(configPath) {
			if rel, err := filepath.Rel(um.Config.DocsBasePath, configPath); err == nil && !strings.HasPrefix(rel, "..") {
				configPath = rel
			}
		}
		profile.ConfigFilePath = configPath
		profile.NoConfigSave = noConfigSaveCheck.Checked

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
			um.Config.Profiles = append(um.Config.Profiles, profile)
			savedIdx = len(um.Config.Profiles) - 1
		} else {
			um.Config.Profiles[profileIdx] = profile
		}
		um.SelectedProfileName = profile.Name

		_ = domain.SaveConfig(um.ConfigPath, um.Config)
		editDialog.Hide()
		if runAfter {
			// Launch via the rebuilt main view so it honors AutoOpenLog and shows
			// the launch band, rather than always forcing the log view open.
			um.pendingLaunchIdx = savedIdx
		}
		um.Window.SetContent(um.makeMainView())
	}

	generalScroll := container.NewVScroll(container.NewVBox(
		NewSectionTitle("Identity"),
		widget.NewLabel("Name"), nameEntry,
		widget.NewLabel("Client"), clientSelect.Container,
		versionGroup,
		customFolderGroup,
	))
	generalTab := container.NewTabItemWithIcon("General Options", theme.InfoIcon(), generalScroll)

	launchScroll := container.NewVScroll(container.NewVBox(
		NewSectionTitle("How should the game start?"),
		modeSelect.Container,
		widget.NewSeparator(),
		optionsStack,
	))
	launchTab := container.NewTabItemWithIcon("Launch Options", theme.MediaPlayIcon(), launchScroll)

	advancedScroll := container.NewVScroll(container.NewVBox(
		NewSectionTitle("OpenTTD Config Behavior"),
		widget.NewLabel("Config File Override (optional)"),
		container.NewBorder(nil, nil, nil, browseConfigBtn, configFileEntry),
		noConfigSaveCheck,
		widget.NewSeparator(),
		NewSectionTitle("NewGRF Scan Behavior"),
		widget.NewLabel("Control NewGRF scanning/loading on startup:"),
		container.NewHBox(newgrfRadio.Container),
		widget.NewSeparator(),
		NewSectionTitle("Custom Command Line Arguments"),
		widget.NewLabel("Specify extra flags to pass to the OpenTTD executable:"),
		extraArgsEntry,
	))
	advancedTab := container.NewTabItemWithIcon("Advanced Options", theme.SettingsIcon(), advancedScroll)

	tabs := container.NewAppTabs(generalTab, launchTab, advancedTab)
	tabs.SetTabLocation(container.TabLocationTop)

	form := container.NewBorder(statusLabel, nil, nil, nil, tabs)

	saveBtn = widget.NewButton("Save", func() { saveProfile(false) })
	saveAndRunBtn = widget.NewButton("Save & Run", func() { saveProfile(true) })
	if isNew {
		saveBtn.SetText("Create")
		saveAndRunBtn.SetText("Create & Run")
	}
	cancelBtn := widget.NewButton("Cancel", func() {
		editDialog.Hide()
	})

	updateState = func() {
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
	for _, entry := range []*widget.Entry{nameEntry, savePathEntry, ipPortEntry, serverPassEntry, companyNumEntry, companyPassEntry, configFileEntry, customFolderEntry} {
		entry.OnChanged = func(string) {
			updateState()
		}
	}
	versionEntry.OnChanged = func(string) {
		updateState()
	}
	updateState()

	dialogTitle := "Edit Profile"
	if isNew {
		dialogTitle = "Create Profile"
	}
	editDialog = NewModalDialog(um.Window.Canvas(), dialogTitle, form, cancelBtn, saveBtn, saveAndRunBtn)
	editDialog.Resize(fyne.NewSize(850, 600))
	editDialog.Show()
}
