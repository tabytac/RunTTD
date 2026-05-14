//go:build !windows

package main

import "syscall"

// getDetachedSysProcAttr returns process attributes to detach the process on Unix-like systems
// so that it survives parent exit.
func getDetachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
