//go:build windows

package platform

import "syscall"

// GetDetachedSysProcAttr returns process attributes to detach the process on Windows so that it survives parent exit
func GetDetachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}
