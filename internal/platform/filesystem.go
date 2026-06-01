package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"runttd/internal/domain"
)

// DefaultOSType returns the platform-specific release package naming classification
func DefaultOSType() string {
	switch runtime.GOOS {
	case "windows":
		switch runtime.GOARCH {
		case "386":
			return "windows-win32"
		case "arm64":
			return "windows-arm64"
		default:
			return "windows-win64"
		}
	case "darwin":
		return "macos-universal"
	default:
		return "linux-generic-amd64"
	}
}

// ClientPlatformAliases resolves lowercase platform tags for filename searches based on configurations
func ClientPlatformAliases(cfg *domain.Config) []string {
	if cfg == nil {
		return []string{DefaultOSType()}
	}
	osType := strings.TrimSpace(cfg.OSType)
	if osType == "" {
		osType = DefaultOSType()
	}
	return []string{strings.ToLower(osType)}
}

// FolderMatchesAnyAlias verifies if a folder name contains any specified OS platform alias
func FolderMatchesAnyAlias(name string, aliases []string) bool {
	for _, alias := range aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == "" {
			continue
		}
		if strings.Contains(name, alias) {
			return true
		}
	}
	return false
}

// VersionMatchesFolder checks if the directory name matches the given version tag pattern
func VersionMatchesFolder(name, version string) bool {
	return strings.Contains(name, version) || strings.HasPrefix(name, version) || strings.Contains(name, "-"+version+"-") || strings.HasSuffix(name, "-"+version)
}

func ClientDownloadDir(cfg *domain.Config, client string) string {
	if cfg == nil {
		return ""
	}
	if cfg.SubfolderPerClient && client != "" {
		return filepath.Join(cfg.ParentDir, client)
	}
	return cfg.ParentDir
}

// runExtractor runs an extraction command, reporting a clear error if the tool is missing or extraction fails
func runExtractor(tool, archivePath string, cmd *exec.Cmd) error {
	if _, err := exec.LookPath(tool); err != nil {
		return fmt.Errorf("%q not found on PATH — install it to extract %s", tool, filepath.Base(archivePath))
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract %s with %q: %w", filepath.Base(archivePath), tool, err)
	}
	return nil
}

// ExtractArchive decompresses tar.xz, zip, or dmg archives depending on current operating system capabilities
func ExtractArchive(archivePath, destDir string) error {
	if strings.HasSuffix(archivePath, ".tar.xz") {
		cmd := exec.Command("tar", "-xf", archivePath, "-C", destDir)
		return runExtractor("tar", archivePath, cmd)
	}
	if strings.HasSuffix(archivePath, ".dmg") {
		return ExtractDMG(archivePath, destDir)
	}
	// .zip
	if runtime.GOOS == "windows" {
		// Expand-Archive's parameter binder cannot read $args[N], so the paths
		// are embedded directly, single-quoted with '' escaping to stay injection-safe
		script := fmt.Sprintf(
			"Expand-Archive -LiteralPath '%s' -DestinationPath '%s' -Force",
			strings.ReplaceAll(archivePath, "'", "''"),
			strings.ReplaceAll(destDir, "'", "''"),
		)
		cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to extract %s with Expand-Archive: %w", filepath.Base(archivePath), err)
		}
		return nil
	}
	cmd := exec.Command("unzip", "-q", archivePath, "-d", destDir)
	return runExtractor("unzip", archivePath, cmd)
}

// ExtractDMG mounts a macOS DMG and copies over any contained .app bundles into target directory
func ExtractDMG(dmgPath, destDir string) error {
	if _, err := exec.LookPath("hdiutil"); err != nil {
		return fmt.Errorf("%q not found on PATH — install it to extract %s", "hdiutil", filepath.Base(dmgPath))
	}
	mountPoint, err := os.MkdirTemp("", "runttd_dmg_mount_")
	if err != nil {
		return fmt.Errorf("failed to create DMG mount dir: %w", err)
	}
	defer func() {
		_ = exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
		_ = os.RemoveAll(mountPoint)
	}()

	if err := exec.Command("hdiutil", "attach", "-nobrowse", "-mountpoint", mountPoint, dmgPath).Run(); err != nil {
		return fmt.Errorf("failed to mount DMG: %w", err)
	}

	baseName := filepath.Base(dmgPath)
	baseName = strings.TrimSuffix(baseName, ".dmg")
	outputDir := filepath.Join(destDir, baseName)
	_ = os.MkdirAll(outputDir, 0755)

	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		return fmt.Errorf("failed to read mounted DMG: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), ".app") {
			src := filepath.Join(mountPoint, entry.Name())
			dst := filepath.Join(outputDir, entry.Name())
			if err := exec.Command("cp", "-R", src, dst).Run(); err != nil {
				return fmt.Errorf("failed to copy %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

// FindVersionFolder searches the folder directory for a specific JGRPP version folder
func FindVersionFolder(parentDir, version string) string {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), fmt.Sprintf("jgrpp-%s", version)) {
			return filepath.Join(parentDir, entry.Name())
		}
	}
	return ""
}

// FindVersionFolderClient finds client-specific installations using matching platform architecture aliases
func FindVersionFolderClient(parentDir, version, client string, cfg *domain.Config) string {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return ""
	}

	platformAliases := ClientPlatformAliases(cfg)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch client {
		case "jgrpp":
			if len(platformAliases) > 0 && !FolderMatchesAnyAlias(strings.ToLower(name), platformAliases) {
				continue
			}
			if strings.Contains(name, fmt.Sprintf("jgrpp-%s", version)) || (strings.Contains(name, version) && strings.Contains(strings.ToLower(name), "jgrpp")) {
				return filepath.Join(parentDir, name)
			}
		case "vanilla", "vanilla-nightly":
			lname := strings.ToLower(name)
			if !strings.Contains(lname, "openttd") {
				continue
			}
			if !VersionMatchesFolder(name, version) {
				continue
			}
			if len(platformAliases) > 0 && !FolderMatchesAnyAlias(lname, platformAliases) {
				continue
			}
			return filepath.Join(parentDir, name)
		default:
			if strings.Contains(name, version) {
				return filepath.Join(parentDir, name)
			}
		}
	}
	return ""
}

// FindLatestFolder finds the most recently updated JGRPP directory folder
func FindLatestFolder(parentDir string) string {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return ""
	}

	var latestFolder string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), "jgrpp") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFolder = filepath.Join(parentDir, entry.Name())
			}
		}
	}
	return latestFolder
}

// FindLatestFolderClient searches directory files and returns the newest installation path for a client
func FindLatestFolderClient(parentDir, client string) string {
	return FindLatestFolderClientWithConfig(parentDir, client, nil)
}

// FindLatestFolderClientWithConfig locates the latest installed folder for a given profile client, using active config platform rules
func FindLatestFolderClientWithConfig(parentDir, client string, cfg *domain.Config) string {
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		return ""
	}

	var latestFolder string
	var latestTime time.Time
	platformAliases := ClientPlatformAliases(cfg)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		switch client {
		case "jgrpp":
			if !strings.Contains(name, "jgrpp") {
				continue
			}
			if len(platformAliases) > 0 && !FolderMatchesAnyAlias(name, platformAliases) {
				continue
			}
		case "vanilla", "vanilla-nightly":
			if !strings.Contains(name, "openttd") {
				continue
			}
			if len(platformAliases) > 0 && !FolderMatchesAnyAlias(name, platformAliases) {
				continue
			}
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latestFolder = filepath.Join(parentDir, entry.Name())
		}
	}
	return latestFolder
}

// FindLatestSaveFile crawls save directory files returning the newest game file matching the specified save filter
func FindLatestSaveFile(gamePath string, filter string) string {
	entries, err := os.ReadDir(gamePath)
	if err != nil {
		return ""
	}

	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))

		match := false
		switch filter {
		case "sav":
			match = ext == ".sav"
		case "scn":
			match = ext == ".scn"
		default: // "both" or empty
			match = ext == ".sav" || ext == ".scn"
		}

		if match {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = filepath.Join(gamePath, entry.Name())
			}
		}
	}
	return latestFile
}

// ResolveConfigPath searches candidate directories and returns the absolute config path
func ResolveConfigPath() string {
	candidates := make([]string, 0, 3)

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "runttd-config.json"))
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "runttd-config.json"),
			filepath.Join(filepath.Dir(exeDir), "runttd-config.json"),
		)
	}

	seen := map[string]bool{}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}

	if len(candidates) > 0 {
		return candidates[0]
	}
	return "runttd-config.json"
}

// ResolveLogPath returns the resolved log file path located next to the config file
func ResolveLogPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "log.txt")
}
