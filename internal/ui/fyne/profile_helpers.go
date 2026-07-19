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
		return "Load the most recent " + folderItemNoun(p.AutoLatestFilter) + " in", filepath.Base(p.SavePath)
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
