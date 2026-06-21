package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
func DownloadAndExtractVersionWithLogger(ctx context.Context, version string, config *domain.Config, logger *Logger) bool {
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
		logf("No downloadable asset found for %s (OS type %s)", tagName, config.OSType)
		return false
	}

	archivePath := filepath.Join(downloadDir, assetName)
	logf("Downloading version: %s", tagName)

	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		logf("Failed to create download directory %s: %v", downloadDir, err)
		return false
	}

	dlCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	resp, err = doGETWithRetry(dlCtx, downloadClient, downloadURL)
	if err != nil {
		logf("Failed to download %s: %v", assetName, err)
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		logf("Failed to download %s: HTTP %d", assetName, resp.StatusCode)
		return false
	}

	file, err := os.Create(archivePath)
	if err != nil {
		logf("Failed to create %s: %v", archivePath, err)
		return false
	}

	if _, err = io.Copy(file, resp.Body); err != nil {
		file.Close()
		os.Remove(archivePath)
		logf("Failed to write %s: %v", assetName, err)
		return false
	}
	file.Close()

	if err := ExtractArchive(archivePath, downloadDir, logger); err != nil {
		os.Remove(archivePath)
		logf("Failed to extract %s: %v", assetName, err)
		return false
	}

	os.Remove(archivePath)
	return true
}
