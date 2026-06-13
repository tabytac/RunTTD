package app

import (
	"context"
	"strings"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// BuildLibrary scans installed versions and annotates each with the profiles
// that resolve to it. "Referenced" is computed by inverting the same resolution
// a launch uses, so an entry with empty ReferencedBy is genuinely unused.
func BuildLibrary(ctx context.Context, cfg *domain.Config) []domain.LibraryEntry {
	scanned := platform.ScanInstalledVersions(cfg)

	refs := map[string][]string{} // folder path -> profile names
	for _, p := range cfg.Profiles {
		client := strings.TrimSpace(p.Client)
		if client == "" {
			client = "jgrpp"
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
