package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// ProfileShortcutArgs is the command line a profile shortcut launches with.
// Headless is the whole point of a shortcut: it starts the game and gets out of
// the way, with no launcher window. --wait is deliberately absent, since nothing
// waits on a double-clicked shortcut.
func ProfileShortcutArgs(profileName string) []string {
	return []string{"--headless", "--profile", profileName}
}

// GenerateProfileShortcut writes a desktop shortcut for one profile beside the
// running executable and returns the file it wrote. title is what the shortcut
// is called; an empty one falls back to the profile's own name.
func GenerateProfileShortcut(profile domain.Profile, title string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not locate the RunTTD executable: %w", err)
	}
	// Resolve so a symlinked launcher records the real target, which is what the
	// shortcut has to point at once the link is gone.
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	exeDir := filepath.Dir(exePath)

	name := strings.TrimSpace(title)
	if name == "" {
		name = profile.Name
	}
	return platform.CreateShortcut(exeDir, platform.ShortcutSpec{
		Name:    name,
		ExePath: exePath,
		Args:    ProfileShortcutArgs(profile.Name),
		WorkDir: exeDir,
		// The one fact the shortcut's own name can lose, since the title is free text.
		Description: "RunTTD profile: " + profile.Name,
	})
}
