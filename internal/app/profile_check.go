package app

import (
	"os"
	"strings"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// ProfileSetupIssue reports why launching p would silently miss its configured
// intent, or "" when it wouldn't. Resolution mirrors StartOpenTTD via the
// shared platform helpers. It does blocking disk I/O on the profile's save/config
// paths, which may be absolute and on a slow or unreachable network share, so
// callers must run it off the UI thread (see the fyne setupIssueCache).
func ProfileSetupIssue(p domain.Profile, docsBasePath string) string {
	switch p.LaunchMode {
	case "file", "folder":
		if p.SavePath != "" {
			gamePath := platform.ResolveProfileSavePath(docsBasePath, p.SavePath)
			if info, err := os.Stat(gamePath); err != nil {
				return "Save path not found"
			} else if info.IsDir() && platform.FindLatestSaveFile(gamePath, p.AutoLatestFilter, p.SaveSearchSubfolders) == "" {
				return "No matching saves in the save folder"
			}
		}
	case "multiplayer":
		if strings.TrimSpace(p.ServerIpPort) == "" {
			return "No server address set"
		}
	}
	if cfg := platform.ResolveProfileConfigOverride(docsBasePath, p.ConfigFilePath); cfg != "" {
		if info, err := os.Stat(cfg); err != nil {
			return "Config file override not found"
		} else if info.IsDir() {
			return "Config override is a folder, not a file"
		}
	}
	return ""
}
