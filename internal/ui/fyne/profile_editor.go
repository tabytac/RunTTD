package fyne

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

// carryForwardAutoLaunch updates the stored auto-launch name when its profile is renamed.
func carryForwardAutoLaunch(oldName, newName, stored string) string {
	if stored != "" && stored == oldName {
		return newName
	}
	return stored
}

// showProfileEditor displays the edit modal popup for creating or updating profiles
func (um *UIManager) showProfileEditor(profileIdx int, isNew bool) {
	var profile domain.Profile
	if !isNew {
		profile = um.Config.Profiles[profileIdx]
	}

	// cancelOrConfirm/commitSave are defined later (they need widgets built below), but the
	// dialog* wrappers constructed from here on need something to forward Escape/Enter to
	// right away; onEscape/onEnter close over the vars so they resolve once assigned.
	var cancelOrConfirm func()
	var commitSave func()
	onEscape := func() {
		if cancelOrConfirm != nil {
			cancelOrConfirm()
		}
	}
	onEnter := func() {
		if commitSave != nil {
			commitSave()
		}
	}

	// Form fields
	nameEntry := newDialogEntry(onEscape, onEnter)
	nameEntry.SetText(profile.Name)
	nameEntry.SetPlaceHolder("Profile name")

	const versionEntryPlaceholder = "latest (Stable), latest (Testing), or 15.3"
	const versionFetchFailedPlaceholder = "Version list unavailable; type a version"
	const versionFetchFailedHint = "Could not load the version list. Check your connection, or type a version directly."
	versionEntry := newDialogSelectEntry(um.CachedVersions, onEscape, onEnter)
	versionEntry.SetOptions([]string{"latest (Stable)", "latest (Testing)"})
	versionEntry.PlaceHolder = versionEntryPlaceholder

	customFolderEntry := newDialogEntry(onEscape, onEnter)
	customFolderEntry.SetText(profile.CustomExecutablePath)
	customFolderEntry.SetPlaceHolder("Folder containing openttd executable")

	var customFolderBtn *dialogButton
	customFolderBtn = newDialogButton("Browse…", func() {
		customFolderBtn.Disable()
		um.browseDirectory(&customFolderEntry.Entry, "Select Custom Executable Folder", "Custom Executable Folder", customFolderBtn.Enable)
	}, onEscape)
	customFolderRow := container.NewBorder(nil, nil, nil, customFolderBtn, customFolderEntry)

	// Text set per client by applyVersionHint; a failed fetch replaces it until the client changes.
	versionTrackHint := NewSectionDescription("")
	versionTrackHint.Hide()

	versionGroup := container.NewVBox(widget.NewLabel("Version"), versionEntry, versionTrackHint)
	customFolderGroup := container.NewVBox(widget.NewLabel("Executable Folder"), customFolderRow)

	applyVersionHint := func(clientID string) {
		if hint := versionTrackHintText(clientID); hint != "" {
			versionTrackHint.SetText(hint)
			versionTrackHint.Show()
		} else {
			versionTrackHint.Hide()
		}
	}

	updateClientFields := func(clientID string) {
		if clientID == "custom" {
			versionGroup.Hide()
			customFolderGroup.Show()
		} else {
			versionGroup.Show()
			customFolderGroup.Hide()
		}
		applyVersionHint(clientID)
	}

	// Client selection (Vanilla OpenTTD Releases/Nightly, JGRPP, Custom Executable).
	// versionFetchGuard drops a slow fetch's result if a newer one has been started
	// since (e.g. the user switches client A -> B -> A quickly), so it can't
	// overwrite the newer client's already-applied options with a stale list.
	var versionFetchGuard debounceGuard
	fetchVersionsForClient := func(clientID string) {
		gen := versionFetchGuard.next()
		versionEntry.PlaceHolder = "Loading versions…"
		versionEntry.Refresh()
		um.startAsync(func() {
			versions, err := app.ClientFetchVersions(context.Background(), clientID, um.Config)
			fyne.Do(func() {
				if !versionFetchGuard.current(gen) {
					return // superseded by a newer fetch
				}
				if err != nil {
					um.Logger.Append(fmt.Sprintf("Failed to fetch versions for %s: %v", clientID, err))
					// The placeholder only paints on an empty field, so the hint carries this.
					versionEntry.PlaceHolder = versionFetchFailedPlaceholder
					versionTrackHint.SetText(versionFetchFailedHint)
					versionTrackHint.Show()
				} else {
					options := versionOptionsFor(clientID, versions)
					um.CachedVersions = options
					versionEntry.SetOptions(options)
					versionEntry.PlaceHolder = versionEntryPlaceholder
					applyVersionHint(clientID)
				}
				versionEntry.Refresh()
			})
		})
	}
	// updateState is defined later, but the client radio callback may need to call it.
	var updateState func()
	var setCompanyPasswordVisible func(visible bool)
	// hideEditor is defined later (it needs editDialog), but saveProfile below must call it.
	var hideEditor func()
	// refreshDirty is defined later (it needs editDialog's title label); guarded nil checks
	// let the radios below reference it before then, matching updateState's own pattern.
	var refreshDirty func()
	initializingClientSelection := true
	clientSelect := NewSegmentedRadio(defaultClientOptions, "", func(s string) {
		cli := defaultClientMap[s]
		updateClientFields(cli)
		setCompanyPasswordVisible(app.ClientSupportsCompanyPassword(companyPasswordClientID(cli)))
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
	}, onEscape)
	// Preselect client and version display for new and existing profiles.
	currentClient := app.EffectiveClient(profile.Client, um.Config.DefaultClient)
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
		if refreshDirty != nil {
			refreshDirty()
		}
	}, onEscape)

	// Multiplayer fields
	ipPortEntry := newDialogEntry(onEscape, onEnter)
	ipPortEntry.SetText(profile.ServerIpPort)
	ipPortEntry.SetPlaceHolder("host:port")

	serverPassEntry := newDialogEntry(onEscape, onEnter)
	serverPassEntry.Password = true
	serverPassEntry.SetText(profile.ServerPassword)
	serverPassEntry.SetPlaceHolder("Optional password")

	companyPassEntry := newDialogEntry(onEscape, onEnter)
	companyPassEntry.Password = true
	companyPassEntry.SetText(profile.ServerCompanyPassword)
	companyPassEntry.SetPlaceHolder("Optional company password")

	companyNumEntry := newDialogEntry(onEscape, onEnter)
	companyNumEntry.SetText(profile.ServerCompanyNumber)
	companyNumEntry.SetPlaceHolder("Optional company number")

	// Save fields
	savePathEntry := newDialogEntry(onEscape, onEnter)
	savePathEntry.SetText(profile.SavePath)
	savePathEntry.SetPlaceHolder("Path to file or folder")

	extraArgsEntry := newDialogMultiLineEntry(onEscape)
	extraArgsEntry.SetText(profile.ExtraArgs)
	extraArgsEntry.SetPlaceHolder("Example: -d 3 -r 1920x1080")
	extraArgsEntry.Wrapping = fyne.TextWrapWord

	configFileEntry := newDialogEntry(onEscape, onEnter)
	configFileEntry.SetText(profile.ConfigFilePath)
	configFileEntry.SetPlaceHolder("Optional path to config file (openttd.cfg)")

	noConfigSaveCheck := newDialogCheck("Do not save config changes on exit", func(bool) {
		if refreshDirty != nil {
			refreshDirty()
		}
	}, onEscape, onEnter)
	noConfigSaveCheck.SetChecked(profile.NoConfigSave)

	// Checked means "apply", matching the entry's plain-English intent; the stored
	// field is inverted (ExtraArgsDisabled) so an old profile with the field absent
	// still defaults to applied, not silently dropped.
	applyExtraArgsCheck := newDialogCheck("Apply custom arguments at launch", func(bool) {
		if refreshDirty != nil {
			refreshDirty()
		}
	}, onEscape, onEnter)
	applyExtraArgsCheck.SetChecked(!profile.ExtraArgsDisabled)

	// browseConfigBtn is declared via var/assign (not :=) so its own onTapped
	// closure below can reference it to disable/re-enable itself around the picker.
	var browseConfigBtn *dialogButton
	browseConfigBtn = newDialogButton("Browse…", func() {
		browseConfigBtn.Disable() // stops a second picker stacking on top while this one is open
		// Widget and config reads stay on the UI thread; both can change mid-pick.
		docsBase := um.Config.DocsBasePath
		startPath := strings.TrimSpace(configFileEntry.Text)
		if startPath == "" && docsBase != "" {
			startPath = filepath.Join(docsBase, "openttd.cfg")
		} else if startPath != "" && !filepath.IsAbs(startPath) && docsBase != "" {
			startPath = filepath.Join(docsBase, startPath)
		}
		go func() {
			defer fyne.Do(browseConfigBtn.Enable)
			defer func() {
				if r := recover(); r != nil {
					um.Logger.Append(fmt.Sprintf("CRITICAL: config picker panicked: %v", r))
				}
			}()
			path, err := um.browseConfigPath(docsBase, startPath)
			if err != nil {
				fyne.Do(func() { um.showErrorf("could not open config picker: %w", err) })
				return
			}
			if path == "" {
				return
			}
			fyne.Do(func() { configFileEntry.SetText(path) })
		}()
	}, onEscape)
	browseConfigBtn.Icon = theme.FileIcon()

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
		if refreshDirty != nil { // selection itself is read on save
			refreshDirty()
		}
	}, onEscape)

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
		if refreshDirty != nil {
			refreshDirty()
		}
	}, onEscape)
	updateFolderInstructions(autoLatestFilterRadio.Selected)

	includeSubfoldersCheck := newDialogCheck("Include subfolders", func(bool) {
		if refreshDirty != nil {
			refreshDirty()
		}
	}, onEscape, onEnter)
	includeSubfoldersCheck.SetChecked(profile.SaveSearchSubfolders)

	var browseFileBtn *dialogButton
	browseFileBtn = newDialogButton("Browse file…", func() {
		browseFileBtn.Disable()
		docsBase, startPath := um.Config.DocsBasePath, savePathEntry.Text
		if startPath == "" {
			if docsBase != "" {
				startPath = filepath.Join(docsBase, "save")
			}
		} else if !filepath.IsAbs(startPath) && docsBase != "" {
			startPath = filepath.Join(docsBase, "save", startPath)
		}
		go func() {
			defer fyne.Do(browseFileBtn.Enable)
			defer func() {
				if r := recover(); r != nil {
					um.Logger.Append(fmt.Sprintf("CRITICAL: file picker panicked: %v", r))
				}
			}()
			path, err := um.browseSavePath(docsBase, startPath, "Select Save or Scenario", false)
			if err != nil {
				fyne.Do(func() { um.showErrorf("could not open file picker: %w", err) })
				return
			}
			if path == "" {
				return
			}
			fyne.Do(func() { savePathEntry.SetText(path) })
		}()
	}, onEscape)
	browseFileBtn.Icon = theme.FileIcon()

	var browseFolderBtn *dialogButton
	browseFolderBtn = newDialogButton("Browse folder…", func() {
		browseFolderBtn.Disable()
		docsBase, startPath := um.Config.DocsBasePath, savePathEntry.Text
		if startPath == "" {
			if docsBase != "" {
				startPath = filepath.Join(docsBase, "save")
			}
		} else if !filepath.IsAbs(startPath) && docsBase != "" {
			startPath = filepath.Join(docsBase, "save", startPath)
		}
		go func() {
			defer fyne.Do(browseFolderBtn.Enable)
			defer func() {
				if r := recover(); r != nil {
					um.Logger.Append(fmt.Sprintf("CRITICAL: folder picker panicked: %v", r))
				}
			}()
			path, err := um.browseSavePath(docsBase, startPath, "Select Save Folder", true)
			if err != nil {
				fyne.Do(func() { um.showErrorf("could not open folder picker: %w", err) })
				return
			}
			if path == "" {
				return
			}
			fyne.Do(func() { savePathEntry.SetText(path) })
		}()
	}, onEscape)
	browseFolderBtn.Icon = theme.FolderIcon()

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
		includeSubfoldersCheck,
	}
	pathManualOption.Objects = []fyne.CanvasObject{
		widget.NewSeparator(),
		widget.NewLabel("Path (Relative or Absolute)"),
		savePathEntry,
	}
	companyNumGroup := container.NewVBox(widget.NewLabel("Company Number"), companyNumEntry)
	companyPassGroup := container.NewVBox(widget.NewLabel("Company Password"), companyPassEntry)
	companyDetailsRow := container.NewStack()

	// Rebuild as 1 column when -P is unsupported; hiding alone strands Company Number in a half-width grid.
	setCompanyPasswordVisible = func(visible bool) {
		companyDetailsRow.Objects = nil
		if visible {
			companyDetailsRow.Add(container.NewGridWithColumns(2, companyNumGroup, companyPassGroup))
		} else {
			companyDetailsRow.Add(container.NewGridWithColumns(1, companyNumGroup))
		}
		companyDetailsRow.Refresh()
	}
	// Set initial visibility now that the helper is assigned (companyNumEntry/
	// companyPassEntry are constructed above, so this can't run any earlier).
	setCompanyPasswordVisible(app.ClientSupportsCompanyPassword(companyPasswordClientID(currentClient)))

	multiplayerOption.Objects = []fyne.CanvasObject{
		NewSectionTitle("Server Connection"),
		widget.NewLabel("Server IP:Port"), ipPortEntry,
		widget.NewLabel("Server Password"), serverPassEntry,
		widget.NewSeparator(),
		NewSectionTitle("Company Details"),
		companyDetailsRow,
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

	// pathCheckTimer debounces the two disk-touching warnings below: validate()'s
	// own checks are pure and show instantly, but a stat on an unreachable network
	// share must not run on every keystroke (cancel-and-reset, so a burst of
	// keystrokes triggers exactly one pair of stats, 300ms after typing pauses).
	// pathCheckGuard additionally covers Timer.Stop()'s documented gap (it can't
	// cancel a callback that already started): a slow, superseded check is
	// dropped by generation rather than allowed to overwrite a newer result,
	// including validate()'s own instant message, which a stale check could
	// otherwise clobber after the fact.
	var pathCheckTimer *time.Timer
	var pathCheckGuard debounceGuard
	setStatus := func() {
		if pathCheckTimer != nil {
			pathCheckTimer.Stop()
		}
		ok, message := validate()
		if !ok {
			pathCheckGuard.next() // supersede any in-flight check; this message wins
			statusLabel.SetText(message)
			statusLabel.Refresh()
			return
		}

		// Capture widget values now (UI thread); the timer callback below must not
		// touch Fyne widgets off-thread, only these plain strings and the disk.
		configPath := configFileEntry.Text
		docsBase := um.Config.DocsBasePath
		// Save path is only used in file/folder mode; resolve it the way launch
		// does (relative to docs/save, after stripping a leading save/ prefix).
		mode := modeMap[modeSelect.Selected]
		checkSave := mode == "file" || mode == "folder"
		rawSave := trimSavePrefix(strings.TrimSpace(savePathEntry.Text))
		saveBase := filepath.Join(docsBase, "save")

		gen := pathCheckGuard.next()
		pathCheckTimer = um.startDebounce(300*time.Millisecond, func() {
			warn := pathExistsWarning("config file", configPath, docsBase)
			if warn == "" && checkSave {
				warn = pathExistsWarning("save path", rawSave, saveBase)
			}
			fyne.Do(func() {
				if !pathCheckGuard.current(gen) {
					return // superseded by a newer check
				}
				statusLabel.SetText(warn)
				statusLabel.Refresh()
			})
		})
	}

	var saveBtn *dialogButton
	var saveAndRunBtn *dialogButton

	saveProfile := func(runAfter bool) {
		if ok, message := validate(); !ok {
			statusLabel.SetText(message)
			statusLabel.Refresh()
			return
		}

		oldProfileName := profile.Name
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
		profile.SaveSearchSubfolders = includeSubfoldersCheck.Checked
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
		profile.ExtraArgsDisabled = !applyExtraArgsCheck.Checked

		switch profile.LaunchMode {
		case "":
			profile.SavePath = ""
			profile.ServerIpPort = ""
		case "multiplayer":
			profile.SavePath = ""
		default:
			profile.ServerIpPort = ""
		}

		// Snapshot what save is about to overwrite, so a failed write can be fully
		// undone: leaving any of this applied would let a later, unrelated save
		// (drag-reorder, an accent click) silently persist an edit the user discarded.
		var prevProfile domain.Profile
		if !isNew {
			prevProfile = um.Config.Profiles[profileIdx]
		}
		prevAutoLaunch := um.Config.AutoLaunchProfile
		prevSelected := um.SelectedProfileName

		savedIdx := profileIdx
		if isNew {
			um.Config.Profiles = append(um.Config.Profiles, profile)
			savedIdx = len(um.Config.Profiles) - 1
		} else {
			um.Config.Profiles[profileIdx] = profile
		}
		um.Config.AutoLaunchProfile = carryForwardAutoLaunch(oldProfileName, profile.Name, um.Config.AutoLaunchProfile)
		um.SelectedProfileName = profile.Name

		if !um.saveConfigOrWarn() {
			if isNew {
				um.Config.Profiles = um.Config.Profiles[:savedIdx]
			} else {
				um.Config.Profiles[profileIdx] = prevProfile
			}
			um.Config.AutoLaunchProfile = prevAutoLaunch
			um.SelectedProfileName = prevSelected
			return
		}
		hideEditor()
		if runAfter {
			// Launch via the rebuilt main view so it honors AutoOpenLog and shows
			// the launch band, rather than always forcing the log view open.
			um.suppressAutoCloseOnce = false // Save & Run is a manual launch; don't inherit a stale startup-suppression.
			um.pendingLaunchIdx = savedIdx
		}
		um.Window.SetContent(um.makeMainView())
	}
	commitSave = func() { saveProfile(false) }

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
		NewSectionTitle("OpenTTD Config Behaviour"),
		widget.NewLabel("Config File Override (optional)"),
		container.NewBorder(nil, nil, nil, browseConfigBtn, configFileEntry),
		noConfigSaveCheck,
		widget.NewSeparator(),
		NewSectionTitle("NewGRF Scan Behaviour"),
		widget.NewLabel("Control NewGRF scanning/loading on startup:"),
		container.NewHBox(newgrfRadio.Container),
		widget.NewSeparator(),
		NewSectionTitle("Custom Command Line Arguments"),
		applyExtraArgsCheck,
		widget.NewLabel("Specify extra flags to pass to the OpenTTD executable:"),
		extraArgsEntry,
	))
	advancedTab := container.NewTabItemWithIcon("Advanced Options", theme.SettingsIcon(), advancedScroll)

	tabs := container.NewAppTabs(generalTab, launchTab, advancedTab)
	tabs.SetTabLocation(container.TabLocationTop)

	form := container.NewBorder(statusLabel, nil, nil, nil, tabs)

	saveBtn = newDialogButton("Save", func() { saveProfile(false) }, onEscape)
	saveAndRunBtn = newDialogButton("Save & run", func() { saveProfile(true) }, onEscape)
	if isNew {
		saveBtn.SetText("Create")
		saveAndRunBtn.SetText("Create & run")
	}

	// Dirty-state compares live widget values to a baseline snapshot, the same
	// approach the settings dialog uses, so Cancel/Escape can't silently discard edits.
	type profileSnapshot struct {
		name, client, version, customFolder, mode                 string
		ipPort, serverPass, companyNum, companyPass               string
		savePath, extraArgs, configFile, newgrf, autoLatestFilter string
		noConfigSave, applyExtraArgs, includeSubfolders           bool
	}
	current := func() profileSnapshot {
		return profileSnapshot{
			name: nameEntry.Text, client: clientSelect.Selected, version: versionEntry.Text,
			customFolder: customFolderEntry.Text, mode: modeSelect.Selected,
			ipPort: ipPortEntry.Text, serverPass: serverPassEntry.Text,
			companyNum: companyNumEntry.Text, companyPass: companyPassEntry.Text,
			savePath: savePathEntry.Text, extraArgs: extraArgsEntry.Text, configFile: configFileEntry.Text,
			newgrf: newgrfRadio.Selected, autoLatestFilter: autoLatestFilterRadio.Selected,
			noConfigSave: noConfigSaveCheck.Checked, applyExtraArgs: applyExtraArgsCheck.Checked,
			includeSubfolders: includeSubfoldersCheck.Checked,
		}
	}
	baseline := current()
	isDirty := func() bool { return current() != baseline }

	hideEditor = func() {
		// The pending path check goes with the dialog: its result would render
		// into widgets that no longer exist on screen.
		if pathCheckTimer != nil {
			pathCheckTimer.Stop()
		}
		pathCheckGuard.next()
		um.editorOverlay = nil
		um.editorOnEscape = nil
		editDialog.Hide()
	}
	cancelOrConfirm = func() {
		if !isDirty() {
			hideEditor()
			return
		}
		um.newConfirmDialog("Discard changes?", "Discard", "Keep editing",
			"You have unsaved changes to this profile.",
			func(discard bool) {
				if discard {
					hideEditor()
				}
			}).Show()
	}
	cancelBtn := newDialogButton("Cancel", cancelOrConfirm, onEscape)

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
		if refreshDirty != nil {
			refreshDirty()
		}
	}
	for _, entry := range []*dialogEntry{nameEntry, savePathEntry, ipPortEntry, serverPassEntry, companyNumEntry, companyPassEntry, configFileEntry, customFolderEntry} {
		entry.OnChanged = func(string) {
			updateState()
		}
	}
	extraArgsEntry.OnChanged = func(string) {
		updateState()
	}
	versionEntry.OnChanged = func(string) {
		updateState()
	}
	updateState()

	dialogTitle := "Edit profile"
	if isNew {
		dialogTitle = "Create profile"
	}
	editDialog = NewModalDialog(um.Window.Canvas(), dialogTitle, form, cancelBtn, saveBtn, saveAndRunBtn)
	editDialog.Resize(fyne.NewSize(850, 600))

	// Scope the Escape handler to this overlay by identity; cleared on hide via hideEditor.
	um.editorOverlay = editDialog
	um.editorOnEscape = cancelOrConfirm

	titleLabel := findTitleLabel(editDialog.Content, dialogTitle)
	refreshDirty = func() {
		dirty := isDirty()
		if dirty {
			saveBtn.Importance = widget.HighImportance
			saveAndRunBtn.Importance = widget.HighImportance
		} else {
			saveBtn.Importance = widget.MediumImportance
			saveAndRunBtn.Importance = widget.MediumImportance
		}
		saveBtn.Refresh()
		saveAndRunBtn.Refresh()
		if titleLabel != nil {
			if dirty {
				titleLabel.SetText(dialogTitle + " *")
			} else {
				titleLabel.SetText(dialogTitle)
			}
		}
	}
	refreshDirty()

	editDialog.Show()
	um.Window.Canvas().Focus(nameEntry)
}
