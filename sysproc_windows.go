//go:build windows
// +build windows

package main

import "syscall"

// getDetachedSysProcAttr returns process attributes to detach the process on Windows
// so that it survives parent exit.
func getDetachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}
