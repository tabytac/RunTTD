//go:build windows
// +build windows

package main

import "syscall"

func getWindowsSysProcAttr() *syscall.SysProcAttr {
    return &syscall.SysProcAttr{
        CreationFlags: 0x08000000 | 0x00000008, // CREATE_NO_WINDOW | DETACHED_PROCESS
    }
}
