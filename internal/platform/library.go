package platform

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
