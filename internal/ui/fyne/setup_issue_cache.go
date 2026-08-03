package fyne

import (
	"strings"
	"sync"

	"fyne.io/fyne/v2"

	apppkg "runttd/internal/app"
	"runttd/internal/domain"
)

// setupIssueEntry caches one profile's ProfileSetupIssue result. sig is the
// content signature it was computed for; issue is "" for no problem (and also
// the provisional value while a compute is pending, so the marker stays hidden
// until the real result lands).
type setupIssueEntry struct {
	sig   string
	issue string
}

// setupIssueCache memoizes ProfileSetupIssue off the UI thread: the check stats
// arbitrary user save/config paths, and one on an unreachable network share would
// freeze the render for the full SMB timeout. The row bind and details echo read
// only this cache; a background goroutine fills it. Keyed by profile name, so a
// signature mismatch (any edited path/mode) misses and recomputes.
type setupIssueCache struct {
	mu sync.Mutex
	m  map[string]setupIssueEntry
}

func newSetupIssueCache() *setupIssueCache {
	return &setupIssueCache{m: make(map[string]setupIssueEntry)}
}

// setupIssueSignature is the content fingerprint of the fields ProfileSetupIssue
// reads, plus docsBase (relative paths resolve against it). Any change misses.
func setupIssueSignature(p domain.Profile, docsBase string) string {
	return strings.Join([]string{
		docsBase, p.LaunchMode, p.SavePath, p.AutoLatestFilter, p.ServerIpPort, p.ConfigFilePath,
	}, "\x00")
}

// lookup returns the cached issue and whether it is known for this signature. A
// pending compute counts as known (issue ""), so callers show no marker and do
// not re-enqueue while a fetch is in flight.
func (c *setupIssueCache) lookup(name, sig string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[name]
	if !ok || e.sig != sig {
		return "", false
	}
	return e.issue, true
}

// markPending claims the compute slot for (name, sig). Returns true if the caller
// should start the compute; false if a fresh or in-flight entry already exists.
func (c *setupIssueCache) markPending(name, sig string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.m[name]; ok && e.sig == sig {
		return false
	}
	c.m[name] = setupIssueEntry{sig: sig}
	return true
}

// store records a completed compute result under the lock.
func (c *setupIssueCache) store(name, sig, issue string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[name] = setupIssueEntry{sig: sig, issue: issue}
}

// setupIssueFor returns a profile's cached setup-issue string, enqueueing a
// background compute on a miss and returning "" until it lands. MUST stay off the
// disk on this path: ProfileSetupIssue may stat an unreachable network share.
func (um *UIManager) setupIssueFor(profile domain.Profile) string {
	sig := setupIssueSignature(profile, um.Config.DocsBasePath)
	if issue, ok := um.setupIssues.lookup(profile.Name, sig); ok {
		return issue
	}
	um.startSetupIssueCheck(profile, sig)
	return ""
}

// startSetupIssueCheck runs one deduped background ProfileSetupIssue compute and,
// on completion, refreshes the list (and the details pane if this profile is
// selected) on the UI thread.
func (um *UIManager) startSetupIssueCheck(profile domain.Profile, sig string) {
	if !um.setupIssues.markPending(profile.Name, sig) {
		return
	}
	// Read on the UI thread: settings can write DocsBasePath while the compute runs.
	docsBase := um.Config.DocsBasePath
	um.startAsync(func() {
		issue := apppkg.ProfileSetupIssue(profile, docsBase)
		um.setupIssues.store(profile.Name, sig, issue)
		fyne.Do(func() {
			if um.profileListRefresh != nil {
				um.profileListRefresh()
			}
			if um.detailsRefresh != nil && profile.Name == um.SelectedProfileName {
				um.detailsRefresh()
			}
		})
	})
}
