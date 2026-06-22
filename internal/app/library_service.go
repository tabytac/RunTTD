package app

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// IsLatestVersion reports whether a profile version string means "track the
// newest" rather than a pinned tag. It covers every alias the launch path and
// profile editor accept, case-insensitively: "", "latest", and the
// stable/testing forms (dashed and parenthesized).
func IsLatestVersion(version string) bool {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "", "latest", "latest-stable", "latest-testing", "latest (stable)", "latest (testing)":
		return true
	default:
		return false
	}
}

// highestInstalledByClient returns, per client, the HIGHEST-version install
// (by version, not mod-time), restricted to the configured platform so a
// launch's actual target wins over an alphabetically-earlier OS build.
func highestInstalledByClient(cfg *domain.Config) map[string]domain.InstalledVersion {
	aliases := platform.ClientPlatformAliases(cfg)
	out := map[string]domain.InstalledVersion{}
	for _, v := range platform.ScanInstalledVersions(cfg) {
		if v.Client == "" {
			continue
		}
		if !platform.FolderMatchesAnyAlias(strings.ToLower(filepath.Base(v.Path)), aliases) {
			continue
		}
		cur, ok := out[v.Client]
		if !ok || compareVersions(v.Version, cur.Version) > 0 {
			out[v.Client] = v
		}
	}
	return out
}

// HighestInstalledFolder returns the folder path of the highest-version install
// for a client (platform-filtered), or "" if none is installed. This is how a
// "latest" profile resolves locally — by version, matching an online launch and
// the library view; do NOT use newest-by-mod-time, which a later re-download of
// an older version would wrongly win.
func HighestInstalledFolder(cfg *domain.Config, client string) string {
	if v, ok := highestInstalledByClient(cfg)[client]; ok {
		return v.Path
	}
	return ""
}

// BuildLibrary scans installed versions and annotates each with the profiles
// that would launch it; an empty ReferencedBy marks an unused folder. A "latest"
// profile resolves to the HIGHEST-version install for its client (by version,
// not mod-time), matching an online launch and staying offline-safe.
// ctx is reserved for a future network-backed build.
func BuildLibrary(ctx context.Context, cfg *domain.Config) []domain.LibraryEntry {
	scanned := platform.ScanInstalledVersions(cfg)

	latestByClient := highestInstalledByClient(cfg)

	refs := map[string][]string{} // folder path -> profile names
	for _, p := range cfg.Profiles {
		client := strings.TrimSpace(p.Client)
		if client == "" {
			client = strings.TrimSpace(cfg.DefaultClient)
			if client == "" {
				client = "jgrpp"
			}
		}
		if client == "custom" {
			continue
		}
		version := strings.TrimSpace(p.Version)
		var folder string
		if IsLatestVersion(version) {
			if v, ok := latestByClient[client]; ok {
				folder = v.Path
			}
		} else {
			folder = platform.FindVersionFolderClient(platform.ClientDownloadDir(cfg, client), version, client, cfg)
		}
		if folder != "" {
			refs[folder] = append(refs[folder], p.Name)
		}
	}

	out := make([]domain.LibraryEntry, 0, len(scanned))
	for _, v := range scanned {
		out = append(out, domain.LibraryEntry{
			InstalledVersion: v,
			ReferencedBy:     refs[v.Path],
		})
	}
	return out
}

// LibraryGroup is a client's entries for display, pre-sorted version-desc.
type LibraryGroup struct {
	Client  string
	Entries []domain.LibraryEntry
}

var libraryGroupOrder = []string{"jgrpp", "vanilla", "vanilla-nightly", ""}

// GroupLibrary partitions entries by client into libraryGroupOrder, each group
// sorted by version descending, then within a version by OS: this machine's build
// first, other platforms next, dedicated (headless) servers last.
func GroupLibrary(entries []domain.LibraryEntry, cfg *domain.Config) []LibraryGroup {
	currentOS := platform.ClientPlatformAliases(cfg)[0]
	byClient := map[string][]domain.LibraryEntry{}
	for _, e := range entries {
		byClient[e.Client] = append(byClient[e.Client], e)
	}
	var groups []LibraryGroup
	for _, client := range libraryGroupOrder {
		es := byClient[client]
		if len(es) == 0 {
			continue
		}
		sort.SliceStable(es, func(i, j int) bool {
			if c := compareVersions(es[i].Version, es[j].Version); c != 0 {
				return c > 0
			}
			return osSortRank(es[i].OSTag, currentOS) < osSortRank(es[j].OSTag, currentOS)
		})
		groups = append(groups, LibraryGroup{Client: client, Entries: es})
	}
	return groups
}

// osSortRank orders same-version builds: current platform, then other platforms,
// then dedicated server builds.
func osSortRank(osTag, currentOS string) int {
	switch {
	case strings.Contains(osTag, "dedicated"):
		return 2
	case osTag == currentOS:
		return 0
	default:
		return 1
	}
}

// compareVersions does a light dotted-numeric compare (missing parts == 0, so
// "1.0" == "1.0.0"); a non-numeric part falls back to a string compare. Not semver.
func compareVersions(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var ai, bi int
		var ae, be error
		if i < len(pa) {
			ai, ae = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			bi, be = strconv.Atoi(pb[i])
		}
		if ae != nil || be != nil {
			return strings.Compare(a, b)
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}
	return 0
}
