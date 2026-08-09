package fyne

import "sync/atomic"

// debounceGuard lets a delayed background check discard its own result if a
// newer check has been scheduled since. This covers a gap time.Timer.Stop()
// leaves open: it cannot cancel a callback that has already started, so a slow
// check (e.g. stat on a dead network share) can otherwise complete AFTER a
// faster, newer one and overwrite its correct result with a stale one.
//
// Atomic because the test driver runs fyne.Do inline on the calling goroutine,
// so a timer callback's current() executes on the timer goroutine while the UI
// thread may be in next(); under the real driver both land on the UI thread.
type debounceGuard struct {
	gen atomic.Int64
}

// next bumps and returns the generation to capture before starting a new check.
func (g *debounceGuard) next() int64 {
	return g.gen.Add(1)
}

// current reports whether gen is still the latest; false means a newer check
// was scheduled since, so this one's result should be discarded.
func (g *debounceGuard) current(gen int64) bool {
	return gen == g.gen.Load()
}
