package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"openttd-launcher/internal/app"
	"openttd-launcher/internal/domain"
	"openttd-launcher/internal/platform"
	fyneuipkg "openttd-launcher/internal/ui/fyne"
)

func setupGuiOutput() {
	if runtime.GOOS == "windows" {
		f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err == nil {
			os.Stdout = f
			os.Stderr = f
		}
	}
}

func main() {
	setupGuiOutput()

	// 1. Register all built-in client engine tracks (JGRPP, Stable, Nightly) explicitly.
	app.RegisterBuiltInClients()

	// 2. Resolve configuration paths from platform filesystem adapter.
	configPath := platform.ResolveConfigPath()
	logPath := platform.ResolveLogPath(configPath)

	// 3. Pre-load configuration first to determine if logging to file is enabled.
	config, err := domain.LoadConfig(configPath)
	bootstrapFileLog := false

	if err == nil {
		bootstrapFileLog = config.LogToFile
	} else if errors.Is(err, os.ErrNotExist) {
		// Default config values if config.json does not exist.
		docsBase := platform.GetDocumentsDir()
		ottdDirName := "OpenTTD"
		if runtime.GOOS == "linux" {
			ottdDirName = "openttd"
		}
		defaultDocsDir := filepath.Join(docsBase, ottdDirName)
		defaultParentDir := filepath.Join(docsBase, ottdDirName+"-JGRPP")

		config = &domain.Config{
			FirstRun:         true,
			ParentDir:        defaultParentDir,
			DocsBasePath:     defaultDocsDir,
			GithubApiUrl:     "https://api.github.com/repos/JGRennison/OpenTTD-patches",
			OSType:           platform.DefaultOSType(),
			AutoCloseOnStart: false,
			Verbose:          false,
			LogToFile:        false,
			DefaultClient:    "",
			VanillaMirror:    "https://cdn.openttd.org/openttd-releases/",
			NightlyMirror:    "https://cdn.openttd.org/openttd-nightlies/",
			Profiles:         []domain.Profile{{Name: "Default", Version: "latest"}},
		}

		if saveErr := domain.SaveConfig(configPath, config); saveErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to create default config at %s: %v\n", configPath, saveErr)
			os.Exit(1)
		}
		bootstrapFileLog = config.LogToFile
	} else {
		fmt.Fprintf(os.Stderr, "Startup failed while loading config: %v\n", err)
		os.Exit(1)
	}

	// 4. Initialize session logs using platform logger.
	if bootstrapFileLog {
		_ = os.WriteFile(logPath, []byte{}, 0644) // Clear old logs from previous sessions
		platform.AppendToLogFileRaw(logPath, fmt.Sprintf("Launcher process starting (config: %s)", configPath))
		platform.AppendToLogFileRaw(logPath, "Config loaded successfully")
	}

	defer func() {
		if r := recover(); r != nil {
			message := fmt.Sprintf("panic: %v\n%s", r, string(debug.Stack()))
			if bootstrapFileLog {
				platform.AppendToLogFileRaw(logPath, message)
			}
			fmt.Fprintln(os.Stderr, message)
			os.Exit(1)
		}
	}()

	ui := fyneuipkg.NewUIManager(config, configPath)
	if bootstrapFileLog {
		platform.AppendToLogFileRaw(logPath, "Launching UI")
	}
	ui.Show()
	if bootstrapFileLog {
		platform.AppendToLogFileRaw(logPath, "UI exited")
	}
}
