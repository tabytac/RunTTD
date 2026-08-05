package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

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

// Exit codes a script can branch on. Under --wait a run that got as far as
// starting the game returns the game's own code instead, which may reuse these.
const (
	exitOK           = 0
	exitUsage        = 1 // bad flags, a startup failure, or a game with no readable exit code
	exitNoProfile    = 2 // --profile matched no profile, or more than one
	exitProfileSetup = 3 // the profile's own setup (its custom executable folder) is unusable
	exitVersion      = 4 // the version could not be resolved, downloaded, or located
	exitDeclined     = 5 // the launch was declined: the build needs manual setup first
	exitNotStarted   = 6 // the install is present but OpenTTD did not start
)

const usageText = `RunTTD, a portable launcher for OpenTTD and JGR's Patchpack.

Usage:
  RunTTD [flags]

Flags:
  -p, --profile <name>  Launch this profile. The name must match a profile
                        exactly, or uniquely ignoring case.
  --headless, --no-gui  Launch the profile given by --profile without opening
                        the window, then exit.
  --wait                With --headless, stay running until the game exits and
                        return its exit code.
  --version             Print the RunTTD version and exit.
  --help                Print this message and exit.

--profile on its own still opens the window and launches on startup, and takes
precedence over a configured startup profile. Without --wait a headless run exits as soon
as the game has started, matching the way RunTTD detaches it.

Exit codes: 0 launched, 1 bad flags or startup failure, 2 no such profile,
3 profile setup problem, 4 download or version problem, 5 launch declined,
6 OpenTTD did not start.

Under --wait the game's own exit code is returned once it has started, so it can
reuse any of those values. A run that never started always says why on stderr,
which is the reliable way to tell the two apart.

RunTTD is a windowed program, so an interactive prompt does not wait for it:
output can appear after the prompt returns, and the exit code is only readable
through something that waits, such as "start /wait RunTTD.exe ..." from cmd.
`

// cliOptions is the parsed command line.
type cliOptions struct {
	profile     string
	headless    bool
	wait        bool
	showVersion bool
}

// parseArgs reads the command line, rejecting flag combinations that can't mean
// anything. Returns flag.ErrHelp for --help; every other message is the caller's
// to print, so help can go to stdout and errors to stderr.
func parseArgs(args []string) (cliOptions, error) {
	var opts cliOptions
	fs := flag.NewFlagSet("RunTTD", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.StringVar(&opts.profile, "profile", "", "launch the named profile")
	fs.StringVar(&opts.profile, "p", "", "shorthand for -profile")
	fs.BoolVar(&opts.headless, "headless", false, "launch without opening the window")
	fs.BoolVar(&opts.headless, "no-gui", false, "alias for -headless")
	fs.BoolVar(&opts.wait, "wait", false, "wait for the game to exit")
	fs.BoolVar(&opts.showVersion, "version", false, "print the RunTTD version")

	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if rest := fs.Args(); len(rest) > 0 {
		return opts, fmt.Errorf("unexpected argument %q", rest[0])
	}
	opts.profile = strings.TrimSpace(opts.profile)
	if opts.headless && opts.profile == "" {
		return opts, errors.New("--headless needs --profile <name>: there is nothing to launch without one")
	}
	if opts.wait && !opts.headless {
		return opts, errors.New("--wait only applies with --headless: a windowed run stays open anyway")
	}
	return opts, nil
}

// resolveProfileIndex maps a --profile value to its position in profiles: an
// exact name wins, else a unique case-insensitive one. Anything else is an
// error, so a typo can never launch the wrong profile.
func resolveProfileIndex(profiles []domain.Profile, name string) (int, error) {
	var exact, folded []int
	for i, p := range profiles {
		switch candidate := strings.TrimSpace(p.Name); {
		case candidate == name:
			exact = append(exact, i)
		case strings.EqualFold(candidate, name):
			folded = append(folded, i)
		}
	}
	matches := exact
	if len(matches) == 0 {
		matches = folded
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return -1, fmt.Errorf("no profile named %q. Available: %s", name, namesOf(profiles, nil))
	default:
		return -1, fmt.Errorf("%q matches more than one profile: %s", name, namesOf(profiles, matches))
	}
}

// namesOf lists the profile names at the given indices, or all of them when idx is nil.
func namesOf(profiles []domain.Profile, idx []int) string {
	var names []string
	if idx == nil {
		for _, p := range profiles {
			names = append(names, p.Name)
		}
	} else {
		for _, i := range idx {
			names = append(names, profiles[i].Name)
		}
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

// headlessFailure maps a launch result to the exit code and the one-line reason
// a script sees on stderr. A started launch maps to exitOK and no reason.
func headlessFailure(result app.LaunchResult) (int, string) {
	switch result {
	case app.LaunchProfileError:
		return exitProfileSetup, "the profile's custom executable folder is unset or missing"
	case app.LaunchVersionError:
		return exitVersion, "the requested version could not be resolved, downloaded, or located"
	case app.LaunchCancelled:
		return exitDeclined, "this version needs manual setup, so it was not downloaded without confirmation"
	case app.LaunchStartError:
		return exitNotStarted, "OpenTTD did not start"
	case app.LaunchStarted:
		return exitOK, ""
	default:
		// Named explicitly so a new LaunchResult reports a failure rather than
		// inheriting success from a default arm.
		return exitUsage, "the launch ended in an unrecognised state"
	}
}

// consoleObserver is the headless ProcessObserver: everything reaches the
// session log, and stdout too once verbose logging is on.
type consoleObserver struct {
	logger  *platform.Logger
	verbose bool
}

func (o *consoleObserver) LogImportant(msg string) {
	o.logger.Append(msg)
	if o.verbose {
		fmt.Fprintln(os.Stdout, msg)
	}
}

func (o *consoleObserver) LogVerbose(msg string) {
	if !o.verbose {
		return
	}
	o.logger.Append(msg)
	fmt.Fprintln(os.Stdout, msg)
}

func (o *consoleObserver) OnStarted() {}

// runHeadless launches one profile with no window and returns the process exit
// code. Progress goes to stdout and failures to stderr; a headless run shows no
// dialog, since a modal would hang an unattended script.
func runHeadless(cfg *domain.Config, logPath string, profile domain.Profile, wait bool) int {
	logger := platform.NewLogger(cfg.LogToFile, logPath)
	obs := &consoleObserver{logger: logger, verbose: cfg.Verbose}

	var waitForGame func() int
	result := app.LaunchProfile(context.Background(), profile, app.LaunchDeps{
		Config:       cfg,
		Logger:       logger,
		Observer:     obs,
		UpdateStatus: func(status string) { fmt.Fprintln(os.Stdout, status) },
		Confirm: func(message string) bool {
			// Nobody is here to answer, and the build needs data files RunTTD
			// can't supply; install it once from the window to unblock this.
			fmt.Fprintln(os.Stderr, message)
			return false
		},
		OnProcessStarted: func(w func() int) { waitForGame = w },
	})
	if code, reason := headlessFailure(result); reason != "" {
		fmt.Fprintf(os.Stderr, "RunTTD: %s\n", reason)
		return code
	}
	if !wait {
		return exitOK
	}
	if code := waitForGame(); code >= 0 {
		return code
	}
	fmt.Fprintln(os.Stderr, "RunTTD: OpenTTD ended without an exit code")
	return exitUsage
}

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

func brokenConfigPath(path string) string { return path + ".broken" }

// recoverCorruptConfig moves an unreadable config aside to <path>.broken
// (overwriting any earlier one) so it is preserved but can't keep failing.
func recoverCorruptConfig(path string) error {
	broken := brokenConfigPath(path)
	os.Remove(broken)
	return os.Rename(path, broken)
}

// interactive is false for a headless run. Every dialog below is gated on it:
// a modal blocks until dismissed, and an unattended script has nobody to do so.
// Set once from the parsed flags, before anything can raise one.
var interactive = true

// fatalStartup reports a startup failure and exits. stderr is nulled on the
// -H=windowsgui build (setupGuiOutput), so a native dialog is the only channel
// that reaches the user there; it's shown alongside stderr for console/dev runs.
func fatalStartup(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	if interactive {
		_ = zenity.Error(msg, zenity.Title("RunTTD failed to start"))
	}
	os.Exit(exitUsage)
}

// fatalProfile reports a --profile that named nothing usable and exits.
func fatalProfile(msg string, logToFile bool, logPath string) {
	fmt.Fprintln(os.Stderr, "RunTTD: "+msg)
	if logToFile {
		platform.AppendToLogFileRaw(logPath, msg)
	}
	if interactive {
		_ = zenity.Error(msg, zenity.Title("RunTTD could not launch that profile"))
	}
	os.Exit(exitNoProfile)
}

func main() {
	// Arguments mean console use; a plain double-click stays detached.
	if len(os.Args) > 1 {
		attachParentConsole()
	}

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(usageText)
			os.Exit(exitOK)
		}
		fmt.Fprintf(os.Stderr, "RunTTD: %v\n\n%s", err, usageText)
		os.Exit(exitUsage)
	}
	interactive = !opts.headless
	if opts.showVersion {
		fmt.Println("RunTTD " + Version)
		os.Exit(exitOK)
	}
	// A headless run keeps the real stdout/stderr: a console, or a shell redirect
	// from the -H=windowsgui build, is then the only channel its messages have.
	if !opts.headless {
		setupGuiOutput()
	}

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
		// aside to .broken first (preserved for recovery) so it can't keep failing.
		if errors.As(err, &parseErr) {
			broken := brokenConfigPath(configPath)
			if recErr := recoverCorruptConfig(configPath); recErr != nil {
				fatalStartup(fmt.Sprintf("Config at %s was unreadable and could not be backed up: %v", configPath, recErr))
			}
			fmt.Fprintf(os.Stderr, "Config at %s was unreadable; backed up to %s and reset to defaults.\n", configPath, broken)
			detail := fmt.Sprintf("Your RunTTD configuration file could not be read, so it has been reset to defaults and setup will run again.\n\n"+
				"The unreadable file was saved as:\n%s\n\nYou can copy your old settings out of it by hand if you want them back.", broken)
			if interactive {
				_ = zenity.Warning(detail, zenity.Title("RunTTD configuration reset"))
			}
		}

		// Left unsaved on purpose: onboarding writes the config when it completes, so
		// quitting first-run early leaves no file and setup runs again next launch.
		config = buildDefaultConfig()
		bootstrapFileLog = config.LogToFile
	default:
		fatalStartup(fmt.Sprintf("Startup failed while loading config: %v", err))
	}

	// 4. Initialise session logs using platform logger.
	if bootstrapFileLog {
		// Headless appends: a scheduled run would otherwise wipe the log of a
		// launcher window open at the same time.
		if interactive {
			_ = os.WriteFile(logPath, []byte{}, 0644) // Clear old logs from previous sessions
		}
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
			// File logging defaults to off and stderr is nulled on the -H=windowsgui
			// build, so without this a crash would be completely silent to the user.
			detail := fmt.Sprintf("RunTTD ran into an unexpected error and must close.\n\n%v", r)
			if bootstrapFileLog {
				detail += fmt.Sprintf("\n\nDetails were written to %s", logPath)
			}
			if interactive {
				_ = zenity.Error(detail, zenity.Title("RunTTD crashed"))
			}
			os.Exit(exitUsage)
		}
	}()

	launchIdx := -1
	if opts.profile != "" {
		idx, resolveErr := resolveProfileIndex(config.Profiles, opts.profile)
		if resolveErr != nil {
			fatalProfile(resolveErr.Error(), bootstrapFileLog, logPath)
		}
		launchIdx = idx
	}

	// A first-run config is the seeded default, not the user's: launching from it
	// would silently download a client and run with none of their own settings.
	if opts.headless && config.FirstRun {
		fatalProfile("setup has not been completed yet; run RunTTD once with the window to finish it", bootstrapFileLog, logPath)
	}

	if opts.headless {
		os.Exit(runHeadless(config, logPath, config.Profiles[launchIdx], opts.wait))
	}

	ui := fyneuipkg.NewUIManager(config, configPath, Version)
	ui.Defaults = domain.NewDefaultConfig(defaultConfigPaths())
	if launchIdx >= 0 {
		ui.ArmLaunchProfile(config.Profiles[launchIdx].Name, false)
	}
	if bootstrapFileLog {
		platform.AppendToLogFileRaw(logPath, "Launching UI")
	}
	ui.Show()
	if bootstrapFileLog {
		platform.AppendToLogFileRaw(logPath, "UI exited")
	}
}
