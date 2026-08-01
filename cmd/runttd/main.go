package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/ncruces/zenity"

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

// defaultConfigPaths resolves the platform-specific default paths. The GOOS branch
// lives here (main.go), the sanctioned exception, so domain stays platform-free.
func defaultConfigPaths() (parentDir, docsBasePath, osDefault string) {
	docsBase := platform.GetDocumentsDir()
	ottdDirName := "OpenTTD"
	clientsSuffix := "-Clients"
	if runtime.GOOS == "linux" {
		ottdDirName = "openttd"
		clientsSuffix = "-clients"
	}
	return filepath.Join(docsBase, ottdDirName+clientsSuffix), filepath.Join(docsBase, ottdDirName), platform.DefaultOSType()
}

// buildDefaultConfig returns a fresh FirstRun config with platform-appropriate paths.
func buildDefaultConfig() *domain.Config {
	return domain.NewDefaultConfig(defaultConfigPaths())
}

// recoverCorruptConfig moves an unreadable config aside to <path>.broken
// (overwriting any earlier one) so it is preserved but can't keep failing.
func recoverCorruptConfig(path string) error {
	broken := path + ".broken"
	os.Remove(broken)
	return os.Rename(path, broken)
}

// fatalStartup reports a startup failure and exits. stderr is nulled on the
// -H=windowsgui build (setupGuiOutput), so a native dialog is the only channel
// that reaches the user there; it's shown alongside stderr for console/dev runs.
func fatalStartup(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	_ = zenity.Error(msg, zenity.Title("RunTTD failed to start"))
	os.Exit(1)
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
				fatalStartup(fmt.Sprintf("Config at %s was unreadable and could not be backed up: %v", configPath, recErr))
			}
			fmt.Fprintf(os.Stderr, "Config at %s was unreadable; backed up to %s.broken and reset to defaults.\n", configPath, configPath)
		}

		// Left unsaved on purpose: onboarding writes the config when it completes, so
		// quitting first-run early leaves no file and setup runs again next launch.
		config = buildDefaultConfig()
		bootstrapFileLog = config.LogToFile
	default:
		fatalStartup(fmt.Sprintf("Startup failed while loading config: %v", err))
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
	ui.Defaults = domain.NewDefaultConfig(defaultConfigPaths())
	if bootstrapFileLog {
		platform.AppendToLogFileRaw(logPath, "Launching UI")
	}
	ui.Show()
	if bootstrapFileLog {
		platform.AppendToLogFileRaw(logPath, "UI exited")
	}
}
