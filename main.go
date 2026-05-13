package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Profile represents a launch profile (per-launch settings)
type Profile struct {
	Name                  string `json:"name"`
	Version               string `json:"version"`
	SavePath              string `json:"savePath"`
	ServerIpPort          string `json:"serverIpPort"`
	ServerPassword        string `json:"serverPassword"`
	ServerCompanyNumber   string `json:"serverCompanyNumber"`
	ServerCompanyPassword string `json:"serverCompanyPassword"`
}

// Config holds global launcher configuration and profiles
type Config struct {
	// Global settings
	ParentDir        string `json:"parentDir"`
	DocsBasePath     string `json:"docsBasePath"`
	GithubApiUrl     string `json:"githubApiUrl"`
	OSType           string `json:"osType"`
	AutoCloseOnStart bool   `json:"autoCloseOnStart"` // default: false (keep window open)
	Verbose          bool   `json:"verbose"`          // default: false (only show important messages)

	// Profiles for different launch configurations
	Profiles []Profile `json:"profiles"`
}

// ReleaseInfo represents GitHub release information
type ReleaseInfo struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// LoadConfig loads configuration from config.json
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Ensure at least one profile exists
	if len(config.Profiles) == 0 {
		config.Profiles = []Profile{
			{
				Name:    "Default",
				Version: config.OSType,
			},
		}
	}

	return &config, nil
}

// SaveConfig saves configuration to config.json
func SaveConfig(filename string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// FindVersionFolder returns the first subfolder matching the version pattern
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

// FindLatestFolder returns the most recently modified jgrpp folder
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

// FindLatestSaveFile returns the most recently modified .sav file
func FindLatestSaveFile(gamePath string) string {
	entries, err := os.ReadDir(gamePath)
	if err != nil {
		return ""
	}

	var latestFile string
	var latestTime time.Time

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sav") {
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

// ExecuteOpenTTD launches the OpenTTD executable with specified parameters
func ExecuteOpenTTD(versionFolder string, ipPort, companyNumber, serverPassword, companyPassword, savePath string, l *Logger, um *UIManager) {
	// Find the save file if savePath is specified
	var saveFile string
	if savePath != "" {
		gamePath := filepath.Join(um.config.DocsBasePath, "save", savePath)
		saveFile = FindLatestSaveFile(gamePath)
	}

	exePath := filepath.Join(versionFolder, "openttd.exe")
	if _, err := os.Stat(exePath); err != nil {
		l.Append(fmt.Sprintf("Executable not found in %s", versionFolder))
		return
	}

	var cmd *exec.Cmd
	var args []string

	if ipPort != "" && serverPassword != "" {
		nArg := ipPort
		if companyNumber != "" && companyPassword != "" {
			nArg = fmt.Sprintf("%s#%s", ipPort, companyNumber)
			args = []string{"-n", nArg, "-p", serverPassword, "-P", companyPassword}
		} else {
			nArg = fmt.Sprintf("%s#255", ipPort)
			args = []string{"-n", nArg, "-p", serverPassword}
		}
	}

	if saveFile != "" {
		args = append(args, "-g", saveFile)
	}

	// Start a process and capture its output so we can show logs in the UI.
	cmd = exec.Command(exePath, args...)
	um.LogVerbose(fmt.Sprintf("Running command: %s %v", exePath, args))

	// Set up process attributes for detaching if auto-close is enabled
	cmd.SysProcAttr = getDetachedSysProcAttr()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		um.LogImportant(fmt.Sprintf("Failed to start OpenTTD: %v", err))
		return
	}

	um.LogImportant("OpenTTD started successfully")
	um.OnOpenTTDStarted()

	// Read stdout/stderr and forward to logger on main UI thread
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			text := scanner.Text()
			um.app.RunOnMainThread(func() {
				um.LogVerbose(text)
			})
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			text := scanner.Text()
			um.app.RunOnMainThread(func() {
				um.LogVerbose("ERR: " + text)
			})
		}
	}()

	go func() {
		if err := cmd.Wait(); err != nil {
			um.app.RunOnMainThread(func() {
				um.LogImportant(fmt.Sprintf("OpenTTD exited with error: %v", err))
			})
		} else {
			um.app.RunOnMainThread(func() {
				um.LogVerbose("OpenTTD exited normally")
			})
		}
	}()
}

// DownloadAndExtractVersion downloads and extracts a specific JGR version
func DownloadAndExtractVersion(version string, config *Config) bool {
	repoURL := fmt.Sprintf("%s/releases/tags/jgrpp-%s", config.GithubApiUrl, version)
	downloadDir := config.ParentDir

	resp, err := http.Get(repoURL)
	if err != nil {
		fmt.Printf("Failed to get release info for version %s: %v\n", version, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("Failed to get release info for version %s: HTTP %d\n", version, resp.StatusCode)
		return false
	}

	var releaseInfo ReleaseInfo
	err = json.NewDecoder(resp.Body).Decode(&releaseInfo)
	if err != nil {
		fmt.Printf("Failed to parse release info: %v\n", err)
		return false
	}

	tagName := releaseInfo.TagName
	extractedFolder := fmt.Sprintf("openttd-%s-%s", tagName, config.OSType)
	if _, err := os.Stat(filepath.Join(downloadDir, extractedFolder)); err == nil {
		return true
	}

	var downloadURL string
	for _, asset := range releaseInfo.Assets {
		if strings.Contains(asset.Name, config.OSType) && strings.HasSuffix(asset.Name, ".zip") {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		fmt.Printf("No downloadable asset found for version %s\n", tagName)
		return false
	}

	zipPath := filepath.Join(downloadDir, fmt.Sprintf("%s.zip", tagName))
	fmt.Printf("Downloading version: %s\n", tagName)

	resp, err = http.Get(downloadURL)
	if err != nil {
		fmt.Printf("Failed to download file: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("Failed to download file: HTTP %d\n", resp.StatusCode)
		return false
	}

	file, err := os.Create(zipPath)
	if err != nil {
		fmt.Printf("Failed to create zip file: %v\n", err)
		return false
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		fmt.Printf("Failed to write zip file: %v\n", err)
		os.Remove(zipPath)
		return false
	}

	// Extract ZIP
	err = extractZip(zipPath, downloadDir)
	if err != nil {
		fmt.Printf("Failed to extract zip: %v\n", err)
		os.Remove(zipPath)
		return false
	}

	fmt.Printf("Download and extraction completed: %s\n", tagName)
	os.Remove(zipPath)
	return true
}

// Simple in-memory logger used by the local web UI
type Logger struct {
	mu    sync.Mutex
	lines []string
}

func (l *Logger) Append(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, s)
	if len(l.lines) > 2000 {
		l.lines = l.lines[len(l.lines)-2000:]
	}
}

func (l *Logger) GetAll() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}


// extractZip extracts a zip file to a directory
func extractZip(zipPath, destDir string) error {
	// Using a simpler approach - this is a placeholder
	// For production, use github.com/klauspost/compress/zip or similar
	// For now, we'll rely on system unzip command on Unix or PowerShell on Windows
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s'", zipPath, destDir))
	} else {
		cmd = exec.Command("unzip", "-q", zipPath, "-d", destDir)
	}

	return cmd.Run()
}

// CheckForNewVersion returns the latest JGR version number from GitHub
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
	err = json.NewDecoder(resp.Body).Decode(&releaseInfo)
	if err != nil {
		fmt.Printf("Failed to parse latest release info: %v\n", err)
		return ""
	}

	parts := strings.Split(releaseInfo.TagName, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// Main launches the GUI launcher
func main() {
	config, err := LoadConfig("config.json")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	ui := NewUIManager(config)
	ui.Show()
}
