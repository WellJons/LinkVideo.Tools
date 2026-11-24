//go:build windows

package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

var (
	clipboardOpen    = user32.NewProc("OpenClipboard")
	clipboardClose   = user32.NewProc("CloseClipboard")
	clipboardEmpty   = user32.NewProc("EmptyClipboard")
	clipboardSetData = user32.NewProc("SetClipboardData")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	globalFree       = kernel32.NewProc("GlobalFree")
)

func copyNetworkText(value string) error {
	value = strings.ReplaceAll(value, "\x00", "")
	if value == "" {
		return fmt.Errorf("нет значения для копирования")
	}
	data := syscall.StringToUTF16(value)
	block, _, _ := globalAlloc.Call(0x0002, uintptr(len(data)*2)) // GMEM_MOVEABLE
	if block == 0 {
		return fmt.Errorf("не удалось выделить память буфера обмена")
	}
	ptr, _, _ := globalLock.Call(block)
	if ptr == 0 {
		globalFree.Call(block)
		return fmt.Errorf("не удалось заблокировать память")
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(data)), data)
	globalUnlock.Call(block)
	ok, _, _ := clipboardOpen.Call(win)
	if ok == 0 {
		globalFree.Call(block)
		return fmt.Errorf("буфер обмена занят другой программой")
	}
	defer clipboardClose.Call()
	ok, _, _ = clipboardEmpty.Call()
	if ok == 0 {
		globalFree.Call(block)
		return fmt.Errorf("не удалось очистить буфер обмена")
	}
	ok, _, _ = clipboardSetData.Call(13, block) // CF_UNICODETEXT; clipboard now owns block.
	if ok == 0 {
		globalFree.Call(block)
		return fmt.Errorf("не удалось скопировать текст")
	}
	return nil
}
