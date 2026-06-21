package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"runttd/internal/domain"
)

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
}

// findReleaseAsset returns the URL and name of the archive asset matching osType
// (which must be resolved, never blank), or empty strings if none match.
func findReleaseAsset(assets []domain.ReleaseAsset, osType string) (url, name string) {
	if osType == "" {
		return "", ""
	}
	for _, a := range assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, osType) && (strings.HasSuffix(n, ".zip") || strings.HasSuffix(n, ".tar.xz") || strings.HasSuffix(n, ".dmg")) {
			return a.BrowserDownloadURL, a.Name
		}
	}
	return "", ""
}

// FetchAvailableVersions fetches the most recent 20 JGRPP releases from the GitHub repository API
func FetchAvailableVersions(ctx context.Context, config *domain.Config) ([]string, error) {
	repoURL := fmt.Sprintf("%s/releases?per_page=20", config.JgrppApiUrl)

	req, err := http.NewRequestWithContext(ctx, "GET", repoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get release info: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get release info: HTTP %d", resp.StatusCode)
	}

	var releases []domain.ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse release info: %w", err)
	}

	versions := []string{"latest"}
	for _, release := range releases {
		tag := strings.TrimPrefix(release.TagName, "jgrpp-")
		versions = append(versions, tag)
	}
	return versions, nil
}

// CheckForNewVersion fetches the latest JGRPP release tag and returns its formatted version suffix
func CheckForNewVersion(ctx context.Context, config *domain.Config) string {
	repoURL := fmt.Sprintf("%s/releases/latest", config.JgrppApiUrl)

	req, err := http.NewRequestWithContext(ctx, "GET", repoURL, nil)
	if err != nil {
		return ""
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("Failed to get latest release info: %v\n", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Printf("Failed to get latest release info: HTTP %d\n", resp.StatusCode)
		return ""
	}

	var releaseInfo domain.ReleaseInfo
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

// DownloadAndExtractVersionWithLogger downloads a specific JGRPP version archive
// and extracts it to the download directory, routing extractor output to the logger
func DownloadAndExtractVersionWithLogger(ctx context.Context, version string, config *domain.Config, logger *Logger, progress ProgressFunc) bool {
	logf := func(format string, args ...any) {
		if logger != nil {
			logger.Append(fmt.Sprintf(format, args...))
		}
	}

	repoURL := fmt.Sprintf("%s/releases/tags/jgrpp-%s", config.JgrppApiUrl, version)
	downloadDir := ClientDownloadDir(config, "jgrpp")

	resp, err := doGETWithRetry(ctx, httpClient, repoURL)
	if err != nil {
		logf("Failed to look up JGRPP release %s: %v", version, err)
		return false
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		logf("Failed to look up JGRPP release %s: HTTP %d", version, resp.StatusCode)
		return false
	}

	var releaseInfo domain.ReleaseInfo
	decodeErr := json.NewDecoder(resp.Body).Decode(&releaseInfo)
	resp.Body.Close()
	if decodeErr != nil {
		logf("Failed to parse JGRPP release info: %v", decodeErr)
		return false
	}

	osType := resolveOSType(config)
	tagName := releaseInfo.TagName
	extractedFolder := fmt.Sprintf("openttd-%s-%s", tagName, osType)
	if _, err := os.Stat(filepath.Join(downloadDir, extractedFolder)); err == nil {
		return true
	}

	downloadURL, assetName := findReleaseAsset(releaseInfo.Assets, osType)
	if downloadURL == "" {
		logf("No downloadable asset found for %s (OS type %s)", tagName, osType)
		return false
	}

	archivePath := filepath.Join(downloadDir, assetName)
	logf("Downloading version: %s", tagName)

	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		logf("Failed to create download directory %s: %v", downloadDir, err)
		return false
	}

	if err := downloadAndExtractTo(ctx, downloadClient, downloadURL, archivePath, downloadDir, logger, progress); err != nil {
		logf("Failed to install %s: %v", assetName, err)
		return false
	}
	return true
}
