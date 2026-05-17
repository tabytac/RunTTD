package fyne

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// OnStarted is the ProcessObserver callback triggered when OpenTTD starts successfully
func (um *UIManager) OnStarted() {
	um.OnOpenTTDStarted()
}

// showToast shows a temporary notification at the bottom of the window
func (um *UIManager) showToast(message string) {
	toast := widget.NewLabel(message)
	toast.Alignment = fyne.TextAlignCenter

	content := container.NewPadded(toast)
	pop := widget.NewPopUp(content, um.Window.Canvas())

	// Position at bottom center
	size := um.Window.Content().Size()
	pop.ShowAtPosition(fyne.NewPos(size.Width/2-100, size.Height-60))

	go func() {
		time.Sleep(3 * time.Second)
		fyne.Do(func() {
			pop.Hide()
		})
	}()
}

// showLogView shows a screen with live logs while launching a profile or in standalone mode
func (um *UIManager) showLogView(profileIdx int) {
	var profile domain.Profile
	isLaunch := profileIdx >= 0
	if isLaunch {
		profile = um.Config.Profiles[profileIdx]
	}

	statusBinding := binding.NewString()
	_ = statusBinding.Set("Preparing launch")

	var summaryObj fyne.CanvasObject
	if isLaunch {
		summary := widget.NewLabel(fmt.Sprintf("Profile: %s\nVersion: %s\nSave path: %s\nServer: %s", profile.Name, valueOrDefault(profile.Version, "latest"), valueOrDefault(profile.SavePath, "(none)"), valueOrDefault(profile.ServerIpPort, "(none)")))
		summary.Wrapping = fyne.TextWrapWord
		summaryObj = widget.NewCard("Launching", "Current launch context", summary)
	}

	statusLabel := widget.NewLabelWithData(statusBinding)
	statusLabel.Wrapping = fyne.TextWrapWord
	statusObj := widget.NewCard("Status", "Background operations", statusLabel)
	if !isLaunch {
		statusObj.Hide()
	}

	// Create log text widget using data binding (thread-safe)
	logBinding := binding.NewString()
	logLabel := widget.NewLabelWithData(logBinding)
	logLabel.Wrapping = fyne.TextWrapWord

	logBox := container.NewVScroll(logLabel)
	logBox.SetMinSize(fyne.NewSize(600, 400))

	// Update the log display whenever logger changes
	updateLogDisplay := func() {
		logs := um.Logger.GetAll()
		text := ""
		for _, line := range logs {
			text += line + "\n"
		}
		fyne.Do(func() {
			_ = logBinding.Set(text)
			logBox.ScrollToBottom()
		})
	}

	// Start a goroutine to periodically update the log display
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				updateLogDisplay()
			case <-done:
				return
			}
		}
	}()

	closeBtn := widget.NewButton("Return to Main", func() {
		select {
		case <-done:
		default:
			close(done)
		}
		um.Window.SetContent(um.makeMainView())
	})

	copyBtn := widget.NewButtonWithIcon("Copy to Clipboard", theme.ContentCopyIcon(), func() {
		logs := um.Logger.GetAll()
		text := ""
		for _, line := range logs {
			text += line + "\n"
		}
		um.App.Clipboard().SetContent(text)
		um.showToast("Logs copied to clipboard!")
	})

	top := container.NewVBox()
	if isLaunch {
		top.Add(summaryObj)
		top.Add(statusObj)
	}

	content := container.NewBorder(
		top,
		container.NewHBox(closeBtn, copyBtn),
		nil,
		nil,
		logBox,
	)

	um.Window.SetContent(content)

	// Launch OpenTTD in background if requested
	if isLaunch {
		go um.launchProfile(profile, func(status string) {
			_ = statusBinding.Set(status)
		}, nil)
	}
}

// launchProfile launches OpenTTD with the specified profile configuration and logging observers
func (um *UIManager) launchProfile(profile domain.Profile, updateStatus func(status string), onError func()) {
	if updateStatus != nil {
		updateStatus("Resolving profile and version")
	}
	um.LogImportant(fmt.Sprintf("Launching profile %q", profile.Name))
	um.LogVerbose(fmt.Sprintf("Profile config: version=%q savePath=%q server=%q company=%q", profile.Version, profile.SavePath, profile.ServerIpPort, profile.ServerCompanyNumber))

	requested := strings.TrimSpace(profile.Version)
	version := requested
	requestedLower := strings.ToLower(requested)
	client := profile.Client
	if strings.TrimSpace(client) == "" {
		client = um.Config.DefaultClient
		if client == "" {
			client = "jgrpp"
		}
	}

	isLatestRequest := false
	latestTrack := "stable"
	switch requestedLower {
	case "", "latest", "latest-stable", "latest (stable)":
		isLatestRequest = true
		latestTrack = "stable"
	case "latest-testing", "latest (testing)":
		isLatestRequest = true
		latestTrack = "testing"
	}
	if client == "vanilla-nightly" && isLatestRequest {
		latestTrack = "testing"
	}

	if isLatestRequest {
		if updateStatus != nil {
			updateStatus(fmt.Sprintf("Resolving latest %s version (%s)", client, latestTrack))
		}
		um.LogImportant(fmt.Sprintf("Resolving latest %s version (%s)", client, latestTrack))
		version = platform.CheckForNewVersionForClientTrack(context.Background(), client, um.Config, latestTrack)
		if version == "" {
			um.LogImportant("Could not determine latest version from remote; trying latest local install.")
			if updateStatus != nil {
				updateStatus("Latest version lookup failed, using latest local install")
			}
			versionFolder := platform.FindLatestFolderClientWithConfig(um.Config.ParentDir, client, um.Config)
			if versionFolder == "" {
				if updateStatus != nil {
					updateStatus("Failed: no local installation found for client")
				}
				um.LogImportant("No local installation found for client.")
				if onError != nil {
					onError()
				}
				return
			}
			um.LogVerbose(fmt.Sprintf("Using latest local version folder: %s", versionFolder))
			if updateStatus != nil {
				updateStatus("Starting OpenTTD from latest local installation")
			}
			platform.ExecuteOpenTTD(context.Background(), versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, profile.LaunchMode, profile.ExtraArgs, profile.AutoLatestFilter, um.Config.DocsBasePath, um)
			if updateStatus != nil {
				updateStatus("Launch command sent")
			}
			return
		}
	}
	if requested != "" && !isLatestRequest {
		if updateStatus != nil {
			updateStatus(fmt.Sprintf("Using requested %s version %s", client, version))
		}
		um.LogImportant(fmt.Sprintf("Using requested %s version %s", client, version))
	}

	if updateStatus != nil {
		updateStatus("Looking for local version folder")
	}
	versionFolder := platform.FindVersionFolderClient(um.Config.ParentDir, version, client, um.Config)
	if versionFolder == "" {
		if updateStatus != nil {
			updateStatus("Version not found locally, downloading")
		}
		um.LogImportant(fmt.Sprintf("Version %s not found locally. Attempting to download for client %s.", version, client))
		if !platform.DownloadAndExtractVersionForClientWithLogger(context.Background(), version, client, um.Config, um.Logger) {
			if updateStatus != nil {
				updateStatus(fmt.Sprintf("Failed: download of version %s did not complete", version))
			}
			um.LogImportant(fmt.Sprintf("Failed to download version %s for client %s.", version, client))
			return
		}
		if updateStatus != nil {
			updateStatus("Download complete, resolving extracted folder")
		}
		versionFolder = platform.FindVersionFolderClient(um.Config.ParentDir, version, client, um.Config)
		if versionFolder == "" {
			if updateStatus != nil {
				updateStatus("Failed: downloaded version folder could not be located")
			}
			um.LogImportant("Failed to locate downloaded version.")
			return
		}
	}

	um.LogVerbose(fmt.Sprintf("Using version folder: %s", versionFolder))
	if updateStatus != nil {
		updateStatus("Starting OpenTTD")
	}
	platform.ExecuteOpenTTD(context.Background(), versionFolder, profile.ServerIpPort, profile.ServerCompanyNumber, profile.ServerPassword, profile.ServerCompanyPassword, profile.SavePath, profile.LaunchMode, profile.ExtraArgs, profile.AutoLatestFilter, um.Config.DocsBasePath, um)
	if updateStatus != nil {
		updateStatus("Launch command sent")
	}
}
