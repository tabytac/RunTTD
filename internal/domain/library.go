package domain

import "time"

// InstalledVersion describes one client/version folder found on disk.
type InstalledVersion struct {
	Path      string // absolute folder path
	Client    string // "jgrpp" | "vanilla" | "vanilla-nightly" | "" if unclassifiable
	Version   string // best-effort, display-only; "" if unknown
	OSTag     string // raw OS/arch tag from the folder name, e.g. "windows-win64"; "" if absent
	SizeBytes int64
	ModTime   time.Time
}

// LibraryEntry is an InstalledVersion annotated with the profiles that resolve
// to it. An empty ReferencedBy means no profile launches this folder (orphan).
type LibraryEntry struct {
	InstalledVersion
	ReferencedBy []string // profile names; empty = orphan
}
