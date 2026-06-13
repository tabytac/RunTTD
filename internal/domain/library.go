package domain

import "time"

// InstalledVersion describes one client/version folder found on disk.
type InstalledVersion struct {
	Path      string
	Client    string // "jgrpp" | "vanilla" | "vanilla-nightly" | "" if unclassifiable
	Version   string // best-effort, display-only
	OSTag     string // raw OS/arch tag, e.g. "windows-win64"
	SizeBytes int64
	ModTime   time.Time
}

// LibraryEntry is an InstalledVersion annotated with the profiles that resolve
// to it; an empty ReferencedBy means an orphan no profile launches.
type LibraryEntry struct {
	InstalledVersion
	ReferencedBy []string
}
