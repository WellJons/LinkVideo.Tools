//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procSHGetFolderPathW = shell32.NewProc("SHGetFolderPathW")
)

func windowsDocumentsFolder() string {
	var buf [260]uint16
	// CSIDL_PERSONAL supports relocated, OneDrive, and localized Documents.
	rc, _, _ := procSHGetFolderPathW.Call(0, 5, 0, 0, uintptr(unsafe.Pointer(&buf[0])))
	if rc != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:])
}
