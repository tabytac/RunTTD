package fyne

import "sync"

// diskLookupEntry caches one key's resolved folder ("" means not installed).
type diskLookupEntry struct {
	folder  string
	pending bool
}

// diskLookupCache memoizes "does (client, track-or-version) resolve to an
// installed folder" and "does this custom path exist" off the UI thread — both
// are plain filesystem calls that can hang for the full SMB timeout on an
// unreachable network share.
//
// Unlike upstreamCache (network results, tolerant of staleness within a TTL) or
// setupIssueCache (a soft advisory warning, tolerant of staleness until the
// profile changes), a stale disk answer here is a real inaccuracy: the dot must
// reflect a fresh download or a library deletion right away. So this cache has
// no TTL and isn't keyed by profile — it's invalidated wholesale by invalidate()
// at the few places installed folders actually change (a launch completing, a
// library delete/cleanup, or a settings save that could move the install root).
//
// gen guards against a compute that was already in flight when invalidate() ran:
// without it, that compute's answer (based on the pre-invalidate disk state) could
// land in store() AFTER the post-invalidate recompute, permanently overwriting the
// correct answer with a stale one. store() only writes if its captured gen still
// matches the cache's current gen; invalidate() bumps gen, so any such late store
// is silently dropped instead.
type diskLookupCache struct {
	mu  sync.Mutex
	gen int
	m   map[string]diskLookupEntry
}

func newDiskLookupCache() *diskLookupCache {
	return &diskLookupCache{m: make(map[string]diskLookupEntry)}
}

// lookup returns the cached folder and whether it is known (present, not pending).
func (c *diskLookupCache) lookup(key string) (folder string, known bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || e.pending {
		return "", false
	}
	return e.folder, true
}

// markPending claims the compute slot for key. Returns the generation to pass to
// store, and whether the caller should start the compute (false if already known
// or already in flight).
func (c *diskLookupCache) markPending(key string) (gen int, start bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.m[key]; ok {
		return c.gen, false
	}
	c.m[key] = diskLookupEntry{pending: true}
	return c.gen, true
}

// store records a completed compute result, but only if gen still matches the
// cache's current generation; a result computed before the last invalidate() is
// silently dropped rather than overwriting whatever replaced it.
func (c *diskLookupCache) store(key, folder string, gen int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if gen != c.gen {
		return
	}
	c.m[key] = diskLookupEntry{folder: folder}
}

// invalidate clears every entry and bumps the generation, forcing the next lookup
// to recompute and discarding any in-flight compute's eventual store().
func (c *diskLookupCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.m = make(map[string]diskLookupEntry)
}
