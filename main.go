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
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type Profile struct {
	Name                  string `json:"name"`
	Version               string `json:"version"`
	SavePath              string `json:"savePath"`
	ServerIpPort          string `json:"serverIpPort"`
	ServerPassword        string `json:"serverPassword"`
	ServerCompanyNumber   string `json:"serverCompanyNumber"`
	ServerCompanyPassword string `json:"serverCompanyPassword"`
}

type Config struct {
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

func ExecuteOpenTTD(versionFolder string, ipPort, companyNumber, serverPassword, companyPassword, savePath string, l *Logger, um *UIManager) {
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
		fmt.Printf("Failed to get release info for version %s: %v\n", version, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("Failed to get release info for version %s: HTTP %d\n", version, resp.StatusCode)
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

	if _, err = io.Copy(file, resp.Body); err != nil {
		fmt.Printf("Failed to write zip file: %v\n", err)
		os.Remove(zipPath)
		return false
	}

	if err := extractZip(zipPath, downloadDir); err != nil {
		fmt.Printf("Failed to extract zip: %v\n", err)
		os.Remove(zipPath)
		return false
	}

	fmt.Printf("Download and extraction completed: %s\n", tagName)
	os.Remove(zipPath)
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

func extractZip(zipPath, destDir string) error {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-Command", fmt.Sprintf("Expand-Archive -Path '%s' -DestinationPath '%s'", zipPath, destDir))
	} else {
		cmd = exec.Command("unzip", "-q", zipPath, "-d", destDir)
	}
	return cmd.Run()
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

func main() {
	configPath := resolveConfigPath()
	logPath := resolveLogPath(configPath)
	bootstrapFileLog := shouldLogToFile(configPath)
	if bootstrapFileLog {
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
		message := fmt.Sprintf("Startup failed while loading config: %v", err)
		if bootstrapFileLog {
			appendToLogFile(logPath, message)
		}
		fmt.Fprintln(os.Stderr, message)
		os.Exit(1)
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
