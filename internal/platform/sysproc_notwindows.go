//go:build !windows

package platform

import "syscall"

// GetDetachedSysProcAttr returns nil process attributes for non-Windows platforms
func GetDetachedSysProcAttr() *syscall.SysProcAttr {
	return nil
}

// GetNoWindowSysProcAttr returns nil process attributes for non-Windows platforms
func GetNoWindowSysProcAttr() *syscall.SysProcAttr {
	return nil
}
