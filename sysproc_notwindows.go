//go:build !windows
// +build !windows

package main

import "syscall"

// getWindowsSysProcAttr returns nil on non-Windows platforms so callers can
// assign it safely without referencing Windows-only struct fields.
func getWindowsSysProcAttr() *syscall.SysProcAttr {
    return nil
}
