package platform

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"runttd/internal/domain"
)

// DirSize returns the total size in bytes of all regular files under root.
// Unreadable entries are skipped rather than aborting the walk.
func DirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries, keep walking
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}

// managedRoots returns the distinct download directories that may contain
// installed client folders, honoring the per-client subfolder layout.
func managedRoots(cfg *domain.Config) []string {
	if cfg == nil {
		return nil
	}
	seen := map[string]bool{}
	var roots []string
	for _, client := range []string{"jgrpp", "vanilla", "vanilla-nightly"} {
		dir := ClientDownloadDir(cfg, client)
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		roots = append(roots, dir)
	}
	return roots
}

// DeleteInstalledVersion removes an installed version folder. It refuses any
// path that is not strictly inside one of the managed download roots (and never
// a root itself), so it can never delete the parent directory or anything
// outside it.
func DeleteInstalledVersion(cfg *domain.Config, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	for _, root := range managedRoots(cfg) {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(rootAbs, abs)
		if err != nil {
			continue
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return os.RemoveAll(abs)
	}
	return fmt.Errorf("refusing to delete %q: not inside a managed download directory", abs)
}

var nightlyDateRe = regexp.MustCompile(`^openttd-(?:19|20)\d{6}`)
var versionRe = regexp.MustCompile(`(\d+\.\d+(?:\.\d+)?)`)

// nightlyDateTokenRe captures the YYYYMMDD build date of a nightly folder name
// (the first token after "openttd-").
var nightlyDateTokenRe = regexp.MustCompile(`^openttd-((?:19|20)\d{6})`)

// osTagRe matches the trailing OS/arch tag of an OpenTTD release folder name.
// Anchored to the end so it picks up the suffix regardless of the version/hash
// block in front of it (e.g. "...-master-g<hash>-windows-arm64").
var osTagRe = regexp.MustCompile(`(?i)-(windows-(?:win64|win32|arm64)|linux-generic-(?:amd64|arm64|i386)|macos-universal)$`)

// classifyClientFolder returns the client a folder belongs to, or "" if it does
// not look like a managed client install.
func classifyClientFolder(name string) string {
	lname := strings.ToLower(name)
	if strings.Contains(lname, "jgrpp") {
		return "jgrpp"
	}
	if strings.Contains(lname, "openttd") {
		if nightlyDateRe.MatchString(lname) {
			return "vanilla-nightly"
		}
		return "vanilla"
	}
	return ""
}

// ScanInstalledVersions walks the managed download directories and returns one
// InstalledVersion per immediate subfolder, classified by client. Folders that
// match no client are returned with Client == "". Size and mod-time are read
// per folder; an unreadable size is reported as 0.
func ScanInstalledVersions(cfg *domain.Config) []domain.InstalledVersion {
	var out []domain.InstalledVersion
	seen := map[string]bool{} // belt-and-suspenders: guard against a path appearing under two roots
	for _, root := range managedRoots(cfg) {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			info, err := entry.Info()
			var modTime time.Time
			if err == nil {
				modTime = info.ModTime()
			}
			size, _ := DirSize(path)
			out = append(out, domain.InstalledVersion{
				Path:      path,
				Client:    classifyClientFolder(entry.Name()),
				Version:   parseVersionFromName(entry.Name()),
				OSTag:     parseOSTag(entry.Name()),
				SizeBytes: size,
				ModTime:   modTime,
			})
		}
	}
	return out
}

// parseVersionFromName makes a best-effort extraction of a version token from a
// folder name. Used for display and for sort order within a client group. For
// nightly folders (openttd-YYYYMMDD-master-...), it returns the build date
// formatted as YYYY-MM-DD; otherwise it returns the dotted version (e.g. 14.1).
func parseVersionFromName(name string) string {
	if m := nightlyDateTokenRe.FindStringSubmatch(strings.ToLower(name)); len(m) == 2 {
		d := m[1] // YYYYMMDD
		return d[0:4] + "-" + d[4:6] + "-" + d[6:8]
	}
	return versionRe.FindString(name)
}

// parseOSTag returns the trailing OS/arch tag of a release folder name (e.g.
// "windows-win64", "macos-universal"), lowercased, or "" if none is present.
func parseOSTag(name string) string {
	m := osTagRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return strings.ToLower(m[1])
	}
	return ""
}

// RevealInFileManager opens the given folder in the OS file manager.
//
// Note: unlike the archive-extraction helpers, this must NOT set the no-window
// SysProcAttr. explorer/open/xdg-open are GUI launchers, and HideWindow /
// CREATE_NO_WINDOW suppresses the very File Explorer window we want to show (the
// launcher still exits cleanly, so Start() returns nil and the click silently
// does nothing). These launchers don't spawn a console, so there's nothing to hide.
func RevealInFileManager(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}
