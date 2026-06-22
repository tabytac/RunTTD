package fyne

import (
	"sync"
	"time"
)

// upstreamEntry is one per-client-track cache record.
type upstreamEntry struct {
	tag       string // newest upstream tag (diagnostics only)
	tagFolder string // the latest tag's resolved installed folder, "" if not installed
	state     upstreamState
	fetched   time.Time
}

// upstreamCache holds the newest-upstream result per client track, keyed by
// client ID ("jgrpp", "vanilla", "vanilla-nightly"). Reads (render) and writes
// (background goroutines) both take the mutex.
type upstreamCache struct {
	mu  sync.Mutex
	m   map[string]upstreamEntry
	ttl time.Duration
	now func() time.Time
}

func newUpstreamCache() *upstreamCache {
	return &upstreamCache{
		m:   make(map[string]upstreamEntry),
		ttl: 10 * time.Minute,
		now: time.Now,
	}
}

// fresh reports whether an entry should be trusted without re-fetching. Pending
// is always fresh (a fetch is in flight). ok is fresh within the TTL. failed and
// absent are never fresh (allow retry). Caller must hold the lock.
func (c *upstreamCache) fresh(e upstreamEntry, ok bool) bool {
	if !ok {
		return false
	}
	switch e.state {
	case pendingUpstream:
		return true
	case okUpstream:
		return c.now().Sub(e.fetched) < c.ttl
	default:
		return false
	}
}

// get returns the entry and whether it is fresh (trustworthy without re-fetch).
func (c *upstreamCache) get(track string) (upstreamEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[track]
	return e, c.fresh(e, ok)
}

// markPending atomically claims the fetch slot for a track. It returns true if
// the caller should start the fetch (no fresh entry and none in flight), and
// records a pending entry. Returns false if a fresh/pending entry already exists.
func (c *upstreamCache) markPending(track string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[track]
	if c.fresh(e, ok) {
		return false
	}
	c.m[track] = upstreamEntry{state: pendingUpstream, fetched: c.now()}
	return true
}

// store records a completed fetch result under the lock.
func (c *upstreamCache) store(track, tag, tagFolder string, state upstreamState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[track] = upstreamEntry{tag: tag, tagFolder: tagFolder, state: state, fetched: c.now()}
}
