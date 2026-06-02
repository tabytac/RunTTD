//go:build windows

package platform

import "syscall"

// GetDetachedSysProcAttr returns process attributes to detach the process on Windows so that it survives parent exit
func GetDetachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}

// GetNoWindowSysProcAttr returns process attributes that suppress the console window on Windows
func GetNoWindowSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
