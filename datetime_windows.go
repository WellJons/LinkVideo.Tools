//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const (
	DTS_UPDOWN           = 0x0001
	DTS_TIMEFORMAT       = 0x0009
	DTM_FIRST            = 0x1000
	DTM_GETSYSTEMTIME    = DTM_FIRST + 1
	DTM_SETSYSTEMTIME    = DTM_FIRST + 2
	DTM_SETFORMATW       = DTM_FIRST + 50
	GDT_VALID            = 0
	BIF_RETURNONLYFSDIRS = 0x0001
	BIF_NEWDIALOGSTYLE   = 0x0040
)

type winSystemTime struct {
	Year         uint16
	Month        uint16
	DayOfWeek    uint16
	Day          uint16
	Hour         uint16
	Minute       uint16
	Second       uint16
	Milliseconds uint16
}

type browseInfo struct {
	Owner       uintptr
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	Param       uintptr
	Image       int32
}

type commonControls struct {
	Size  uint32
	Flags uint32
}

var (
	comctl32                 = syscall.NewLazyDLL("comctl32.dll")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
	procSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	ole32                    = syscall.NewLazyDLL("ole32.dll")
	procCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
	procOleInitialize        = ole32.NewProc("OleInitialize")
	procOleUninitialize      = ole32.NewProc("OleUninitialize")
)

func initDateControls() {
	init := commonControls{Size: 8, Flags: 0x00000100}
	procInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&init)))
	procOleInitialize.Call(0)
}
func shutdownDateControls() { procOleUninitialize.Call() }

func dateControl(parent uintptr, id int, timeOnly bool) uintptr {
	style := uintptr(WS_CHILD | WS_TABSTOP)
	if timeOnly {
		style |= DTS_TIMEFORMAT | DTS_UPDOWN
	}
	hwnd, _, _ := procCreateWindowExW.Call(0, ptr("SysDateTimePick32"), 0, style, 0, 0, 0, 0, parent, uintptr(id), 0, 0)
	if hwnd == 0 {
		return 0
	}
	procSendMessageW.Call(hwnd, WM_SETFONT, fontNormal, 1)
	if timeOnly {
		procSendMessageW.Call(hwnd, DTM_SETFORMATW, 0, ptr("HH':'mm':'ss"))
	} else {
		procSendMessageW.Call(hwnd, DTM_SETFORMATW, 0, ptr("dd'.'MM'.'yyyy"))
	}
	setDateTime(hwnd, time.Now())
	return hwnd
}
func setDateTime(hwnd uintptr, t time.Time) {
	if hwnd == 0 {
		return
	}
	st := winSystemTime{Year: uint16(t.Year()), Month: uint16(t.Month()), Day: uint16(t.Day()), Hour: uint16(t.Hour()), Minute: uint16(t.Minute()), Second: uint16(t.Second())}
	procSendMessageW.Call(hwnd, DTM_SETSYSTEMTIME, GDT_VALID, uintptr(unsafe.Pointer(&st)))
}
func dateTimeValue(hwnd uintptr) (time.Time, error) {
	if hwnd == 0 {
		return time.Time{}, fmt.Errorf("элемент выбора даты/времени не создан")
	}
	var st winSystemTime
	val, _, _ := procSendMessageW.Call(hwnd, DTM_GETSYSTEMTIME, 0, uintptr(unsafe.Pointer(&st)))
	if val != GDT_VALID {
		return time.Time{}, fmt.Errorf("не выбрана дата или время")
	}
	if st.Month < 1 || st.Month > 12 || st.Day < 1 || st.Day > 31 || st.Hour > 23 || st.Minute > 59 || st.Second > 59 {
		return time.Time{}, fmt.Errorf("некорректная дата или время")
	}
	t := time.Date(int(st.Year), time.Month(st.Month), int(st.Day), int(st.Hour), int(st.Minute), int(st.Second), 0, time.Local)
	if t.Year() != int(st.Year) || int(t.Month()) != int(st.Month) || t.Day() != int(st.Day) {
		return time.Time{}, fmt.Errorf("некорректная календарная дата")
	}
	return t, nil
}
func combinedPickerDateTime(dateH, timeH uintptr) (time.Time, error) {
	d, err := dateTimeValue(dateH)
	if err != nil {
		return time.Time{}, err
	}
	v, err := dateTimeValue(timeH)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(d.Year(), d.Month(), d.Day(), v.Hour(), v.Minute(), v.Second(), 0, time.Local), nil
}
func folderDialog(owner uintptr, initial string) string {
	var buf [260]uint16
	bi := browseInfo{Owner: owner, DisplayName: &buf[0], Title: u("Выберите папку Videos с записями LinkVideo.Server"), Flags: BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE}
	pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer procCoTaskMemFree.Call(pidl)
	var path [32768]uint16
	ok, _, _ := procSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return ""
	}
	found := syscall.UTF16ToString(path[:])
	if filepath.Base(found) == "." {
		return ""
	}
	return found
}
