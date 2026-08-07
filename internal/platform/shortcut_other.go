//go:build !windows && !darwin && !linux

package platform

import (
	"fmt"
	"runtime"
)

// CreateShortcut reports that this platform has no shortcut format RunTTD knows
// how to write. The three release targets each have their own implementation.
func CreateShortcut(destDir string, spec ShortcutSpec) (string, error) {
	return "", fmt.Errorf("creating shortcuts is not supported on %s", runtime.GOOS)
}
