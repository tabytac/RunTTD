package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Engine defines a pluggable engine adapter
type Engine interface {
	ID() string
	DisplayName() string
	FetchVersions(cfg *Config) ([]string, error)
	Latest(cfg *Config) (string, error)
	DownloadAndExtract(version string, cfg *Config) (bool, error)
	FindInstalled(parentDir, version string, cfg *Config) string
}

var engineRegistry = map[string]Engine{}

func RegisterEngine(e Engine) {
	engineRegistry[e.ID()] = e
}

func GetEngine(id string) Engine {
	return engineRegistry[id]
}

// jgrppEngine wraps existing GitHub-based helpers
type jgrppEngine struct{}

func (j *jgrppEngine) ID() string          { return "jgrpp" }
func (j *jgrppEngine) DisplayName() string { return "JGRPP" }
func (j *jgrppEngine) FetchVersions(cfg *Config) ([]string, error) {
	return FetchAvailableVersions(cfg)
}
func (j *jgrppEngine) Latest(cfg *Config) (string, error) { return CheckForNewVersion(cfg), nil }
func (j *jgrppEngine) DownloadAndExtract(version string, cfg *Config) (bool, error) {
	return DownloadAndExtractVersion(version, cfg), nil
}
func (j *jgrppEngine) FindInstalled(parentDir, version string, cfg *Config) string {
	return FindVersionFolderEngine(parentDir, version, "jgrpp", cfg)
}

// vanillaEngine uses CDN mirrors
type vanillaEngine struct{ nightly bool }

func (v *vanillaEngine) ID() string {
	if v.nightly {
		return "vanilla-nightly"
	}
	return "vanilla"
}
func (v *vanillaEngine) DisplayName() string {
	if v.nightly {
		return "OpenTTD (Nightly)"
	}
	return "OpenTTD (Stable)"
}
func (v *vanillaEngine) FetchVersions(cfg *Config) ([]string, error) {
	return FetchAvailableVersionsForEngine(v.ID(), cfg)
}
func (v *vanillaEngine) Latest(cfg *Config) (string, error) {
	latest := CheckForNewVersionForEngine(v.ID(), cfg)
	return latest, nil
}
func (v *vanillaEngine) DownloadAndExtract(version string, cfg *Config) (bool, error) {
	ok := DownloadAndExtractVersionForEngine(version, v.ID(), cfg)
	return ok, nil
}
func (v *vanillaEngine) FindInstalled(parentDir, version string, cfg *Config) string {
	return FindVersionFolderEngine(parentDir, version, v.ID(), cfg)
}

// helper: try to fetch a remote sha256 checksum and compare it with a local file
func verifyRemoteSHA256(localPath, remoteURL string) (bool, error) {
	resp, err := http.Get(remoteURL)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("checksum not found: %s (http %d)", remoteURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	// checksum may be in form "<sha>  filename" or just the hex
	fields := strings.Fields(string(body))
	if len(fields) == 0 {
		return false, fmt.Errorf("empty checksum")
	}
	hexsum := fields[0]

	f, err := os.Open(localPath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	ours := hex.EncodeToString(h.Sum(nil))
	return strings.EqualFold(ours, hexsum), nil
}

// initialize built-in engines
func init() {
	RegisterEngine(&jgrppEngine{})
	RegisterEngine(&vanillaEngine{nightly: false})
	RegisterEngine(&vanillaEngine{nightly: true})
}
