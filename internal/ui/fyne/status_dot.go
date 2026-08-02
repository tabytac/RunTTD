package fyne

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	apppkg "runttd/internal/app"
	"runttd/internal/domain"
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

// dotFill is a state's fill per theme. Green and amber wash out on the near-white
// light row background, so light mode darkens them to clear the 3:1 contrast bar
// (#2F8A2F, #B36B00); red and grey read fine on both, so dark == light there.
type dotFill struct{ dark, light color.Color }

// dotFills is the source of truth for dot colors. To retune a dot, edit here.
var dotFills = map[DotState]dotFill{
	DotGreen:  {dark: libGreen, light: color.NRGBA{R: 47, G: 138, B: 47, A: 255}},  // #2F8A2F
	DotOrange: {dark: libAmber, light: color.NRGBA{R: 179, G: 107, B: 0, A: 255}},  // #B36B00
	DotRed:    {dark: dotRed, light: dotRed},
	DotGrey:   {dark: libGrey, light: libGrey},
}

// dotColor returns a DotState's fill for the active theme. Unknown states (none
// today) fall back to grey.
func dotColor(s DotState, light bool) color.Color {
	f, ok := dotFills[s]
	if !ok {
		return libGrey
	}
	if light {
		return f.light
	}
	return f.dark
}

// isLightTheme reports whether the active (override-aware) theme is light, by
// the lightness of its foreground: dark text means a light background.
func isLightTheme() bool {
	fg := theme.Color(theme.ColorNameForeground)
	r, g, b, _ := fg.RGBA()
	return (r>>8)+(g>>8)+(b>>8) < 3*128
}

// upstreamState is the lifecycle of a per-track upstream-latest lookup.
type upstreamState int

const (
	noUpstream      upstreamState = iota // no entry yet / expired — caller should enqueue a fetch
	pendingUpstream                      // a fetch is in flight
	okUpstream                           // fetch succeeded; latestTagFolder is set (possibly "")
	failedUpstream                       // fetch failed (offline/429/timeout)
)

// dotInput holds the pre-computed inputs to dotState. Every disk-derived field has
// a matching *Known bool: false means the async lookup hasn't resolved yet, so
// dotState must show Grey rather than guess Red/Green from a zero value.
type dotInput struct {
	clientKnown bool

	isCustom        bool
	customPathKnown bool
	customPathExists bool

	isLatest             bool
	installedFolderKnown bool
	installedFolder      string

	cacheState           upstreamState
	latestTagFolderKnown bool
	latestTagFolder      string
}

// dotState resolves the four-state dot from pre-computed inputs. Every branch that
// depends on an async disk lookup checks its *Known flag first, so an unresolved
// lookup reads as "still checking" (Grey), never a guessed Red or Green.
func dotState(in dotInput) DotState {
	if !in.clientKnown {
		return DotGrey
	}
	if in.isCustom {
		if !in.customPathKnown {
			return DotGrey
		}
		if in.customPathExists {
			return DotGreen
		}
		return DotRed
	}
	if !in.installedFolderKnown {
		return DotGrey
	}
	if in.installedFolder == "" {
		return DotRed
	}
	if !in.isLatest {
		return DotGreen // pinned + installed
	}
	switch in.cacheState {
	case okUpstream:
		if !in.latestTagFolderKnown {
			return DotGrey
		}
		// Not installedFolder: that scan is track-blind, so a higher off-track beta would read as an update.
		if in.latestTagFolder != "" {
			return DotGreen
		}
		return DotOrange
	default: // pending, failed, none
		return DotGrey
	}
}

const dotDiameter = 10

// statusDot is a small filled circle whose color encodes the profile's status.
// Fills are picked per theme (dotColor) so each clears the contrast bar on its
// own — no outline ring.
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
	circle := canvas.NewCircle(dotColor(d.state, isLightTheme()))
	return &statusDotRenderer{dot: d, circle: circle}
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
	r.circle.FillColor = dotColor(r.dot.state, isLightTheme())
	r.circle.Refresh()
}

func (r *statusDotRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.circle}
}

func (r *statusDotRenderer) Destroy() {}

// resolveDotState computes a profile's dot state reading only caches (the disk
// lookup cache below, and the pre-existing upstream network cache), and enqueues a
// background compute for whichever inputs aren't cached yet. It MUST stay
// I/O-free on this path: every disk read and network call happens in a
// backgrounded goroutine dispatched from here, never inline.
func (um *UIManager) resolveDotState(profile domain.Profile) DotState {
	client := apppkg.EffectiveClient(profile.Client, um.Config.DefaultClient)

	in := dotInput{}
	in.clientKnown = apppkg.IsKnownClient(client)
	if !in.clientKnown {
		return dotState(in)
	}

	if client == "custom" {
		in.isCustom = true
		p := strings.TrimSpace(profile.CustomExecutablePath)
		if p == "" {
			in.customPathKnown = true // nothing configured; "not found" is already final
			return dotState(in)
		}
		key := "custom|" + p
		if folder, known := um.diskLookups.lookup(key); known {
			in.customPathKnown = true
			in.customPathExists = folder != ""
		} else {
			um.startDiskLookup(key, func() string {
				if _, err := os.Stat(p); err == nil {
					return p
				}
				return ""
			})
		}
		return dotState(in)
	}

	// Snapshot Config now (UI thread) rather than let a background compute read
	// um.Config directly: settings can write ParentDir/SubfolderPerClient/OSType
	// concurrently, and *domain.Config has no mutex of its own.
	cfg := *um.Config

	in.isLatest = apppkg.IsLatestVersion(profile.Version)

	if in.isLatest {
		track := apppkg.LatestTrack(client, profile.Version)
		key := "track|" + client + "|" + track
		if folder, known := um.diskLookups.lookup(key); known {
			in.installedFolderKnown = true
			in.installedFolder = folder
		} else {
			um.startDiskLookup(key, func() string {
				return apppkg.HighestInstalledFolderInRoot(&cfg, client, track)
			})
		}
	} else {
		key := "version|" + client + "|" + profile.Version
		if folder, known := um.diskLookups.lookup(key); known {
			in.installedFolderKnown = true
			in.installedFolder = folder
		} else {
			version := profile.Version
			um.startDiskLookup(key, func() string {
				folder, _ := apppkg.ClientFindInstalled(context.Background(), client, version, &cfg)
				return folder
			})
		}
	}

	if !in.installedFolderKnown {
		return dotState(in) // still checking
	}
	if in.installedFolder == "" {
		return dotState(in) // Red — nothing installed
	}
	if !in.isLatest {
		return dotState(in) // Green — pinned + installed
	}

	// Check upstream on the profile's track, and re-resolve the cached tag to a
	// folder here (not at fetch time) so a download is reflected at once (the
	// disk cache is invalidated wholesale after a launch or a library delete).
	track := apppkg.LatestTrack(client, profile.Version)
	ukey := upstreamKey(client, track)
	if e, fresh := um.upstream.get(ukey); fresh {
		in.cacheState = e.state
		if e.state == okUpstream && e.tag != "" {
			tag := e.tag
			dkey := "version|" + client + "|" + tag
			if folder, known := um.diskLookups.lookup(dkey); known {
				in.latestTagFolderKnown = true
				in.latestTagFolder = folder
			} else {
				um.startDiskLookup(dkey, func() string {
					folder, _ := apppkg.ClientFindInstalled(context.Background(), client, tag, &cfg)
					return folder
				})
			}
		} else {
			in.latestTagFolderKnown = true // no tag to resolve against; "" is already final
		}
	} else {
		in.cacheState = pendingUpstream
		um.startUpstreamFetch(client, track)
	}
	return dotState(in)
}

// upstreamKey scopes a cache entry to a (client, track) pair so a client's
// latest-stable and latest-testing profiles don't share one stale tag.
func upstreamKey(client, track string) string { return client + "|" + track }

// startUpstreamFetch launches one background lookup per (client, track) (deduped).
// On completion it stores the result and refreshes the profile list on the UI
// thread. Errors are silent.
func (um *UIManager) startUpstreamFetch(client, track string) {
	key := upstreamKey(client, track)
	if !um.upstream.markPending(key) {
		return // already fresh or in flight
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		tag := apppkg.ClientLatestForTrack(ctx, client, track, um.Config)
		if tag == "" {
			um.LogVerbose(fmt.Sprintf("Upstream version check failed for %s (%s); status unknown (offline?)", client, track))
			um.upstream.store(key, "", failedUpstream)
		} else {
			um.upstream.store(key, tag, okUpstream)
		}
		fyne.Do(func() {
			if um.profileListRefresh != nil {
				um.profileListRefresh()
			}
		})
	}()
}

// startDiskLookup runs one deduped background compute for key (an os.Stat or an
// installed-folder scan — both can hang on an unreachable network share, so
// neither may run on the UI thread) and, on completion, stores the result and
// refreshes the profile list so the dot re-renders.
func (um *UIManager) startDiskLookup(key string, compute func() string) {
	gen, start := um.diskLookups.markPending(key)
	if !start {
		return // already known or already in flight
	}
	go func() {
		folder := compute()
		um.diskLookups.store(key, folder, gen)
		fyne.Do(func() {
			if um.profileListRefresh != nil {
				um.profileListRefresh()
			}
		})
	}()
}
