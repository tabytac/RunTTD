//go:build !windows

package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetDocumentsDir returns the user's documents-equivalent folder
// macOS: ~/Documents
// Linux: ~/.local/share
func GetDocumentsDir() string {
	homeDir, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(homeDir, "Documents")
	}
	// Linux / other Unix
	return filepath.Join(homeDir, ".local", "share")
}
