package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/mod/semver"

	"runttd/internal/domain"
)

// runttdReleasesURL is the GitHub API endpoint for RunTTD's own latest release.
const runttdReleasesURL = "https://api.github.com/repos/tabytac/RunTTD/releases/latest"

// LatestRunTTDRelease returns the latest RunTTD release tag and its release-page
// URL (html_url). Any transport, status, or decode error is returned.
func LatestRunTTDRelease(ctx context.Context) (tag string, htmlURL string, err error) {
	return latestReleaseFrom(ctx, runttdReleasesURL)
}

// latestReleaseFrom is LatestRunTTDRelease against an explicit URL (for testing).
func latestReleaseFrom(ctx context.Context, url string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("failed to get latest release: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to get latest release: HTTP %d", resp.StatusCode)
	}
	var info domain.ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", fmt.Errorf("failed to parse latest release: %w", err)
	}
	return info.TagName, info.HTMLURL, nil
}

// IsNewerVersion reports whether latest is a strictly greater semantic version
// than current. A non-semver current (e.g. "dev") or latest returns false, so
// dev builds and unparseable tags never prompt for an update.
func IsNewerVersion(current, latest string) bool {
	cur := normalizeSemver(current)
	lat := normalizeSemver(latest)
	if !semver.IsValid(cur) || !semver.IsValid(lat) {
		return false
	}
	return semver.Compare(cur, lat) < 0
}

// normalizeSemver ensures a leading "v" so golang.org/x/mod/semver accepts it.
func normalizeSemver(v string) string {
	v = strings.TrimSpace(v)
	// Return "" untouched so we never produce the bare "v" token (which would
	// then read as a near-valid prefix); callers treat "" as invalid → no prompt.
	if v == "" {
		return ""
	}
	if !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}
