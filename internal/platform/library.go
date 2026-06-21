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

// DirSize sums the sizes of all regular files under root, skipping unreadable entries.
func DirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
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

// managedRoots returns the distinct per-client download directories.
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

// DeleteInstalledVersion removes a folder only if it sits strictly inside a
// managed download root (never a root itself), guarding against deleting the
// parent or anything outside it.
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

// nightlyDateTokenRe captures the YYYYMMDD build date of a nightly folder name.
var nightlyDateTokenRe = regexp.MustCompile(`^openttd-((?:19|20)\d{6})`)

// osTagRe matches the trailing OS/arch tag. The linux branch covers any variant
// (generic, dedicated, debian-*, ubuntu-*) ending in a known arch, not just generic.
var osTagRe = regexp.MustCompile(`(?i)-(windows-(?:win64|win32|arm64)|linux-[a-z0-9-]+-(?:amd64|arm64|i386)|macos-universal)$`)

// classifyClientFolder returns the client a folder belongs to, or "" if unrecognized.
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

// ScanInstalledVersions returns one InstalledVersion per immediate subfolder of
// the managed download directories, classified by client (Client == "" if none).
func ScanInstalledVersions(cfg *domain.Config) []domain.InstalledVersion {
	var out []domain.InstalledVersion
	seen := map[string]bool{} // guard against a path appearing under two roots
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

// parseVersionFromName extracts a version token for display/sort: a nightly's
// YYYYMMDD build date formatted YYYY-MM-DD, else the dotted version (e.g. 14.1).
func parseVersionFromName(name string) string {
	if m := nightlyDateTokenRe.FindStringSubmatch(strings.ToLower(name)); len(m) == 2 {
		d := m[1]
		return d[0:4] + "-" + d[4:6] + "-" + d[6:8]
	}
	return versionRe.FindString(name)
}

// parseOSTag returns the trailing OS/arch tag (lowercased), or "" if absent.
func parseOSTag(name string) string {
	m := osTagRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return strings.ToLower(m[1])
	}
	return ""
}

// linuxFileManagers are tried in order when xdg-open has no directory handler
// (e.g. under WSLg, which ships no desktop session or file manager).
var linuxFileManagers = []string{"nautilus", "dolphin", "thunar", "nemo", "pcmanfm", "caja"}

// RevealInFileManager opens the given folder in the OS file manager.
//
// Must NOT set the no-window SysProcAttr: HideWindow/CREATE_NO_WINDOW would
// suppress the very Explorer window we want (Start() then returns nil and the
// click silently does nothing).
func RevealInFileManager(path string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return revealLinux(path)
	}
}

// revealLinux Run()s xdg-open (so a non-zero "no handler" exit is detected, not
// mistaken for success) and falls back through linuxFileManagers, returning an
// error if nothing worked.
func revealLinux(path string) error {
	if _, err := exec.LookPath("xdg-open"); err == nil {
		if err := exec.Command("xdg-open", path).Run(); err == nil {
			return nil
		}
	}
	for _, fm := range linuxFileManagers {
		if _, err := exec.LookPath(fm); err != nil {
			continue
		}
		if err := exec.Command(fm, path).Start(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no file manager available to open %q (tried xdg-open and %s); "+
		"under WSL no file manager is installed", path, strings.Join(linuxFileManagers, ", "))
}
