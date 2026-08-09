package platform

import (
	"context"
	"fmt"
	"io"
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

// parseHrefFolders returns first-seen, de-duplicated capture-group-1 values; clean is a nil-safe per-capture transform (nil = identity) and "" results are skipped.
func parseHrefFolders(html string, re *regexp.Regexp, clean func(string) string) []string {
	set := map[string]bool{}
	out := []string{}
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		if len(m) < 2 {
			continue
		}
		tag := m[1]
		if clean != nil {
			tag = clean(tag)
		}
		if tag == "" || set[tag] {
			continue
		}
		set[tag] = true
		out = append(out, tag)
	}
	return out
}

// ParseCDNVersionFoldersFromHTML extracts version numbers from the scraped HTML folder index of official OpenTTD CDN stable mirrors
func ParseCDNVersionFoldersFromHTML(html string) []string {
	return parseHrefFolders(html, cdnVersionFolderRe, strings.TrimSpace)
}

// ParseCDNYearFoldersFromHTML extracts the year folder listings from the CDN root index HTML
func ParseCDNYearFoldersFromHTML(html string) []string {
	return parseHrefFolders(html, cdnYearFolderRe, nil)
}

// ParseNightlyBuildFoldersFromHTML parses HTML directory contents for nightly timestamp builds
func ParseNightlyBuildFoldersFromHTML(html string) []string {
	return parseHrefFolders(html, cdnNightlyBuildRe, strings.TrimSpace)
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

// preferredExtsForOS lists acceptable archive extensions, most-preferred first.
// Old folders need several (Linux gained .tar.xz only at 1.4.0; macOS used .dmg
// then .zip). .exe/.pdb/.deb/source/docs are intentionally absent.
func preferredExtsForOS(osType string) []string {
	switch {
	case strings.Contains(osType, "linux"):
		return []string{".tar.xz", ".tar.bz2", ".tar.gz", ".zip"}
	case strings.Contains(osType, "mac") || strings.Contains(osType, "darwin"):
		return []string{".dmg", ".zip"}
	default:
		return []string{".zip"}
	}
}

// selectManifestAsset returns the first manifest id matching an alias (outer loop,
// canonical first) and a preferred extension (inner). aliasIndex is the matched
// alias position, or -1 if nothing matched; aliasIndex > 0 means a cross-arch
// fallback was used and the caller should log a note.
func selectManifestAsset(ids, aliases, exts []string) (string, int) {
	for ai, alias := range aliases {
		alias = strings.ToLower(strings.TrimSpace(alias))
		if alias == "" {
			continue
		}
		for _, ext := range exts {
			for _, id := range ids {
				lower := strings.ToLower(id)
				if strings.Contains(lower, alias) && strings.HasSuffix(lower, ext) {
					return id, ai
				}
			}
		}
	}
	return "", -1
}

// osDisplayHint returns a short human-readable OS word for skip messages.
func osDisplayHint(osType string) string {
	switch {
	case strings.Contains(osType, "linux"):
		return "Linux"
	case strings.Contains(osType, "mac") || strings.Contains(osType, "darwin"):
		return "macOS"
	default:
		return "Windows"
	}
}

// FetchNightlyManifest downloads manifest.yaml for a specific nightly tag build
func FetchNightlyManifest(ctx context.Context, base, year, version string) (domain.NightlyManifestData, error) {
	url := fmt.Sprintf("%s/%s/%s/manifest.yaml", strings.TrimRight(base, "/"), year, version)
	resp, err := doGETWithRetry(ctx, httpClient, url)
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
	resp, err := doGETWithRetry(ctx, httpClient, url)
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

	resp, err := doGETWithRetry(ctx, httpClient, baseTrimmed+"/")
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
		yResp, err := doGETWithRetry(ctx, httpClient, yearURL)
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
		resp, err := doGETWithRetry(ctx, httpClient, base)
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
	return CheckForNewVersionForClientTrack(ctx, client, cfg, "stable", nil)
}

// CheckForNewVersionForClientTrack returns version filtered by track (stable / testing).
// logger (nil-safe) receives the reason when the lookup fails.
func CheckForNewVersionForClientTrack(ctx context.Context, client string, cfg *domain.Config, track string, logger *Logger) string {
	switch client {
	case "jgrpp":
		// Stable-only: /releases/latest excludes pre-releases. A testing track would branch on `track` here.
		return CheckForNewVersion(ctx, cfg, logger)
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
			if track == "stable" && IsPreReleaseVersion(lower) {
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
		logf("Could not create download folder %s: %v", downloadDir, err)
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
			osType := resolveOSType(cfg)
			id, aliasIdx := selectManifestAsset(manifest.FileIDs, platformAliases, preferredExtsForOS(osType))
			if id == "" {
				logf("No %s build exists for nightly %s; install skipped.", osDisplayHint(osType), version)
				return false // authoritative manifest: short-circuit, do not guess
			}
			if aliasIdx > 0 {
				logf("No %s build for %s; using %s (runs via emulation on this platform).",
					osType, version, platformAliases[aliasIdx])
			}
			matchedTag := canonicalOSTag(platformAliases[aliasIdx])
			url := fmt.Sprintf("%s/%s/%s/%s", baseTrimmed, nightlyYear, version, id)
			logf("Nightly selected asset: %s", url)
			archivePath := filepath.Join(downloadDir, id)
			if err := downloadAndExtractTo(ctx, downloadClient, url, archivePath, downloadDir, matchedTag, logger, progress); err != nil {
				logf("Nightly asset failed (%s): %v", url, err)
				return false
			}
			return true
		}
	}

	if client == "vanilla" {
		manifest, err := FetchReleaseManifest(ctx, baseTrimmed, version)
		if err != nil {
			logf("Release manifest fetch failed (%s); trying candidate URLs: %v", version, err)
		} else {
			osType := resolveOSType(cfg)
			id, aliasIdx := selectManifestAsset(manifest.FileIDs, platformAliases, preferredExtsForOS(osType))
			if id == "" {
				logf("No %s build exists for %s; install skipped.", osDisplayHint(osType), version)
				return false // authoritative manifest: short-circuit, do not guess
			}
			if aliasIdx > 0 {
				logf("No %s build for %s; using %s (runs via emulation on this platform).",
					osType, version, platformAliases[aliasIdx])
			}
			matchedTag := canonicalOSTag(platformAliases[aliasIdx])
			url := baseTrimmed + "/" + version + "/" + id
			logf("Selected asset: %s", url)
			archivePath := filepath.Join(downloadDir, id)
			if err := downloadAndExtractTo(ctx, downloadClient, url, archivePath, downloadDir, matchedTag, logger, progress); err != nil {
				logf("Asset download failed (%s): %v", url, err)
				return false
			}
			return true
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
			if err := downloadAndExtractTo(ctx, downloadClient, url, archivePath, downloadDir, resolveOSType(cfg), logger, progress); err != nil {
				continue
			}
			return true
		}
	}

	return false
}
