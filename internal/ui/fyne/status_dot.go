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

// upstreamState is the lifecycle of a per-track upstream-latest lookup.
type upstreamState int

const (
	noUpstream      upstreamState = iota // no entry yet / expired — caller should enqueue a fetch
	pendingUpstream                      // a fetch is in flight
	okUpstream                           // fetch succeeded; latestTagFolder is set (possibly "")
	failedUpstream                       // fetch failed (offline/429/timeout)
)

// dotInput holds the pre-computed, network-free inputs to dotState.
type dotInput struct {
	clientKnown      bool
	isCustom         bool
	customPathExists bool
	installedFolder  string
	isLatest         bool
	cacheState       upstreamState
	latestTagFolder  string
}

// dotState resolves the four-state dot from pre-computed, network-free inputs.
func dotState(in dotInput) DotState {
	// Step 1: classify & disk check.
	if !in.clientKnown {
		return DotGrey
	}
	if in.isCustom {
		if in.customPathExists {
			return DotGreen
		}
		return DotRed
	}
	if in.installedFolder == "" {
		return DotRed
	}
	// Installed. Step 2: currency check.
	if !in.isLatest {
		return DotGreen // pinned + installed
	}
	switch in.cacheState {
	case okUpstream:
		if in.latestTagFolder == in.installedFolder {
			return DotGreen
		}
		return DotOrange
	default: // pending, failed, none
		return DotGrey
	}
}
