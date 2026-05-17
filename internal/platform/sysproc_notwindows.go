//go:build !windows

package platform

import "syscall"

// GetDetachedSysProcAttr returns nil process attributes for non-Windows platforms
func GetDetachedSysProcAttr() *syscall.SysProcAttr {
	return nil
}
