package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"runttd/internal/domain"
)

var (
	cdnVersionFolderRe = regexp.MustCompile(`href="(?:https?://[^"]*/)?([0-9][0-9A-Za-z.\-]*)/"`)
	cdnYearFolderRe    = regexp.MustCompile(`href="(?:https?://[^"]*/)?((?:19|20)\d{2})/"`)
	cdnNightlyBuildRe  = regexp.MustCompile(`href="(?:https?://[^"]*/)?([0-9]{8}-[^"]+)/"`)
)

// ParseCDNVersionFoldersFromHTML extracts version numbers from the scraped HTML folder index of official OpenTTD CDN stable mirrors
func ParseCDNVersionFoldersFromHTML(html string) []string {
	set := map[string]bool{}
	versions := []string{}

	matches := cdnVersionFolderRe.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		tag := strings.TrimSpace(m[1])
		if tag == "" {
			continue
		}
		if !set[tag] {
			set[tag] = true
			versions = append(versions, tag)
		}
	}
	return versions
}

// ParseCDNYearFoldersFromHTML extracts the year folder listings from the CDN root index HTML
func ParseCDNYearFoldersFromHTML(html string) []string {
	set := map[string]bool{}
	years := []string{}
	matches := cdnYearFolderRe.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		y := m[1]
		if !set[y] {
			set[y] = true
			years = append(years, y)
		}
	}
	return years
}

// ParseNightlyBuildFoldersFromHTML parses HTML directory contents for nightly timestamp builds
func ParseNightlyBuildFoldersFromHTML(html string) []string {
	set := map[string]bool{}
	builds := []string{}
	matches := cdnNightlyBuildRe.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		b := strings.TrimSpace(m[1])
		if b == "" {
			continue
		}
		if !set[b] {
			set[b] = true
			builds = append(builds, b)
		}
	}
	return builds
}

// ParseReleaseManifest processes a CDN manifest YAML and extracts the file IDs.
// Used for both stable releases and nightlies; both manifests share the `- id:` shape.
func ParseReleaseManifest(text string) domain.NightlyManifestData {
	out := domain.NightlyManifestData{FileIDs: []string{}}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- id:") {
			id := strings.TrimSpace(strings.TrimPrefix(line, "- id:"))
			if id != "" {
				out.FileIDs = append(out.FileIDs, id)
			}
		}
	}
	return out
}

// NightlyPlatformAliases resolves architecture patterns used for nightlies (re-mapping from ClientPlatformAliases)
func NightlyPlatformAliases(cfg *domain.Config) []string {
	return ClientPlatformAliases(cfg)
}

// NightlyIDMatchesPlatform checks whether a file identifier contains target architecture substrings
func NightlyIDMatchesPlatform(id string, aliases []string) bool {
	lower := strings.ToLower(id)
	for _, a := range aliases {
		a = strings.ToLower(strings.TrimSpace(a))
		if a != "" && strings.Contains(lower, a) {
			return true
		}
	}
	return false
}

// FetchNightlyManifest downloads manifest.yaml for a specific nightly tag build
func FetchNightlyManifest(ctx context.Context, base, year, version string) (domain.NightlyManifestData, error) {
	url := fmt.Sprintf("%s/%s/%s/manifest.yaml", strings.TrimRight(base, "/"), year, version)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return domain.NightlyManifestData{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return domain.NightlyManifestData{}, fmt.Errorf("failed to fetch nightly manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return domain.NightlyManifestData{}, fmt.Errorf("failed to fetch nightly manifest: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NightlyManifestData{}, fmt.Errorf("failed to read nightly manifest: %w", err)
	}
	return ParseReleaseManifest(string(body)), nil
}

// FetchReleaseManifest downloads manifest.yaml for a stable release. Unlike the
// nightly path, the URL has no year segment: {base}/{version}/manifest.yaml.
func FetchReleaseManifest(ctx context.Context, base, version string) (domain.NightlyManifestData, error) {
	url := fmt.Sprintf("%s/%s/manifest.yaml", strings.TrimRight(base, "/"), version)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return domain.NightlyManifestData{}, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return domain.NightlyManifestData{}, fmt.Errorf("failed to fetch release manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return domain.NightlyManifestData{}, fmt.Errorf("failed to fetch release manifest: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return domain.NightlyManifestData{}, fmt.Errorf("failed to read release manifest: %w", err)
	}
	return ParseReleaseManifest(string(body)), nil
}

// FetchRecentNightlyVersions scrapes years and retrieves up to the limit of latest nightly build folders
func FetchRecentNightlyVersions(ctx context.Context, base string, limit int) ([]string, error) {
	if limit <= 0 {
		return []string{}, nil
	}
	baseTrimmed := strings.TrimRight(base, "/")

	req, err := http.NewRequestWithContext(ctx, "GET", baseTrimmed+"/", nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nightly root index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch nightly root index: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read nightly root index: %w", err)
	}

	years := ParseCDNYearFoldersFromHTML(string(body))
	if len(years) == 0 {
		return []string{}, nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(years)))

	all := []string{}
	seen := map[string]bool{}
	for _, year := range years {
		yearURL := fmt.Sprintf("%s/%s/", baseTrimmed, year)
		yReq, err := http.NewRequestWithContext(ctx, "GET", yearURL, nil)
		if err != nil {
			continue
		}
		yResp, err := httpClient.Do(yReq)
		if err != nil {
			continue
		}
		if yResp.StatusCode != 200 {
			yResp.Body.Close()
			continue
		}
		yBody, err := io.ReadAll(yResp.Body)
		yResp.Body.Close()
		if err != nil {
			continue
		}
		builds := ParseNightlyBuildFoldersFromHTML(string(yBody))
		sort.Sort(sort.Reverse(sort.StringSlice(builds)))
		for _, b := range builds {
			if !seen[b] {
				seen[b] = true
				all = append(all, b)
				if len(all) >= limit {
					return all, nil
				}
			}
		}
	}

	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// FetchAvailableVersionsForClient gathers versions based on client configuration settings
func FetchAvailableVersionsForClient(ctx context.Context, client string, cfg *domain.Config) ([]string, error) {
	switch client {
	case "jgrpp":
		return FetchAvailableVersions(ctx, cfg)
	case "vanilla", "vanilla-nightly":
		if client == "vanilla-nightly" {
			base := cfg.NightlyMirror
			if base == "" {
				base = "https://cdn.openttd.org/openttd-nightlies/"
			}
			recent, err := FetchRecentNightlyVersions(ctx, base, 10)
			if err != nil {
				return nil, err
			}
			out := []string{"latest"}
			out = append(out, recent...)
			return out, nil
		}

		// Scrape stable CDN version folders from index
		base := cfg.VanillaMirror
		if base == "" {
			base = "https://cdn.openttd.org/openttd-releases/"
		}
		req, err := http.NewRequestWithContext(ctx, "GET", base, nil)
		if err != nil {
			return nil, err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch vanilla index: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("failed to fetch vanilla index: HTTP %d", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read vanilla index: %w", err)
		}
		versions := ParseCDNVersionFoldersFromHTML(string(body))
		out := []string{"latest (Stable)", "latest (Testing)"}
		out = append(out, versions...)
		return out, nil
	default:
		return nil, fmt.Errorf("unknown client: %s", client)
	}
}

// CheckForNewVersionForClient fetches the latest version tag name from CDN / GitHub
func CheckForNewVersionForClient(ctx context.Context, client string, cfg *domain.Config) string {
	return CheckForNewVersionForClientTrack(ctx, client, cfg, "stable")
}

// CheckForNewVersionForClientTrack returns version filtered by track (stable / testing)
func CheckForNewVersionForClientTrack(ctx context.Context, client string, cfg *domain.Config, track string) string {
	switch client {
	case "jgrpp":
		// Stable-only: /releases/latest excludes pre-releases. A testing track would branch on `track` here.
		return CheckForNewVersion(ctx, cfg)
	case "vanilla", "vanilla-nightly":
		versions, err := FetchAvailableVersionsForClient(ctx, client, cfg)
		if err != nil || len(versions) == 0 {
			return ""
		}
		for _, v := range versions {
			vv := strings.TrimSpace(v)
			if vv == "" {
				continue
			}
			lower := strings.ToLower(vv)
			if lower == "latest (stable)" || lower == "latest (testing)" || lower == "latest" {
				continue
			}
			if track == "stable" && (strings.Contains(lower, "-rc") || strings.Contains(lower, "beta")) {
				continue
			}
			return vv
		}
		return ""
	default:
		return ""
	}
}

// DownloadAndExtractVersionForClientWithLogger downloads a client package and outputs status tracking messages
func DownloadAndExtractVersionForClientWithLogger(ctx context.Context, version, client string, cfg *domain.Config, logger *Logger, progress ProgressFunc) bool {
	logf := func(format string, args ...any) {
		if logger != nil {
			logger.Append(fmt.Sprintf(format, args...))
		}
	}
	if client == "jgrpp" {
		return DownloadAndExtractVersionWithLogger(ctx, version, cfg, logger, progress)
	}

	base := cfg.VanillaMirror
	if client == "vanilla-nightly" {
		base = cfg.NightlyMirror
		if base == "" {
			base = "https://cdn.openttd.org/openttd-nightlies/"
		}
	}
	if base == "" {
		base = "https://cdn.openttd.org/openttd-releases/"
	}

	platformAliases := NightlyPlatformAliases(cfg)
	candidates := []string{}
	for _, plat := range platformAliases {
		candidates = append(candidates,
			fmt.Sprintf("openttd-%s-%s.zip", version, plat),
			fmt.Sprintf("openttd-%s-%s.tar.xz", version, plat),
			fmt.Sprintf("openttd-%s-%s.dmg", version, plat),
		)
	}
	candidates = append(candidates,
		fmt.Sprintf("openttd-%s.zip", version),
		fmt.Sprintf("openttd-%s.tar.xz", version),
		fmt.Sprintf("openttd-%s.dmg", version),
	)

	downloadDir := ClientDownloadDir(cfg, client)
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return false
	}

	baseTrimmed := strings.TrimRight(base, "/")
	nightlyYear := ""
	if client == "vanilla-nightly" {
		if len(version) >= 4 {
			nightlyYear = version[:4]
		}
	}
	if client == "vanilla-nightly" && nightlyYear != "" {
		manifest, err := FetchNightlyManifest(ctx, baseTrimmed, nightlyYear, version)
		if err != nil {
			logf("Nightly manifest fetch failed: %v", err)
		} else {
			targetExt := ".zip"
			lowerOS := resolveOSType(cfg)
			switch {
			case strings.Contains(lowerOS, "linux"):
				targetExt = ".tar.xz"
			case strings.Contains(lowerOS, "mac") || strings.Contains(lowerOS, "darwin"):
				targetExt = ".dmg"
			}
			for _, id := range manifest.FileIDs {
				if !NightlyIDMatchesPlatform(id, platformAliases) {
					continue
				}
				if !strings.HasSuffix(strings.ToLower(id), targetExt) {
					continue
				}
				url := fmt.Sprintf("%s/%s/%s/%s", baseTrimmed, nightlyYear, version, id)
				logf("Nightly selected asset: %s", url)
				archivePath := filepath.Join(downloadDir, id)

				if err := downloadAndExtractTo(ctx, downloadClient, url, archivePath, downloadDir, logger, progress); err != nil {
					logf("Nightly asset failed (%s): %v", url, err)
					continue
				}
				return true
			}
		}
	}

	for _, name := range candidates {
		urlCandidates := []string{}
		if nightlyYear != "" {
			urlCandidates = append(urlCandidates, baseTrimmed+"/"+nightlyYear+"/"+version+"/"+name)
		}
		urlCandidates = append(urlCandidates, baseTrimmed+"/"+version+"/"+name, baseTrimmed+"/"+name)
		for _, url := range urlCandidates {
			archivePath := filepath.Join(downloadDir, name)
			if err := downloadAndExtractTo(ctx, downloadClient, url, archivePath, downloadDir, logger, progress); err != nil {
				continue
			}
			return true
		}
	}

	return false
}
