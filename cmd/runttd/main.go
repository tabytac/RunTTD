package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"runttd/internal/app"
	"runttd/internal/domain"
	"runttd/internal/platform"
	fyneuipkg "runttd/internal/ui/fyne"
)

// Version is the RunTTD release version, set at build time via
// -ldflags "-X main.Version=<tag>". Defaults to "dev" for local builds,
// which suppresses the update indicator.
var Version = "dev"

func setupGuiOutput() {
	if runtime.GOOS == "windows" {
		f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err == nil {
			os.Stdout = f
			os.Stderr = f
		}
	}
}

// buildDefaultConfig returns a fresh FirstRun config with platform-appropriate
// OpenTTD directory names. Used for both a missing and a recovered-corrupt config.
func buildDefaultConfig() *domain.Config {
	docsBase := platform.GetDocumentsDir()
	ottdDirName := "OpenTTD"
	clientsSuffix := "-Clients"
	if runtime.GOOS == "linux" {
		ottdDirName = "openttd"
		clientsSuffix = "-clients"
	}
	return &domain.Config{
		FirstRun:           true,
		ParentDir:          filepath.Join(docsBase, ottdDirName+clientsSuffix),
		DocsBasePath:       filepath.Join(docsBase, ottdDirName),
		JgrppApiUrl:        domain.DefaultJgrppApiUrl,
		OSType:             platform.DefaultOSType(),
		SubfolderPerClient: true,
		VanillaMirror:      domain.DefaultVanillaMirror,
		NightlyMirror:      domain.DefaultNightlyMirror,
		Profiles:           []domain.Profile{{Name: "Default", Version: "latest"}},
	}
}

// recoverCorruptConfig moves an unreadable config aside to <path>.broken
// (overwriting any earlier one) so it is preserved but can't keep failing.
func recoverCorruptConfig(path string) error {
	broken := path + ".broken"
	os.Remove(broken)
	return os.Rename(path, broken)
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

	var parseErr *domain.ConfigParseError
	switch {
	case err == nil:
		bootstrapFileLog = config.LogToFile
	case errors.Is(err, os.ErrNotExist), errors.As(err, &parseErr):
		// Missing OR corrupt config -> start from defaults. A corrupt file is moved
		// aside to .broken first (preserved for recovery) so it can't keep failing;
		// FirstRun then runs onboarding, signalling the reset without a dialog.
		if errors.As(err, &parseErr) {
			if recErr := recoverCorruptConfig(configPath); recErr != nil {
				fmt.Fprintf(os.Stderr, "Config at %s was unreadable and could not be backed up: %v\n", configPath, recErr)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Config at %s was unreadable; backed up to %s.broken and reset to defaults.\n", configPath, configPath)
		}

		config = buildDefaultConfig()
		if saveErr := domain.SaveConfig(configPath, config); saveErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to create default config at %s: %v\n", configPath, saveErr)
			os.Exit(1)
		}
		bootstrapFileLog = config.LogToFile
	default:
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

	ui := fyneuipkg.NewUIManager(config, configPath, Version)
	if bootstrapFileLog {
		platform.AppendToLogFileRaw(logPath, "Launching UI")
	}
	ui.Show()
	if bootstrapFileLog {
		platform.AppendToLogFileRaw(logPath, "UI exited")
	}
}
