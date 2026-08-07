//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreateShortcut writes an XDG .desktop entry into destDir and returns the file
// it wrote. The executable bit is set because several desktops refuse to launch
// an entry without it.
func CreateShortcut(destDir string, spec ShortcutSpec) (string, error) {
	stem, err := validateShortcutSpec(destDir, spec)
	if err != nil {
		return "", err
	}
	path := filepath.Join(destDir, stem+".desktop")

	exec := desktopExecValue(spec.ExePath, spec.Args)
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Version=1.0\n")
	b.WriteString("Name=" + desktopValue(spec.Name) + "\n")
	if spec.Description != "" {
		b.WriteString("Comment=" + desktopValue(spec.Description) + "\n")
	}
	b.WriteString("Exec=" + exec + "\n")
	if spec.WorkDir != "" {
		b.WriteString("Path=" + desktopValue(spec.WorkDir) + "\n")
	}
	b.WriteString("Icon=" + desktopValue(spec.ExePath) + "\n")
	b.WriteString("Terminal=false\n")
	b.WriteString("Categories=Game;\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		return "", fmt.Errorf("could not write %s: %w", path, err)
	}
	return path, nil
}

// desktopValue escapes the characters the desktop-entry spec reserves inside a
// plain string value, and flattens newlines so one value cannot become two keys.
func desktopValue(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", " ", "\r", " ", "\t", `\t`)
	return r.Replace(s)
}

// desktopExecValue builds the Exec line. The spec quotes arguments with double
// quotes, escapes a handful of characters inside them with a backslash, and
// reserves a literal percent as "%%" because %f, %U and friends are field codes.
func desktopExecValue(exe string, args []string) string {
	quoted := make([]string, 0, len(args)+1)
	for _, part := range append([]string{exe}, args...) {
		inner := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", `$`, `\\$`).Replace(part)
		quoted = append(quoted, `"`+inner+`"`)
	}
	return strings.ReplaceAll(strings.Join(quoted, " "), "%", "%%")
}
