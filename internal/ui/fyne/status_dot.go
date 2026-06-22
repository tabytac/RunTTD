package fyne

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
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

const dotDiameter = 10

// statusDot is a small filled circle with a thin foreground ring (the ring
// guarantees edge contrast on any theme/background — amber-on-light fails the
// 3:1 bar as a bare fill).
type statusDot struct {
	widget.BaseWidget
	state DotState
}

func newStatusDot() *statusDot {
	d := &statusDot{state: DotGrey}
	d.ExtendBaseWidget(d)
	return d
}

// SetState recolors the dot in place.
func (d *statusDot) SetState(s DotState) {
	if s == d.state {
		return
	}
	d.state = s
	d.Refresh()
}

func (d *statusDot) CreateRenderer() fyne.WidgetRenderer {
	circle := canvas.NewCircle(dotColor(d.state))
	circle.StrokeColor = ringColor()
	circle.StrokeWidth = 1
	return &statusDotRenderer{dot: d, circle: circle}
}

// ringColor is the dot's outline: the theme foreground at low opacity.
func ringColor() color.Color {
	fg := theme.Color(theme.ColorNameForeground)
	r, g, b, _ := fg.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 90}
}

type statusDotRenderer struct {
	dot    *statusDot
	circle *canvas.Circle
}

func (r *statusDotRenderer) Layout(size fyne.Size) {
	// Center a fixed-diameter circle within whatever space we are given.
	d := float32(dotDiameter)
	x := (size.Width - d) / 2
	y := (size.Height - d) / 2
	r.circle.Move(fyne.NewPos(x, y))
	r.circle.Resize(fyne.NewSize(d, d))
}

func (r *statusDotRenderer) MinSize() fyne.Size {
	return fyne.NewSize(dotDiameter, dotDiameter)
}

func (r *statusDotRenderer) Refresh() {
	r.circle.FillColor = dotColor(r.dot.state)
	r.circle.StrokeColor = ringColor()
	r.circle.Refresh()
}

func (r *statusDotRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.circle}
}

func (r *statusDotRenderer) Destroy() {}
