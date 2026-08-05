package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// LaunchResult reports how a launch ended so a caller can map it to an exit code
// or a UI state; the readable detail has already gone to UpdateStatus and the log.
type LaunchResult int

const (
	LaunchStarted      LaunchResult = iota
	LaunchCancelled                 // ctx was cancelled, or the confirm gate declined
	LaunchProfileError              // the profile's own setup (its custom folder) is unusable
	LaunchVersionError              // the version could not be resolved, downloaded, or located
	LaunchStartError                // the install is present but the game did not start
)

// LaunchDeps carries everything the launch pipeline needs from its caller, so
// the pipeline itself stays free of any UI. Observer must not be nil; every
// callback may be, and is then skipped.
type LaunchDeps struct {
	Config   *domain.Config
	Logger   *platform.Logger         // the download helper's log sink
	Observer platform.ProcessObserver // pipeline log lines plus the game's lifecycle callbacks

	UpdateStatus func(status string)
	OnProgress   platform.ProgressFunc
	OnError      func()

	// InvalidateCaches is called once a download has landed a new folder on disk.
	InvalidateCaches func()

	// Confirm gates the download of a pre-1.2.0 vanilla build (no bundled
	// graphics) and blocks until answered. A nil Confirm declines.
	Confirm func(message string) bool

	// OnProcessStarted, when set, receives a function that blocks until the game
	// exits and returns its exit code. Leaving it nil keeps the launcher's
	// fire-and-forget behaviour: the game is started and never waited on.
	OnProcessStarted func(wait func() int)
}

func (d LaunchDeps) status(s string) {
	if d.UpdateStatus != nil {
		d.UpdateStatus(s)
	}
}

func (d LaunchDeps) failed() {
	if d.OnError != nil {
		d.OnError()
	}
}

func (d LaunchDeps) invalidateCaches() {
	if d.InvalidateCaches != nil {
		d.InvalidateCaches()
	}
}

func (d LaunchDeps) confirm(message string) bool {
	return d.Confirm != nil && d.Confirm(message)
}

// execute starts the game from versionFolder and reports the outcome. ctx must
// not be cancellable: cancelling after the game has started would kill it
// through exec.CommandContext.
func (d LaunchDeps) execute(ctx context.Context, versionFolder string, profile domain.Profile, client string) LaunchResult {
	started, wait := platform.StartOpenTTD(ctx, versionFolder, profile, d.Config.DocsBasePath, ClientSupportsCompanyPassword(client), d.Observer)
	if !started {
		d.status("Failed: OpenTTD did not start")
		d.failed()
		return LaunchStartError
	}
	d.status("Launch command sent")
	if d.OnProcessStarted != nil {
		d.OnProcessStarted(wait)
	}
	return LaunchStarted
}

// LaunchProfile runs the resolve, ensure-installed and start pipeline for one
// profile. ctx governs only the version check and the download/extract step;
// cancelling it is how the Cancel button works. The game starts on its own
// context, for the reason execute documents.
func LaunchProfile(ctx context.Context, profile domain.Profile, deps LaunchDeps) LaunchResult {
	deps.status("Resolving profile and version")
	deps.Observer.LogImportant(fmt.Sprintf("Launching profile %q", profile.Name))
	deps.Observer.LogVerbose(fmt.Sprintf("Profile config: version=%q savePath=%q server=%q company=%q", profile.Version, profile.SavePath, profile.ServerIpPort, profile.ServerCompanyNumber))

	requested := strings.TrimSpace(profile.Version)
	version := requested
	client := EffectiveClient(profile.Client, deps.Config.DefaultClient)

	if client == "custom" {
		folder := strings.TrimSpace(profile.CustomExecutablePath)
		if folder == "" {
			deps.status("Failed: custom executable folder is not set")
			deps.Observer.LogImportant("Custom client selected but no executable folder is configured.")
			deps.failed()
			return LaunchProfileError
		}
		if _, err := os.Stat(folder); err != nil {
			deps.status("Failed: custom executable folder does not exist")
			deps.Observer.LogImportant(fmt.Sprintf("Custom executable folder not found: %s (%v)", folder, err))
			deps.failed()
			return LaunchProfileError
		}
		deps.Observer.LogVerbose(fmt.Sprintf("Using custom executable folder: %s", folder))
		deps.status("Starting OpenTTD from custom folder")
		return deps.execute(context.Background(), folder, profile, client)
	}

	isLatestRequest := IsLatestVersion(requested)
	latestTrack := LatestTrack(client, requested)

	if isLatestRequest {
		deps.status(fmt.Sprintf("Resolving latest %s version (%s)", client, latestTrack))
		deps.Observer.LogImportant(fmt.Sprintf("Resolving latest %s version (%s)", client, latestTrack))
		version = platform.CheckForNewVersionForClientTrack(ctx, client, deps.Config, latestTrack)
		if errors.Is(ctx.Err(), context.Canceled) {
			// A cancel here must abort the whole launch, not fall back to
			// launching whatever's already installed; that would be Cancel
			// silently not cancelling, and the launch band would show a
			// dishonest "Launched" straight after the user clicked Cancel.
			deps.Observer.LogImportant("Cancelled: version check was cancelled by the user.")
			deps.status("Cancelled")
			deps.failed()
			return LaunchCancelled
		}
		if version == "" {
			// The remote lookup failed or returned nothing, typically because the
			// download server is unreachable (offline). Skip the update check and
			// fall back to launching the newest install already on disk.
			deps.Observer.LogImportant("Could not reach the download server to check for a newer version; falling back to the latest local install.")
			deps.status("Update check unavailable (offline?), using latest local install")
			// Highest-version install on this track (matches the online launch target
			// and the status dot); NOT newest-by-mod-time, which a re-downloaded older build wins.
			versionFolder := HighestInstalledFolderInRoot(deps.Config, client, latestTrack)
			if versionFolder == "" {
				deps.status("Failed: offline and no local install found for client")
				deps.Observer.LogImportant("No local install found for client, and the download server could not be reached.")
				deps.failed()
				return LaunchVersionError
			}
			deps.Observer.LogVerbose(fmt.Sprintf("Using latest local version folder: %s", versionFolder))
			deps.status("Starting OpenTTD from latest local install")
			return deps.execute(context.Background(), versionFolder, profile, client)
		}
	}
	if requested != "" && !isLatestRequest {
		deps.status(fmt.Sprintf("Using requested %s version %s", client, version))
		deps.Observer.LogImportant(fmt.Sprintf("Using requested %s version %s", client, version))
	}

	deps.status("Looking for local version folder")
	versionFolder := platform.FindVersionFolderClient(platform.ClientDownloadDir(deps.Config, client), version, client, deps.Config)
	if versionFolder == "" {
		deps.status("Version not found locally, downloading")
		deps.Observer.LogImportant(fmt.Sprintf("Version %s not found locally. Attempting to download for client %s.", version, client))
		// Block until the caller confirms before downloading a pre-1.2.0 vanilla build (no bundled graphics).
		if (client == "vanilla" || client == "vanilla-nightly") && platform.VanillaNeedsBaseSetWarning(version) {
			if !deps.confirm(veryOldVersionWarning(version)) {
				deps.Observer.LogImportant(fmt.Sprintf("Cancelled: %s needs manual setup before it can run.", version))
				deps.status("Cancelled (version needs manual setup)")
				deps.failed()
				return LaunchCancelled
			}
		}
		if !platform.DownloadAndExtractVersionForClientWithLogger(ctx, version, client, deps.Config, deps.Logger, deps.OnProgress) {
			if errors.Is(ctx.Err(), context.Canceled) {
				deps.status("Cancelled")
				deps.Observer.LogImportant(fmt.Sprintf("Cancelled: download of version %s was cancelled by the user.", version))
				deps.failed()
				return LaunchCancelled
			}
			deps.status(fmt.Sprintf("Failed: download of version %s did not complete", version))
			deps.Observer.LogImportant(fmt.Sprintf("Failed to download version %s for client %s.", version, client))
			deps.failed()
			return LaunchVersionError
		}
		// A new folder landed on disk; the dot cache's answers for this client are
		// now stale regardless of which view (or none) is showing when this completes.
		deps.invalidateCaches()
		deps.status("Download complete, resolving extracted folder")
		versionFolder = platform.FindVersionFolderClient(platform.ClientDownloadDir(deps.Config, client), version, client, deps.Config)
		if versionFolder == "" {
			deps.status("Failed: downloaded version folder could not be located")
			deps.Observer.LogImportant("Failed to locate downloaded version.")
			deps.failed()
			return LaunchVersionError
		}
	}

	// A cancel can land here with nothing left to interrupt (e.g. the version was
	// already installed locally, so the download step above never even ran);
	// check explicitly rather than launching anyway just because nothing failed.
	if errors.Is(ctx.Err(), context.Canceled) {
		deps.Observer.LogImportant("Cancelled: launch was cancelled by the user before starting OpenTTD.")
		deps.status("Cancelled")
		deps.failed()
		return LaunchCancelled
	}

	deps.Observer.LogVerbose(fmt.Sprintf("Using version folder: %s", versionFolder))
	deps.status("Starting OpenTTD")
	return deps.execute(context.Background(), versionFolder, profile, client)
}

// veryOldVersionWarning is the text put to Confirm before downloading a
// pre-1.2.0 build, so the GUI dialog and a headless refusal describe the same problem.
func veryOldVersionWarning(version string) string {
	return fmt.Sprintf("OpenTTD %s needs manual setup before it will run through RunTTD. "+
		"Versions before 1.2.0 don't include free graphics, so you'll need original "+
		"Transport Tycoon Deluxe data files to play. Some old releases also predate builds for many systems.", version)
}
