//go:build windows

package main

import (
	"os"
	"syscall"
)

const attachParentProcess = ^uintptr(0) // ATTACH_PARENT_PROCESS

// attachParentConsole joins the console RunTTD was started from, which a
// windowsgui binary otherwise detaches from, sending --help and every stderr
// message nowhere. Redirected handles are left alone so pipes still work.
func attachParentConsole() {
	outValid := stdHandleValid(syscall.STD_OUTPUT_HANDLE)
	errValid := stdHandleValid(syscall.STD_ERROR_HANDLE)
	if outValid && errValid {
		return
	}
	attach := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	if r, _, _ := attach.Call(attachParentProcess); r == 0 {
		return // no parent console: a double-click or a scheduled run
	}
	if !outValid {
		if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			os.Stdout = f
		}
	}
	if !errValid {
		if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			os.Stderr = f
		}
	}
}

func stdHandleValid(nStdHandle int) bool {
	h, err := syscall.GetStdHandle(nStdHandle)
	if err != nil || h == 0 || h == syscall.InvalidHandle {
		return false
	}
	t, err := syscall.GetFileType(h)
	return err == nil && t != syscall.FILE_TYPE_UNKNOWN
}
