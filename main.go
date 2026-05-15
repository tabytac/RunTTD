package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"
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

type Profile struct {
	Name                  string `json:"name"`
	Version               string `json:"version"`
	SavePath              string `json:"savePath"`
	ServerIpPort          string `json:"serverIpPort"`
	ServerPassword        string `json:"serverPassword"`
	ServerCompanyNumber   string `json:"serverCompanyNumber"`
	ServerCompanyPassword string `json:"serverCompanyPassword"`
	LaunchMode            string `json:"launchMode"` // "", "file", "folder", "multiplayer"
	ExtraArgs             string `json:"extraArgs"`
}

type Config struct {
	FirstRun         bool      `json:"-"`
	ParentDir        string    `json:"parentDir"`
	DocsBasePath     string    `json:"docsBasePath"`
	GithubApiUrl     string    `json:"githubApiUrl"`
	OSType           string    `json:"osType"`
	AutoCloseOnStart bool      `json:"autoCloseOnStart"`
	Verbose          bool      `json:"verbose"`
	LogToFile        bool      `json:"logToFile"`
	Profiles         []Profile `json:"profiles"`
}

type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if len(config.Profiles) == 0 {
		config.Profiles = []Profile{{Name: "Default", Version: "latest"}}
	}

	return &config, nil
}

func SaveConfig(filename string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

func resolveConfigPath() string {
	candidates := make([]string, 0, 3)

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "config.json"))
	}
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "config.json"),
			filepath.Join(filepath.Dir(exeDir), "config.json"),
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
	return "config.json"
}

func resolveLogPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "log.txt")
}

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

func FindLatestSaveFile(gamePath string) string {
	entries, err := os.ReadDir(gamePath)
	if err != nil {
		return ""
	}

	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if !entry.IsDir() && (ext == ".sav" || ext == ".scn") {
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

func ExecuteOpenTTD(versionFolder string, ipPort, companyNumber, serverPassword, companyPassword, savePath, launchMode, extraArgs string, l *Logger, um *UIManager) {
	var saveFile string
	var finalIpPort string

	switch launchMode {
	case "multiplayer":
		finalIpPort = ipPort
	case "file", "folder":
		if savePath != "" {
			gamePath := savePath
			if !filepath.IsAbs(savePath) {
				gamePath = filepath.Join(um.config.DocsBasePath, "save", savePath)
			}

			info, err := os.Stat(gamePath)
			if err == nil {
				if info.IsDir() {
					saveFile = FindLatestSaveFile(gamePath)
				} else {
					saveFile = gamePath
				}
			} else {
				l.Append(fmt.Sprintf("Save path not found: %s (%v)", gamePath, err))
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
				l.Append(fmt.Sprintf("Executable not found in %s (also checked .app bundles)", versionFolder))
				return
			}
		} else {
			l.Append(fmt.Sprintf("Executable not found in %s", versionFolder))
			return
		}
	}

	var cmd *exec.Cmd
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

	cmd = exec.Command(exePath, args...)
	um.LogVerbose(fmt.Sprintf("Running command: %s %v", exePath, args))
	cmd.SysProcAttr = getDetachedSysProcAttr()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		um.LogImportant(fmt.Sprintf("Failed to start OpenTTD: %v", err))
		return
	}

	um.LogImportant("OpenTTD started successfully")
	um.OnOpenTTDStarted()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			um.LogVerbose(scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			um.LogVerbose("ERR: " + scanner.Text())
		}
	}()
	go func() {
		if err := cmd.Wait(); err != nil {
			um.LogImportant(fmt.Sprintf("OpenTTD exited with error: %v", err))
		} else {
			um.LogVerbose("OpenTTD exited normally")
		}
	}()
}

func DownloadAndExtractVersion(version string, config *Config) bool {
	repoURL := fmt.Sprintf("%s/releases/tags/jgrpp-%s", config.GithubApiUrl, version)
	downloadDir := config.ParentDir

	resp, err := http.Get(repoURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}

	var releaseInfo ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		fmt.Printf("Failed to parse release info: %v\n", err)
		return false
	}

	tagName := releaseInfo.TagName
	extractedFolder := fmt.Sprintf("openttd-%s-%s", tagName, config.OSType)
	if _, err := os.Stat(filepath.Join(downloadDir, extractedFolder)); err == nil {
		return true
	}

	var downloadURL string
	var assetName string
	for _, asset := range releaseInfo.Assets {
		if strings.Contains(asset.Name, config.OSType) && (strings.HasSuffix(asset.Name, ".zip") || strings.HasSuffix(asset.Name, ".tar.xz") || strings.HasSuffix(asset.Name, ".dmg")) {
			downloadURL = asset.BrowserDownloadURL
			assetName = asset.Name
			break
		}
	}
	if downloadURL == "" {
		fmt.Printf("No downloadable asset found for version %s\n", tagName)
		return false
	}

	archivePath := filepath.Join(downloadDir, assetName)
	fmt.Printf("Downloading version: %s\n", tagName)

	// Ensure the download directory exists
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		fmt.Printf("Failed to create download directory %s: %v\n", downloadDir, err)
		return false
	}

	resp, err = http.Get(downloadURL)
	if err != nil {
		fmt.Printf("Failed to download file: %v\n", err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}

	file, err := os.Create(archivePath)
	if err != nil {
		return false
	}

	if _, err = io.Copy(file, resp.Body); err != nil {
		file.Close()
		os.Remove(archivePath)
		return false
	}
	file.Close() // Must close before extraction — Windows locks open files

	if err := extractArchive(archivePath, downloadDir); err != nil {
		os.Remove(archivePath)
		return false
	}

	os.Remove(archivePath)
	return true
}

type Logger struct {
	mu        sync.Mutex
	lines     []string
	logToFile bool
	logPath   string
}

func NewLogger(logToFile bool, logPath string) *Logger {
	return &Logger{logToFile: logToFile, logPath: logPath}
}

func appendToLogFile(path, msg string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), msg))
}

func shouldLogToFile(configPath string) bool {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}
	var cfg struct {
		LogToFile bool `json:"logToFile"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}
	return cfg.LogToFile
}

func (l *Logger) Append(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, s)
	if len(l.lines) > 2000 {
		l.lines = l.lines[len(l.lines)-2000:]
	}
	if l.logToFile {
		appendToLogFile(l.logPath, s)
	}
}

func (l *Logger) GetAll() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}

func extractArchive(archivePath, destDir string) error {
	if strings.HasSuffix(archivePath, ".tar.xz") {
		cmd := exec.Command("tar", "-xf", archivePath, "-C", destDir)
		return cmd.Run()
	}
	if strings.HasSuffix(archivePath, ".dmg") {
		return extractDMG(archivePath, destDir)
	}
	// .zip
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s'", archivePath, destDir))
	} else {
		cmd = exec.Command("unzip", "-q", archivePath, "-d", destDir)
	}
	return cmd.Run()
}

func extractDMG(dmgPath, destDir string) error {
	mountPoint := filepath.Join(os.TempDir(), "jgrpp_dmg_mount")
	_ = os.MkdirAll(mountPoint, 0755)
	defer func() {
		_ = exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
		_ = os.RemoveAll(mountPoint)
	}()

	if err := exec.Command("hdiutil", "attach", "-nobrowse", "-mountpoint", mountPoint, dmgPath).Run(); err != nil {
		return fmt.Errorf("failed to mount DMG: %w", err)
	}

	// Derive output folder name from DMG filename (e.g. openttd-jgrpp-0.72.2-macos-universal)
	baseName := filepath.Base(dmgPath)
	baseName = strings.TrimSuffix(baseName, ".dmg")
	outputDir := filepath.Join(destDir, baseName)
	_ = os.MkdirAll(outputDir, 0755)

	// Copy .app bundles from the mounted volume
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

func defaultOSType() string {
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

func CheckForNewVersion(config *Config) string {
	repoURL := fmt.Sprintf("%s/releases/latest", config.GithubApiUrl)

	resp, err := http.Get(repoURL)
	if err != nil {
		fmt.Printf("Failed to get latest release info: %v\n", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("Failed to get latest release info: HTTP %d\n", resp.StatusCode)
		return ""
	}

	var releaseInfo ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releaseInfo); err != nil {
		fmt.Printf("Failed to parse latest release info: %v\n", err)
		return ""
	}

	parts := strings.Split(releaseInfo.TagName, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func FetchAvailableVersions(config *Config) ([]string, error) {
	repoURL := fmt.Sprintf("%s/releases?per_page=20", config.GithubApiUrl)

	resp, err := http.Get(repoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get release info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get release info: HTTP %d", resp.StatusCode)
	}

	var releases []ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	versions := []string{"latest"}
	for _, release := range releases {
		tag := release.TagName
		if strings.HasPrefix(tag, "jgrpp-") {
			tag = strings.TrimPrefix(tag, "jgrpp-")
		}
		versions = append(versions, tag)
	}
	return versions, nil
}

func main() {
	setupGuiOutput()
	configPath := resolveConfigPath()
	logPath := resolveLogPath(configPath)
	bootstrapFileLog := shouldLogToFile(configPath)
	if bootstrapFileLog {
		_ = os.WriteFile(logPath, []byte{}, 0644) // Clear old logs from previous sessions
		appendToLogFile(logPath, fmt.Sprintf("Launcher process starting (config: %s)", configPath))
	}

	defer func() {
		if r := recover(); r != nil {
			message := fmt.Sprintf("panic: %v\n%s", r, string(debug.Stack()))
			if bootstrapFileLog {
				appendToLogFile(logPath, message)
			}
			fmt.Fprintln(os.Stderr, message)
			os.Exit(1)
		}
	}()

	config, err := LoadConfig(configPath)
	if err != nil {
		// If config is missing, create a sensible default and continue.
		if errors.Is(err, os.ErrNotExist) {
			docsBase := getDocumentsDir()

			// OpenTTD folder name varies by OS
			ottdDirName := "OpenTTD"
			if runtime.GOOS == "linux" {
				ottdDirName = "openttd"
			}

			defaultDocsDir := filepath.Join(docsBase, ottdDirName)
			defaultParentDir := filepath.Join(docsBase, ottdDirName+"-JGRPP")

			// Validate: check for openttd.cfg to confirm this is the right folder
			cfgPath := filepath.Join(defaultDocsDir, "openttd.cfg")
			if _, statErr := os.Stat(cfgPath); statErr != nil {
				if bootstrapFileLog {
					appendToLogFile(logPath, fmt.Sprintf("Warning: openttd.cfg not found at %s — docs path may need adjusting in Settings", cfgPath))
				}
			}

			defaultCfg := &Config{
				FirstRun:         true,
				ParentDir:        defaultParentDir,
				DocsBasePath:     defaultDocsDir,
				GithubApiUrl:     "https://api.github.com/repos/JGRennison/OpenTTD-patches",
				OSType:           defaultOSType(),
				AutoCloseOnStart: false,
				Verbose:          false,
				LogToFile:        false,
				Profiles:         []Profile{{Name: "Default", Version: "latest"}},
			}

			if saveErr := SaveConfig(configPath, defaultCfg); saveErr != nil {
				message := fmt.Sprintf("Failed to create default config at %s: %v", configPath, saveErr)
				if bootstrapFileLog {
					appendToLogFile(logPath, message)
				}
				fmt.Fprintln(os.Stderr, message)
				os.Exit(1)
			}

			config = defaultCfg
			if config.LogToFile {
				appendToLogFile(logPath, fmt.Sprintf("Default config created at %s", configPath))
			}
		} else {
			message := fmt.Sprintf("Startup failed while loading config: %v", err)
			if bootstrapFileLog {
				appendToLogFile(logPath, message)
			}
			fmt.Fprintln(os.Stderr, message)
			os.Exit(1)
		}
	}

	if config.LogToFile {
		appendToLogFile(logPath, fmt.Sprintf("Config loaded successfully from %s", configPath))
	}

	ui := NewUIManager(config, configPath)
	if config.LogToFile {
		appendToLogFile(logPath, "Launching UI")
	}
	ui.Show()
	if config.LogToFile {
		appendToLogFile(logPath, "UI exited")
	}
}
