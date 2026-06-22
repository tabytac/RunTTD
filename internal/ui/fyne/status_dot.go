package fyne

import (
	"image/color"
)

// DotState is the resolved status of a profile's installed/upstream state,
// rendered as a colored dot on the profile row.
type DotState int

const (
	DotGrey   DotState = iota // checking, fetch failed, or unknown client
	DotGreen                  // installed and current
	DotRed                    // not installed; launch will download
	DotOrange                 // installed but a newer version is available
)

// dotColor maps a DotState to its fill color. Greys default state.
func dotColor(s DotState) color.Color {
	switch s {
	case DotGreen:
		return libGreen
	case DotRed:
		return dotRed
	case DotOrange:
		return libAmber
	default:
		return libGrey
	}
}
