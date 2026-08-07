//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreateShortcut writes a double-clickable .command script into destDir and
// returns the file it wrote.
//
// A .command opens Terminal alongside the game, which an .app bundle would not;
// a bundle is a directory of its own with a plist and a binary stub, which is a
// heavier thing to generate and to keep valid than this feature warrants.
func CreateShortcut(destDir string, spec ShortcutSpec) (string, error) {
	stem, err := validateShortcutSpec(destDir, spec)
	if err != nil {
		return "", err
	}
	path := filepath.Join(destDir, stem+".command")

	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	if spec.Description != "" {
		b.WriteString("# " + strings.ReplaceAll(spec.Description, "\n", " ") + "\n")
	}
	if spec.WorkDir != "" {
		b.WriteString("cd " + posixQuote(spec.WorkDir) + " || exit 1\n")
	}
	b.WriteString("exec " + posixQuote(spec.ExePath))
	for _, arg := range spec.Args {
		b.WriteString(" " + posixQuote(arg))
	}
	b.WriteString("\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		return "", fmt.Errorf("could not write %s: %w", path, err)
	}
	return path, nil
}

// posixQuote wraps s in single quotes, which the shell takes literally, ending
// and reopening them around any single quote in s itself.
func posixQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
