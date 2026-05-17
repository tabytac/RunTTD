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
)

// ProcessObserver provides lifecycle hook notifications and operational logging for executing background game processes
type ProcessObserver interface {
	LogVerbose(msg string)
	LogImportant(msg string)
	OnStarted()
}

// ExecuteOpenTTD starts the OpenTTD application using profiles and detached platform routines
func ExecuteOpenTTD(
	ctx context.Context,
	versionFolder string,
	ipPort, companyNumber, serverPassword, companyPassword string,
	savePath, launchMode, extraArgs string,
	autoLatestFilter string,
	docsBasePath string,
	obs ProcessObserver,
) {
	var saveFile string
	var finalIpPort string

	switch launchMode {
	case "multiplayer":
		finalIpPort = ipPort
	case "file", "folder":
		if savePath != "" {
			gamePath := savePath
			if !filepath.IsAbs(savePath) {
				gamePath = filepath.Join(docsBasePath, "save", savePath)
			}

			info, err := os.Stat(gamePath)
			if err == nil {
				if info.IsDir() {
					saveFile = FindLatestSaveFile(gamePath, autoLatestFilter)
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
				return
			}
		} else {
			obs.LogImportant(fmt.Sprintf("Executable not found in %s", versionFolder))
			return
		}
	}

	var args []string

	if finalIpPort != "" {
		nArg := finalIpPort
		if companyNumber != "" {
			nArg = fmt.Sprintf("%s#%s", finalIpPort, companyNumber)
		}
		args = append(args, "-n", nArg)

		if serverPassword != "" {
			args = append(args, "-p", serverPassword)
		}
		if companyPassword != "" {
			args = append(args, "-P", companyPassword)
		}
	}

	if saveFile != "" {
		args = append(args, "-g", saveFile)
	}

	// Append extra arguments from the Advanced tab
	if extraArgs != "" {
		fields := strings.Fields(extraArgs)
		args = append(args, fields...)
	}

	cmd := exec.CommandContext(ctx, exePath, args...)
	obs.LogVerbose(fmt.Sprintf("Running command: %s %v", exePath, args))
	cmd.SysProcAttr = GetDetachedSysProcAttr()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		obs.LogImportant(fmt.Sprintf("Failed to start OpenTTD: %v", err))
		return
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
}
