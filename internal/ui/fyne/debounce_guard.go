package fyne

// debounceGuard lets a delayed background check discard its own result if a
// newer check has been scheduled since. This covers a gap time.Timer.Stop()
// leaves open: it cannot cancel a callback that has already started, so a slow
// check (e.g. stat on a dead network share) can otherwise complete AFTER a
// faster, newer one and overwrite its correct result with a stale one.
//
// Not synchronized: every caller in this package only touches it from the UI
// thread (an OnChanged-style callback to claim a generation, a fyne.Do callback
// to check it), the same assumption main_view.go's launchGen/scanGen already make.
type debounceGuard struct {
	gen int
}

// next bumps and returns the generation to capture before starting a new check.
func (g *debounceGuard) next() int {
	g.gen++
	return g.gen
}

// current reports whether gen is still the latest; false means a newer check
// was scheduled since, so this one's result should be discarded.
func (g *debounceGuard) current(gen int) bool {
	return gen == g.gen
}
