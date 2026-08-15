package fyne

import (
	"fmt"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
)

func centeredLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Alignment = fyne.TextAlignCenter
	l.Wrapping = fyne.TextWrapWord
	return l
}

func mutedCenteredLabel(text string) *widget.Label {
	l := centeredLabel(text)
	l.Importance = widget.LowImportance
	return l
}

func mutedLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Importance = widget.LowImportance
	return l
}

// versionCaption formats the app version for display; blank/whitespace builds read "dev".
func versionCaption(version string) string {
	if v := strings.TrimSpace(version); v != "" {
		return v
	}
	return "dev"
}

func indexOfProfileByName(profiles []domain.Profile, name string) int {
	needle := strings.TrimSpace(name)
	if needle == "" {
		return -1
	}
	for i, p := range profiles {
		if strings.EqualFold(strings.TrimSpace(p.Name), needle) {
			return i
		}
	}
	return -1
}

// reorderProfiles moves the profile at from to Fyne's drag-list destination to
// (draggedTo: insert before whatever currently sits there) and returns a new
// slice; profiles is left untouched. Callers re-resolve any selection by name
// afterward (indexOfProfileByName), since the move can shift other indices too.
func reorderProfiles(profiles []domain.Profile, from, to int) []domain.Profile {
	if to > from {
		to--
	}
	profile := profiles[from]
	rest := make([]domain.Profile, 0, len(profiles)-1)
	rest = append(rest, profiles[:from]...)
	rest = append(rest, profiles[from+1:]...)

	out := make([]domain.Profile, 0, len(profiles))
	out = append(out, rest[:to]...)
	out = append(out, profile)
	out = append(out, rest[to:]...)
	return out
}

// restoredProfileName keeps a restored profile's name unique, which selection,
// auto-launch and the startup marker all resolve by. A new profile can have taken
// the name while the undo offer was up, and the editor forbids duplicates.
func restoredProfileName(profiles []domain.Profile, base string) string {
	if indexOfProfileByName(profiles, base) < 0 {
		return base
	}
	candidate := base + " (restored)"
	for n := 2; indexOfProfileByName(profiles, candidate) >= 0; n++ {
		candidate = fmt.Sprintf("%s (restored %d)", base, n)
	}
	return candidate
}

// uniqueProfileName returns "base Copy", escalating to "base Copy (2)", "(3)"... on case-insensitive collision.
func uniqueProfileName(profiles []domain.Profile, base string) string {
	candidate := base + " Copy"
	for n := 2; indexOfProfileByName(profiles, candidate) >= 0; n++ {
		candidate = fmt.Sprintf("%s Copy (%d)", base, n)
	}
	return candidate
}

func intentParts(p domain.Profile) (verb, target string) {
	switch p.LaunchMode {
	case "file":
		return "Load the selected file", filepath.Base(p.SavePath)
	case "folder":
		verb := "Load the most recent " + folderItemNoun(p.AutoLatestFilter)
		if p.SaveSearchSubfolders {
			verb += " anywhere in"
		} else {
			verb += " in"
		}
		return verb, filepath.Base(p.SavePath)
	case "multiplayer":
		return "Launch and join the server at", valueOrDefault(p.ServerIpPort, "Server")
	default:
		return "Launch straight into the Main Menu", ""
	}
}

func folderItemNoun(filter string) string {
	switch filter {
	case "sav":
		return "save"
	case "scn":
		return "scenario"
	default:
		return "save or scenario"
	}
}

// filterDisplay maps an auto-latest filter to a grammar-matched (label, value): plural "File types" for both, singular otherwise.
func filterDisplay(filter string) (label, value string) {
	switch filter {
	case "sav":
		return "File type", "Saves only"
	case "scn":
		return "File type", "Scenarios only"
	default:
		return "File types", "Saves & Scenarios"
	}
}

func newGRFDesc(mode string) string {
	switch strings.ToUpper(strings.TrimSpace(mode)) {
	case "Q":
		return "Skip NewGRF loading at startup"
	case "QQ":
		return "Disable all NewGRF scanning/loading (session-wide)"
	default:
		return ""
	}
}
