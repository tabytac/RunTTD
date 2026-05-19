package domain

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config represents the top-level launcher configuration
type Config struct {
	FirstRun           bool   `json:"-"`
	ParentDir          string `json:"parentDir"`
	DocsBasePath       string `json:"docsBasePath"`
	GithubApiUrl       string `json:"githubApiUrl"`
	OSType             string `json:"osType"`
	AutoOpenLog        bool   `json:"autoOpenLog"`
	AutoCloseOnStart   bool   `json:"autoCloseOnStart"`
	Verbose            bool   `json:"verbose"`
	LogToFile          bool   `json:"logToFile"`
	SubfolderPerClient bool   `json:"subfolderPerClient"`
	ThemeVariant       string `json:"themeVariant"`
	AccentPreset       int    `json:"accentPreset"`
	DefaultClient      string `json:"defaultClient"`
	VanillaMirror      string `json:"vanillaMirror"`
	NightlyMirror      string `json:"nightlyMirror"`

	Profiles []Profile `json:"profiles"`
}

// LoadConfig reads launcher configuration from the specified JSON file path
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

// SaveConfig writes the launcher configuration to the specified JSON file path safely
func SaveConfig(filename string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tmpFile := filename + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	if err := os.Rename(tmpFile, filename); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	return nil
}
