package platform

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"runttd/internal/domain"
)

// ProcessObserver provides lifecycle hook notifications and operational logging for executing background game processes
type ProcessObserver interface {
	LogVerbose(msg string)
	LogImportant(msg string)
	OnStarted()
}

// ExecuteOpenTTD starts the OpenTTD application for the given profile using
// detached platform routines. versionFolder is the resolved client install
// directory; docsBasePath anchors relative save/config paths; obs receives
// lifecycle and logging callbacks. Returns whether the process actually
// started (obs.OnStarted() was called) — callers must check this rather than
// assume success, since every failure here is otherwise reported only via
// LogImportant.
func ExecuteOpenTTD(
	ctx context.Context,
	versionFolder string,
	profile domain.Profile,
	docsBasePath string,
	allowCompanyPassword bool,
	obs ProcessObserver,
) bool {
	var saveFile string

	switch profile.LaunchMode {
	case "file", "folder":
		if profile.SavePath != "" {
			gamePath := ResolveProfileSavePath(docsBasePath, profile.SavePath)

			info, err := os.Stat(gamePath)
			if err == nil {
				if info.IsDir() {
					saveFile = FindLatestSaveFile(gamePath, profile.AutoLatestFilter)
				} else {
					saveFile = gamePath
				}
			} else {
				obs.LogImportant(fmt.Sprintf("Save path not found: %s (%v)", gamePath, err))
			}
		}
	}

	exeName := "openttd"
	if runtime.GOOS == "windows" {
		exeName = "openttd.exe"
	}
	exePath := filepath.Join(versionFolder, exeName)
	if _, err := os.Stat(exePath); err != nil {
		// macOS: look inside .app bundle
		if runtime.GOOS == "darwin" {
			appGlob, _ := filepath.Glob(filepath.Join(versionFolder, "*.app", "Contents", "MacOS", "openttd"))
			if len(appGlob) > 0 {
				exePath = appGlob[0]
			} else {
				obs.LogImportant(fmt.Sprintf("Executable not found in %s (also checked .app bundles)", versionFolder))
				return false
			}
		} else {
			obs.LogImportant(fmt.Sprintf("Executable not found in %s", versionFolder))
			return false
		}
	}

	configPath := ResolveProfileConfigOverride(docsBasePath, profile.ConfigFilePath)

	args := buildLaunchArgs(profile, saveFile, configPath, allowCompanyPassword)

	cmd := exec.CommandContext(ctx, exePath, args...)
	if len(args) > 0 {
		obs.LogVerbose(fmt.Sprintf("Running command: %s %v", exePath, args))
	} else {
		obs.LogVerbose(fmt.Sprintf("Running command: %s", exePath))
	}
	cmd.SysProcAttr = GetDetachedSysProcAttr()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		obs.LogImportant(fmt.Sprintf("Failed to start OpenTTD: %v", err))
		return false
	}

	obs.LogImportant("OpenTTD started successfully")
	obs.OnStarted()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			obs.LogVerbose(scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			obs.LogVerbose("ERR: " + scanner.Text())
		}
	}()
	go func() {
		if err := cmd.Wait(); err != nil {
			obs.LogImportant(fmt.Sprintf("OpenTTD exited with error: %v", err))
		} else {
			obs.LogVerbose("OpenTTD exited normally")
		}
	}()
	return true
}

// ResolveProfileSavePath resolves a profile's save target the way launch does:
// absolute as-is, else under <docsBase>/save. Callers pass a non-empty savePath.
func ResolveProfileSavePath(docsBase, savePath string) string {
	if filepath.IsAbs(savePath) {
		return savePath
	}
	return filepath.Join(docsBase, "save", savePath)
}

// ResolveProfileConfigOverride resolves a profile's -c override, or "" when unset:
// absolute as-is, else under docsBase.
func ResolveProfileConfigOverride(docsBase, raw string) string {
	p := strings.TrimSpace(raw)
	if p != "" && !filepath.IsAbs(p) && docsBase != "" {
		p = filepath.Join(docsBase, p)
	}
	return p
}

// buildLaunchArgs assembles the OpenTTD CLI arguments for a profile. saveFile and
// configPath are pre-resolved absolute/relative strings; allowCompanyPassword gates
// the JGRPP-only -P flag (callers pass app.ClientSupportsCompanyPassword(effClient)).
func buildLaunchArgs(profile domain.Profile, saveFile, configPath string, allowCompanyPassword bool) []string {
	var args []string

	var finalIpPort string
	if profile.LaunchMode == "multiplayer" {
		finalIpPort = profile.ServerIpPort
	}

	if finalIpPort != "" {
		nArg := finalIpPort
		if profile.ServerCompanyNumber != "" {
			nArg = fmt.Sprintf("%s#%s", finalIpPort, profile.ServerCompanyNumber)
		}
		args = append(args, "-n", nArg)

		if profile.ServerPassword != "" {
			args = append(args, "-p", profile.ServerPassword)
		}
		if profile.ServerCompanyPassword != "" && allowCompanyPassword {
			args = append(args, "-P", profile.ServerCompanyPassword)
		}
	}

	if saveFile != "" {
		args = append(args, "-g", saveFile)
	}

	if configPath != "" {
		args = append(args, "-c", configPath)
	}

	if profile.NoConfigSave {
		args = append(args, "-x")
	}

	// NewGRF scan mode dedicated flags
	switch strings.ToUpper(strings.TrimSpace(profile.NewGRFScanMode)) {
	case "Q":
		args = append(args, "-Q")
	case "QQ":
		args = append(args, "-QQ")
	}

	// Append extra arguments from the Advanced tab, unless the user toggled them off
	// (the text itself is preserved on the profile either way).
	if profile.ExtraArgs != "" && !profile.ExtraArgsDisabled {
		fields := stripDedicatedConfigArgs(strings.Fields(profile.ExtraArgs))
		args = append(args, fields...)
	}

	return args
}

// dedicatedConfigFlags maps each stripped flag to whether it also consumes the next arg (true only for -c's config path).
var dedicatedConfigFlags = map[string]bool{
	"-x":  false,
	"-c":  true,
	"-q":  false,
	"-qq": false,
}

func stripDedicatedConfigArgs(fields []string) []string {
	filtered := make([]string, 0, len(fields))
	skipNext := false

	for _, field := range fields {
		if skipNext {
			skipNext = false
			continue
		}

		lower := strings.ToLower(field)
		if takesValue, drop := dedicatedConfigFlags[lower]; drop {
			skipNext = takesValue
			continue
		}
		if strings.HasPrefix(lower, "-c=") {
			continue
		}
		filtered = append(filtered, field)
	}

	return filtered
}
