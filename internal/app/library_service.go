package app

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// BuildLibrary scans installed versions and annotates each with the profiles
// that resolve to it. "Referenced" is computed by inverting the same resolution
// a launch uses, so an entry with empty ReferencedBy is genuinely unused.
// ctx is reserved for a future network-backed build; current resolution is local/synchronous.
func BuildLibrary(ctx context.Context, cfg *domain.Config) []domain.LibraryEntry {
	scanned := platform.ScanInstalledVersions(cfg)

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
		dir := platform.ClientDownloadDir(cfg, client)
		version := strings.TrimSpace(p.Version)
		lower := strings.ToLower(version)
		var folder string
		switch lower {
		case "", "latest", "latest-stable", "latest-testing", "latest (stable)", "latest (testing)":
			folder = platform.FindLatestFolderClientWithConfig(dir, client, cfg)
		default:
			folder = platform.FindVersionFolderClient(dir, version, client, cfg)
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

// GroupLibrary partitions entries by client into the fixed display order, with
// each group's entries sorted by version number descending (newest first).
func GroupLibrary(entries []domain.LibraryEntry) []LibraryGroup {
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
			return compareVersions(es[i].Version, es[j].Version) > 0
		})
		groups = append(groups, LibraryGroup{Client: client, Entries: es})
	}
	return groups
}

// compareVersions does a light numeric dotted compare. Missing components are
// treated as 0 ("1.0" == "1.0.0"). A non-numeric component falls back to a
// string compare of the whole version. Not a full semver implementation.
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
