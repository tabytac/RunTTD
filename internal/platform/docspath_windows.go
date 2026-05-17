//go:build windows

package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetDocumentsDir returns the user's actual Documents folder on Windows, respecting any folder redirection
func GetDocumentsDir() string {
	cmd := exec.Command("powershell", "-NoProfile", "-Command", "[Environment]::GetFolderPath('MyDocuments')")
	out, err := cmd.Output()
	if err == nil {
		dir := strings.TrimSpace(string(out))
		if dir != "" {
			return dir
		}
	}
	// Fallback
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "Documents")
}
