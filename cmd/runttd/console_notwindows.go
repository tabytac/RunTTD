//go:build !windows

package main

// attachParentConsole is Windows-only: elsewhere a terminal-started process
// already has its terminal.
func attachParentConsole() {}
