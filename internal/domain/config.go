package domain

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Official endpoints. A config may override these (custom mirrors), but only
// with an https URL — see sanitizeURLs.
const (
	DefaultJgrppApiUrl   = "https://api.github.com/repos/JGRennison/OpenTTD-patches"
	DefaultVanillaMirror = "https://cdn.openttd.org/openttd-releases/"
	DefaultNightlyMirror = "https://cdn.openttd.org/openttd-nightlies/"
)

// Window size bounds. The min floor rejects an unset (0) or implausibly small
// saved size — e.g. a value mis-captured during teardown — so the next launch
// falls back to a usable default rather than an unusable sliver.
const (
	DefaultWindowWidth  = 940
	DefaultWindowHeight = 860
	minWindowWidth      = 640
	minWindowHeight     = 480
)

// Config represents the top-level launcher configuration
type Config struct {
	FirstRun           bool   `json:"-"`
	ParentDir          string `json:"parentDir"`
	DocsBasePath       string `json:"docsBasePath"`
	JgrppApiUrl        string `json:"jgrppApiUrl"`
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
	AutoLaunchProfile  string `json:"autoLaunchProfile"`
	WindowWidth        int    `json:"windowWidth"`
	WindowHeight       int    `json:"windowHeight"`

	Profiles []Profile `json:"profiles"`
}

// WindowSize returns the saved main-window size, or the default when unset or
// below the minimum (the floor guards against a garbage value persisting an
// unusable window).
func (c *Config) WindowSize() (w, h float32) {
	if c.WindowWidth < minWindowWidth || c.WindowHeight < minWindowHeight {
		return DefaultWindowWidth, DefaultWindowHeight
	}
	return float32(c.WindowWidth), float32(c.WindowHeight)
}

// ConfigParseError marks a config file that exists but isn't valid JSON, so the
// caller can recover (back it up + reset) instead of treating it like any load
// failure. Distinct from a read error, which wraps os.ErrNotExist.
type ConfigParseError struct{ err error }

func (e *ConfigParseError) Error() string { return "failed to parse config file: " + e.err.Error() }
func (e *ConfigParseError) Unwrap() error { return e.err }

// LoadConfig reads launcher configuration from the specified JSON file path
func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, &ConfigParseError{err: err}
	}

	if len(config.Profiles) == 0 {
		config.Profiles = []Profile{{Name: "Default", Version: "latest"}}
	}

	config.sanitizeURLs()
	return &config, nil
}

// sanitizeURLs forces the mirror/API endpoints back to their official defaults
// unless overridden with a valid https URL. This stops a shared config from
// silently downgrading downloads to http or redirecting them to another host.
func (c *Config) sanitizeURLs() {
	c.JgrppApiUrl = httpsOrDefault(c.JgrppApiUrl, DefaultJgrppApiUrl)
	c.VanillaMirror = httpsOrDefault(c.VanillaMirror, DefaultVanillaMirror)
	c.NightlyMirror = httpsOrDefault(c.NightlyMirror, DefaultNightlyMirror)
}

func httpsOrDefault(raw, fallback string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" {
		return fallback
	}
	return raw
}

// SaveConfig writes the launcher configuration to the specified JSON file path safely
func SaveConfig(filename string, config *Config) error {
	config.sanitizeURLs()
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
