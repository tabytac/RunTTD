package platform

import (
	"fmt"
	"os"
	"strings"
)

// ShortcutSpec describes one desktop shortcut. Each OS writes a different file
// format from it: a .lnk on Windows, a .command script on macOS, a .desktop
// entry on Linux.
type ShortcutSpec struct {
	Name        string   // display name, and the filename stem after sanitising
	ExePath     string   // absolute path to the launcher executable
	Args        []string // arguments handed to ExePath
	WorkDir     string   // working directory the shortcut launches in
	Description string   // tooltip or comment; may be empty
}

// shortcutNameReplacer maps every character Windows forbids in a filename, plus
// the separators the other two platforms care about, to a space.
var shortcutNameReplacer = strings.NewReplacer(
	`\`, " ", "/", " ", ":", " ", "*", " ", "?", " ", `"`, " ", "<", " ", ">", " ", "|", " ",
)

// reservedDeviceNames are the DOS device names Windows still refuses as a
// filename, whatever extension follows them.
var reservedDeviceNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// sanitiseShortcutName turns a profile name into a filename stem that is legal on
// all three platforms, collapsing the runs of spaces that replacing leaves behind.
// A reserved name is suffixed rather than rejected, so the profile still gets a
// shortcut; the same stem is produced everywhere so one profile means one name.
func sanitiseShortcutName(name string) string {
	cleaned := strings.Join(strings.Fields(shortcutNameReplacer.Replace(name)), " ")
	cleaned = strings.Trim(cleaned, " .") // Windows also rejects a trailing dot or space
	if cleaned == "" {
		return "RunTTD"
	}
	if reservedDeviceNames[strings.ToUpper(cleaned)] {
		return cleaned + " shortcut"
	}
	return cleaned
}

// validateShortcutSpec covers the checks every platform shares, so each
// implementation only handles its own file format.
func validateShortcutSpec(destDir string, spec ShortcutSpec) (string, error) {
	if strings.TrimSpace(spec.ExePath) == "" {
		return "", fmt.Errorf("no launcher executable to point the shortcut at")
	}
	if _, err := os.Stat(spec.ExePath); err != nil {
		return "", fmt.Errorf("launcher executable %s is not readable: %w", spec.ExePath, err)
	}
	if strings.TrimSpace(destDir) == "" {
		return "", fmt.Errorf("no folder to write the shortcut into")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("could not create %s: %w", destDir, err)
	}
	return sanitiseShortcutName(spec.Name), nil
}
