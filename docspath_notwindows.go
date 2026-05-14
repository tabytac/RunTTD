//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// getDocumentsDir returns the user's documents-equivalent folder.
// macOS: ~/Documents
// Linux: ~/.local/share
func getDocumentsDir() string {
	homeDir, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(homeDir, "Documents")
	}
	// Linux / other Unix
	return filepath.Join(homeDir, ".local", "share")
}
