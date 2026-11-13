//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Standalone Windows desktop dashboard. Archive reads are read-only.
// Optional local API operation requests the ORIGINAL LinkVideo.Server to
// synchronize an operator-confirmed interval. Original archives remain untouched.
const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_VISIBLE          = 0x10000000
	WS_CHILD            = 0x40000000
	WS_CLIPSIBLINGS     = 0x04000000
	WS_VSCROLL          = 0x00200000
	WS_TABSTOP          = 0x00010000
	ES_MULTILINE        = 0x0004
	ES_AUTOVSCROLL      = 0x0040
	ES_READONLY         = 0x0800
	ES_AUTOHSCROLL      = 0x0080
	WM_CREATE           = 0x0001
	WM_DESTROY          = 0x0002
	WM_PAINT            = 0x000F
	WM_SIZE             = 0x0005
	WM_COMMAND          = 0x0111
	WM_TIMER            = 0x0113
	WM_SETFONT          = 0x0030
	WM_GETTEXT          = 0x000D
	WM_SETTEXT          = 0x000C
	WM_LBUTTONDOWN      = 0x0201
	WM_MOUSEMOVE        = 0x0200
	WM_CTLCOLORSTATIC   = 0x0138
	WM_CTLCOLOREDIT     = 0x0133
	WM_ERASEBKGND       = 0x0014
	WM_GETMINMAXINFO    = 0x0024
	WM_SETCURSOR        = 0x0020
	SW_SHOW             = 5
	SW_HIDE             = 0
	PROCESS_SNAPSHOT    = 0x00000002
	ES_CONTINUOUS       = 0x80000000
	ES_SYSTEM_REQUIRED  = 0x00000001
	DT_LEFT             = 0x00000000
	DT_CENTER           = 0x00000001
	DT_VCENTER          = 0x00000004
	DT_SINGLELINE       = 0x00000020
	DT_END_ELLIPSIS     = 0x00008000
	TRANSPARENT         = 1
	PS_SOLID            = 0
	SRCCOPY             = 0x00CC0020
	IDC_ARROW           = 32512
	IDC_HAND            = 32649
)

var (
	user32                       = syscall.NewLazyDLL("user32.dll")
	kernel32                     = syscall.NewLazyDLL("kernel32.dll")
	gdi32                        = syscall.NewLazyDLL("gdi32.dll")
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procShowWindow               = user32.NewProc("ShowWindow")
	procSendMessageW             = user32.NewProc("SendMessageW")
	procSetTimer                 = user32.NewProc("SetTimer")
	procKillTimer                = user32.NewProc("KillTimer")
	procMessageBoxW              = user32.NewProc("MessageBoxW")
	procGetClientRect            = user32.NewProc("GetClientRect")
	procInvalidateRect           = user32.NewProc("InvalidateRect")
	procMoveWindow               = user32.NewProc("MoveWindow")
	procBeginPaint               = user32.NewProc("BeginPaint")
	procEndPaint                 = user32.NewProc("EndPaint")
	procSetWindowTextW           = user32.NewProc("SetWindowTextW")
	procLoadCursorW              = user32.NewProc("LoadCursorW")
	procSetCursor                = user32.NewProc("SetCursor")
	procGetModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
	procCloseHandle              = kernel32.NewProc("CloseHandle")
	procSetThreadExecutionState  = kernel32.NewProc("SetThreadExecutionState")
	procSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	procCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap   = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject             = gdi32.NewProc("SelectObject")
	procDeleteObject             = gdi32.NewProc("DeleteObject")
	procDeleteDC                 = gdi32.NewProc("DeleteDC")
	procBitBlt                   = gdi32.NewProc("BitBlt")
	procCreateSolidBrush         = gdi32.NewProc("CreateSolidBrush")
	procCreatePen                = gdi32.NewProc("CreatePen")
	procRoundRect                = gdi32.NewProc("RoundRect")
	procRectangle                = gdi32.NewProc("Rectangle")
	procCreateFontW              = gdi32.NewProc("CreateFontW")
	procSetTextColor             = gdi32.NewProc("SetTextColor")
	procSetBkMode                = gdi32.NewProc("SetBkMode")
	procSetBkColor               = gdi32.NewProc("SetBkColor")
	procDrawTextW                = user32.NewProc("DrawTextW")
)

type wndClass struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	SmallIcon  uintptr
}
type point struct{ X, Y int32 }
type winMessage struct {
	HWND           uintptr
	Message        uint32
	_              uint32
	WParam, LParam uintptr
	Time           uint32
	PT             point
	LPrivate       uint32
}
type rect struct{ Left, Top, Right, Bottom int32 }
type paintStruct struct {
	HDC       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}
type processEntry struct {
	Size, Usage, ProcessID      uint32
	DefaultHeapID               uintptr
	ModuleID, Threads, ParentID uint32
	Priority                    int32
	Flags                       uint32
	ExeFile                     [260]uint16
}
type minmaxinfo struct {
	Reserved     point
	MaxSize      point
	MaxPosition  point
	MinTrackSize point
	MaxTrackSize point
}
type region struct {
	r      rect
	action string
}

type snapshot struct {
	Time      time.Time
	Processes map[string][]uint32
	ProcError string
	API       bool
	Archive   ArchiveScan
	Logs      LogScan
}

var (
	win, pathEdit, reportEdit                          uintptr
	fontNormal, fontSmall, fontBold, fontBig, fontHero uintptr
	current                                            snapshot
	page                                               = 0
	dark                                               = false
	awake                                              = false
	areas                                              []region
	hover                                                    = ""
	exportText                                               = ""
	viewWidth                                          int32 = 1180
	viewHeight                                         int32 = 790
	pathDirty                                                = false
	brushInput                                         uintptr
)

func u(s string) *uint16   { p, _ := syscall.UTF16PtrFromString(s); return p }
func ptr(s string) uintptr { return uintptr(unsafe.Pointer(u(s))) }
func col(h string) uintptr {
	var r, g, b uint32
	fmt.Sscanf(strings.TrimPrefix(h, "#"), "%02x%02x%02x", &r, &g, &b)
	return uintptr(r | (g << 8) | (b << 16))
}
func rr(x, y, w, h int32) rect       { return rect{x, y, x + w, y + h} }
func inside(r rect, x, y int32) bool { return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom }
func textFrom(h uintptr) string {
	a := make([]uint16, 32768)
	procSendMessageW.Call(h, WM_GETTEXT, uintptr(len(a)), uintptr(unsafe.Pointer(&a[0])))
	return strings.TrimSpace(syscall.UTF16ToString(a))
}
func setText(h uintptr, s string) { procSetWindowTextW.Call(h, ptr(s)) }
func sysColor(k string) string {
	switch k {
	case "side":
		return "#20252C"
	case "sideHover":
		return "#313943"
	case "sideText":
		return "#E6EBEF"
	case "muted":
		if dark {
			return "#AAB4C0"
		}
		return "#626F7D"
	case "bg":
		if dark {
			return "#171A1E"
		}
		return "#F3F4F6"
	case "panel":
		if dark {
			return "#252B33"
		}
		return "#FFFFFF"
	case "panelAlt":
		if dark {
			return "#313943"
		}
		return "#F8F9FA"
	case "border":
		if dark {
			return "#3E4853"
		}
		return "#DCE1E6"
	case "text":
		if dark {
			return "#F0F3F6"
		}
		return "#20252C"
	case "accent":
		if dark {
			return "#E27638"
		}
		return "#C94E08"
	case "accentSoft":
		if dark {
			return "#423021"
		}
		return "#FFF0E4"
	case "good":
		if dark {
			return "#48D6A6"
		}
		return "#168F65"
	case "bad":
		if dark {
			return "#FF777D"
		}
		return "#DC4F56"
	case "warn":
		if dark {
			return "#E8B35C"
		}
		return "#B7791F"
	}
	return "#FFFFFF"
}
func fill(hdc uintptr, r rect, bg string, radius int32) {
	brush, _, _ := procCreateSolidBrush.Call(col(bg))
	pen, _, _ := procCreatePen.Call(PS_SOLID, 1, col(bg))
	ob, _, _ := procSelectObject.Call(hdc, brush)
	op, _, _ := procSelectObject.Call(hdc, pen)
	if radius > 0 {
		procRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
	} else {
		procRectangle.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
	}
	procSelectObject.Call(hdc, ob)
	procSelectObject.Call(hdc, op)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}
func outline(hdc uintptr, r rect, stroke string, radius int32, bg string) {
	brush, _, _ := procCreateSolidBrush.Call(col(bg))
	pen, _, _ := procCreatePen.Call(PS_SOLID, 1, col(stroke))
	ob, _, _ := procSelectObject.Call(hdc, brush)
	op, _, _ := procSelectObject.Call(hdc, pen)
	procRoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
	procSelectObject.Call(hdc, ob)
	procSelectObject.Call(hdc, op)
	procDeleteObject.Call(brush)
	procDeleteObject.Call(pen)
}
func txt(hdc uintptr, s string, r rect, font uintptr, color string, flags uintptr) {
	of, _, _ := procSelectObject.Call(hdc, font)
	procSetBkMode.Call(hdc, TRANSPARENT)
	procSetTextColor.Call(hdc, col(color))
	ww := syscall.StringToUTF16(s)
	if len(ww) > 0 {
		procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(&ww[0])), uintptr(len(ww)-1), uintptr(unsafe.Pointer(&r)), flags|DT_END_ELLIPSIS)
	}
	procSelectObject.Call(hdc, of)
}
func label(hdc uintptr, s string, x, y, w, h int32, font uintptr, color string) {
	txt(hdc, s, rr(x, y, w, h), font, color, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}
func center(hdc uintptr, s string, r rect, font uintptr, color string) {
	txt(hdc, s, r, font, color, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}
func action(hdc uintptr, id string, r rect, title string, primary bool) {
	bg := sysColor("panel")
	fg := sysColor("text")
	if primary {
		bg = sysColor("accent")
		fg = "#FFFFFF"
	} else if hover == id {
		bg = sysColor("accentSoft")
	}
	fill(hdc, r, bg, 12)
	if !primary {
		outline(hdc, r, sysColor("border"), 12, bg)
	}
	center(hdc, title, r, fontBold, fg)
	areas = append(areas, region{r, id})
}
func card(hdc uintptr, r rect) { fill(hdc, r, sysColor("panel"), 18) /* subtle border */ }
func tag(hdc uintptr, s string, r rect, bg, fg string) {
	fill(hdc, r, bg, 14)
	center(hdc, s, r, fontSmall, fg)
}
func nav(hdc uintptr, id string, y int32, icon, labelName string, active bool) {
	r := rr(13, y, 210, 43)
	bg := sysColor("side")
	fg := "#CED7DF"
	if active {
		bg = sysColor("accent")
		fg = "#FFFFFF"
	} else if hover == id {
		bg = sysColor("sideHover")
	}
	fill(hdc, r, bg, 13)
	label(hdc, icon, 31, y, 27, 43, fontBold, fg)
	label(hdc, labelName, 66, y, 149, 43, fontNormal, fg)
	areas = append(areas, region{r, id})
}
func dot(hdc uintptr, x, y int32, c string) { fill(hdc, rr(x, y, 9, 9), c, 9) }
func createFont(height int32, weight int32) uintptr {
	h, _, _ := procCreateFontW.Call(uintptr(height), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, ptr("Segoe UI"))
	return h
}
func mainX() int32 { return 253 }
func mainW() int32 { return viewWidth - mainX() - 28 }
func drawSidebar(h uintptr) {
	fill(h, rr(0, 0, 236, viewHeight), sysColor("side"), 0)
	fill(h, rr(23, 25, 37, 37), sysColor("accent"), 11)
	center(h, "▶", rr(23, 25, 37, 37), fontBold, "#FFFFFF")
	label(h, "LinkVideo", 72, 20, 145, 30, fontBig, "#FFFFFF")
	label(h, "SERVER TOOLS", 73, 51, 150, 19, fontSmall, "#A8B3BE")
	fill(h, rr(20, 90, 195, 1), "#3A434D", 0)
	nav(h, "nav0", 117, "◫", "Обзор", page == 0)
	nav(h, "nav1", 169, "▣", "Камеры", page == 1)
	nav(h, "nav2", 221, "▤", "Архив", page == 2)
	nav(h, "nav3", 273, "≡", "Журнал", page == 3)
	nav(h, "nav4", 325, "⚙", "Настройки", page == 4)
	nav(h, "nav5", 377, "⟳", "Nimble", page == 5)
	fill(h, rr(23, viewHeight-102, 190, 1), "#3A434D", 0)
	label(h, "Архив + диагностика + Nimble", 23, viewHeight-89, 190, 26, fontSmall, "#B1BBC5")
	label(h, "v0.8  •  LinkVideo", 23, viewHeight-57, 185, 24, fontSmall, "#84919F")
}
func drawHeader(h uintptr, title, sub string) {
	x := mainX()
	w := mainW()
	label(h, title, x, 24, w-240, 39, fontHero, sysColor("text"))
	label(h, sub, x, 68, w-110, 25, fontNormal, sysColor("muted"))
	action(h, "refresh", rr(x+w-137, 28, 137, 40), "↻  Обновить", true)
	fill(h, rr(x, 110, w, 1), sysColor("border"), 0)
}
func dateLabel(h uintptr) {
	s := "Проверено: "
	if !current.Time.IsZero() {
		s += current.Time.Format("15:04:05")
	} else {
		s += "—"
	}
	label(h, s, mainX(), viewHeight-33, mainW(), 25, fontSmall, sysColor("muted"))
}
func processCard(h uintptr, r rect, title, name string) {
	card(h, r)
	has := len(current.Processes[name]) > 0
	label(h, title, r.Left+19, r.Top+18, r.Right-r.Left-32, 22, fontNormal, sysColor("muted"))
	if has {
		dot(h, r.Left+19, r.Top+60, sysColor("good"))
		label(h, "Обнаружен", r.Left+35, r.Top+50, r.Right-r.Left-47, 36, fontBig, sysColor("text"))
		label(h, formatPIDs(current.Processes[name]), r.Left+19, r.Top+93, r.Right-r.Left-38, 25, fontSmall, sysColor("muted"))
	} else {
		dot(h, r.Left+19, r.Top+60, sysColor("bad"))
		label(h, "Не обнаружен", r.Left+35, r.Top+50, r.Right-r.Left-47, 36, fontBig, sysColor("bad"))
		label(h, "Процесс отсутствует", r.Left+19, r.Top+93, r.Right-r.Left-38, 25, fontSmall, sysColor("muted"))
	}
}
func apiCard(h uintptr, r rect) {
	card(h, r)
	label(h, "Локальный API", r.Left+19, r.Top+18, r.Right-r.Left-32, 22, fontNormal, sysColor("muted"))
	s := "Порт закрыт"
	c := sysColor("bad")
	if current.API {
		s = "Порт доступен"
		c = sysColor("good")
	}
	dot(h, r.Left+19, r.Top+60, c)
	label(h, s, r.Left+35, r.Top+50, r.Right-r.Left-47, 36, fontBig, sysColor("text"))
	label(h, "127.0.0.1:48518  ·  TCP", r.Left+19, r.Top+93, r.Right-r.Left-38, 25, fontSmall, sysColor("muted"))
}
func counts() (cams, dats int, last time.Time) {
	cams = len(current.Archive.Cameras)
	for _, c := range current.Archive.Cameras {
		dats += c.DatFiles
		if c.LastWrite.After(last) {
			last = c.LastWrite
		}
	}
	return
}
func drawOverview(h uintptr) {
	drawHeader(h, "Обзор системы", "Мониторинг LinkVideo.Server и локальной записи без вмешательства")
	x, w := mainX(), mainW()
	gap := int32(14)
	cw := (w - 2*gap) / 3
	y := int32(136)
	processCard(h, rr(x, y, cw, 140), "LINKVIDEO.SERVER", "linkvideo.server.exe")
	processCard(h, rr(x+cw+gap, y, cw, 140), "NIMBLE STREAMER", "nimble.exe")
	apiCard(h, rr(x+(cw+gap)*2, y, cw, 140))
	leftW := (w * 58) / 100
	rightW := w - leftW - gap
	y = 293
	card(h, rr(x, y, leftW, 218))
	card(h, rr(x+leftW+gap, y, rightW, 218))
	label(h, "Локальный архив", x+21, y+17, leftW-30, 29, fontBold, sysColor("text"))
	cams, dats, last := counts()
	label(h, fmt.Sprintf("%d", cams), x+22, y+65, 125, 48, fontHero, sysColor("accent"))
	label(h, "камер", x+24, y+111, 125, 22, fontSmall, sysColor("muted"))
	label(h, fmt.Sprintf("%d", dats), x+182, y+65, 125, 48, fontHero, sysColor("text"))
	label(h, "файлов .dat*", x+183, y+111, 178, 22, fontSmall, sysColor("muted"))
	note := "Последнее изменение .dat: " + ageString(last)
	if current.Archive.Error != "" {
		note = "Архив недоступен: " + current.Archive.Error
	}
	label(h, note, x+22, y+155, leftW-40, 24, fontNormal, sysColor("muted"))
	label(h, "* В трёх последних каталогах каждой камеры", x+22, y+184, leftW-40, 22, fontSmall, sysColor("muted"))
	rx := x + leftW + gap
	label(h, "Ошибки за последний час", rx+21, y+17, rightW-35, 29, fontBold, sysColor("text"))
	if current.Logs.Warning != "" {
		label(h, "Журнал недоступен", rx+20, y+61, rightW-40, 28, fontBig, sysColor("bad"))
		label(h, current.Logs.Warning, rx+20, y+102, rightW-40, 25, fontSmall, sysColor("muted"))
	} else {
		label(h, fmt.Sprintf("%d", current.Logs.Errors), rx+23, y+66, 110, 48, fontHero, sysColor("text"))
		label(h, "строк с ошибками", rx+23, y+109, rightW-35, 24, fontSmall, sysColor("muted"))
		label(h, fmt.Sprintf("Кодек / поток: %d", current.Logs.Codec), rx+23, y+151, rightW-35, 22, fontNormal, sysColor("muted"))
		label(h, fmt.Sprintf("Сеть: %d  ·  cron: %d", current.Logs.Network, current.Logs.Cron), rx+23, y+177, rightW-35, 22, fontNormal, sysColor("muted"))
	}
	y = 528
	card(h, rr(x, y, w, maxInt32(108, viewHeight-y-55)))
	label(h, "Камеры", x+20, y+16, w-35, 28, fontBold, sysColor("text"))
	if cams == 0 {
		label(h, "Камеры не найдены в указанной папке Videos", x+22, y+59, w-46, 32, fontNormal, sysColor("muted"))
	}
	for i, c := range current.Archive.Cameras {
		if i >= 3 {
			break
		}
		cy := y + 56 + int32(i)*44
		if cy+36 > viewHeight-57 {
			break
		}
		dot(h, x+24, cy+10, cameraColor(c.LastWrite))
		label(h, c.Camera, x+45, cy, w/2, 26, fontNormal, sysColor("text"))
		label(h, ageString(c.LastWrite), x+w/2, cy, w/2-25, 26, fontSmall, sysColor("muted"))
	}
	dateLabel(h)
}
func maxInt32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
func cameraColor(t time.Time) string {
	if t.IsZero() {
		return sysColor("bad")
	}
	if time.Since(t) > 5*time.Minute {
		return sysColor("warn")
	}
	return sysColor("good")
}
func drawCameras(h uintptr) {
	drawHeader(h, "Камеры", "Состояние локальных файлов записи, не проверка изображения или RTSP")
	x, w := mainX(), mainW()
	if len(current.Archive.Cameras) == 0 {
		card(h, rr(x, 139, w, 146))
		label(h, "Камеры не найдены", x+22, 159, w-44, 38, fontBig, sysColor("text"))
		label(h, "Проверьте путь к архиву на вкладке «Архив».", x+22, 209, w-44, 27, fontNormal, sysColor("muted"))
		dateLabel(h)
		return
	}
	for i, c := range current.Archive.Cameras {
		y := int32(137 + i*92)
		if y+81 > viewHeight-48 {
			break
		}
		r := rr(x, y, w, 82)
		card(h, r)
		dot(h, x+24, y+35, cameraColor(c.LastWrite))
		label(h, c.Camera, x+47, y+15, w/2, 29, fontBold, sysColor("text"))
		label(h, "Последнее изменение: "+ageString(c.LastWrite), x+47, y+46, w/2, 22, fontSmall, sysColor("muted"))
		idx := "Индекс не найден"
		if c.IndexFound {
			idx = "Индекс найден"
		}
		label(h, fmt.Sprintf(".dat: %d  •  %s", c.DatFiles, idx), x+w/2, y+21, w/2-25, 39, fontNormal, sysColor("muted"))
	}
	dateLabel(h)
}
func drawArchive(h uintptr) {
	drawHeader(h, "Локальный архив", "Файлы Nimble и доступность данных на диске")
	x, w := mainX(), mainW()
	card(h, rr(x, 136, w, 141))
	label(h, "Папка Videos", x+22, 151, w-44, 28, fontBold, sysColor("text"))
	// A real native text field is overlaid at (x+25, 199) by positionInputs().
	label(h, "Измените путь и нажмите «Обновить»", x+22, 244, w-44, 23, fontSmall, sysColor("muted"))
	card(h, rr(x, 292, w, 198))
	c, d, last := counts()
	label(h, "Проверка индексов и сегментов", x+23, 309, w-46, 32, fontBold, sysColor("text"))
	label(h, fmt.Sprintf("Камер: %d    Файлов .dat: %d", c, d), x+23, 363, w-46, 31, fontBig, sysColor("text"))
	label(h, "Последний .dat: "+ageString(last), x+23, 405, w-46, 25, fontNormal, sysColor("muted"))
	label(h, "Доступность файлов не означает, что видео целое или выгружено в облако.", x+23, 448, w-46, 24, fontSmall, sysColor("muted"))
	card(h, rr(x, 507, w, 110))
	label(h, "Ручная выгрузка", x+22, 521, w-44, 30, fontBold, sysColor("text"))
	label(h, "Пока не реализована. Проверку облачных пропусков и передачу файлов добавим после тестов API.", x+22, 558, w-44, 32, fontNormal, sysColor("muted"))
	dateLabel(h)
}
func drawJournal(h uintptr) {
	drawHeader(h, "Журнал диагностики", "Сводка без исходных строк логов, URL камер и секретов")
	x, w := mainX(), mainW()
	card(h, rr(x, 137, w, 130))
	l := current.Logs
	label(h, "Последний час", x+21, 151, w-40, 27, fontBold, sysColor("text"))
	label(h, fmt.Sprintf("Ошибки: %d        Кодек / поток: %d        Сеть: %d        Cron: %d", l.Errors, l.Codec, l.Network, l.Cron), x+21, 190, w-41, 31, fontBig, sysColor("text"))
	action(h, "export", rr(x+w-181, 223, 164, 35), "↓  Сохранить TXT", false)
	card(h, rr(x, 285, w, maxInt32(190, viewHeight-342)))
	label(h, "Техническая сводка", x+20, 300, w-40, 33, fontBold, sysColor("text"))
	// ReportBox positioned by positionInputs().
	dateLabel(h)
}
func drawSettings(h uintptr) {
	drawHeader(h, "Настройки", "Параметры отображения и предотвращение автоматического сна")
	x, w := mainX(), mainW()
	card(h, rr(x, 137, w, 165))
	label(h, "Внешний вид", x+22, 155, w-44, 30, fontBold, sysColor("text"))
	label(h, "Палитра LinkVideo.Helper: графит, оранжевый акцент и светлые карточки", x+22, 196, w-44, 24, fontNormal, sysColor("muted"))
	themeText := "Тема: LinkVideo"
	if dark {
		themeText = "Тема: графит"
	}
	action(h, "theme", rr(x+22, 237, 190, 43), themeText, false)
	card(h, rr(x, 321, w, 180))
	label(h, "Электропитание", x+22, 339, w-44, 30, fontBold, sysColor("text"))
	label(h, "Запрет системного сна действует только пока открыта программа.", x+22, 376, w-44, 26, fontNormal, sysColor("muted"))
	text := "Запрет сна: выкл"
	if awake {
		text = "Запрет сна: ВКЛ"
	}
	action(h, "awake", rr(x+22, 415, 205, 44), text, awake)
	label(h, "Не защищает от принудительной гибернации, отключения ПК или групповых политик.", x+22, 472, w-44, 23, fontSmall, sysColor("muted"))
	card(h, rr(x, 519, w, 104))
	label(h, "Безопасный режим", x+22, 534, w-44, 30, fontBold, sysColor("text"))
	label(h, "Изменения Nimble только из вкладки Nimble. Загрузок в облако нет.", x+22, 572, w-44, 24, fontNormal, sysColor("muted"))
	dateLabel(h)
}
func draw(hdc uintptr) {
	w, h := viewWidth, viewHeight
	if w < 600 || h < 400 {
		return
	}
	mem, _, _ := procCreateCompatibleDC.Call(hdc)
	bmp, _, _ := procCreateCompatibleBitmap.Call(hdc, uintptr(w), uintptr(h))
	old, _, _ := procSelectObject.Call(mem, bmp)
	fill(mem, rr(0, 0, w, h), sysColor("bg"), 0)
	areas = areas[:0]
	drawSidebar(mem)
	switch page {
	case 0:
		drawOverview(mem)
	case 1:
		drawCameras(mem)
	case 2:
		drawArchiveNew(mem)
	case 3:
		drawJournal(mem)
	case 4:
		drawSettings(mem)
	case 5:
		drawNimble(mem)
	}
	procBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, SRCCOPY)
	procSelectObject.Call(mem, old)
	procDeleteObject.Call(bmp)
	procDeleteDC.Call(mem)
}
func processSnapshot() (map[string][]uint32, error) {
	result := make(map[string][]uint32)
	h, _, err := procCreateToolhelp32Snapshot.Call(PROCESS_SNAPSHOT, 0)
	if h == 0 || h == ^uintptr(0) {
		return result, fmt.Errorf("не удалось прочитать процессы: %v", err)
	}
	defer procCloseHandle.Call(h)
	var p processEntry
	p.Size = uint32(unsafe.Sizeof(p))
	ok, _, e := procProcess32FirstW.Call(h, uintptr(unsafe.Pointer(&p)))
	if ok == 0 {
		return result, fmt.Errorf("Process32FirstW: %v", e)
	}
	for ok != 0 {
		name := strings.ToLower(syscall.UTF16ToString(p.ExeFile[:]))
		result[name] = append(result[name], p.ProcessID)
		p.Size = uint32(unsafe.Sizeof(p))
		ok, _, _ = procProcess32NextW.Call(h, uintptr(unsafe.Pointer(&p)))
	}
	return result, nil
}
func formatPIDs(pids []uint32) string {
	if len(pids) == 0 {
		return "не найден"
	}
	s := make([]string, len(pids))
	for i, p := range pids {
		s[i] = fmt.Sprint(p)
	}
	return "PID: " + strings.Join(s, ", ")
}
func timestamp(t time.Time) string {
	if t.IsZero() {
		return "нет данных"
	}
	return t.Format("02.01.2006 15:04:05")
}
func snapshotReport(s snapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "LinkVideo.Server Tools v0.8 — диагностика\r\nПроверено: %s\r\n\r\n", s.Time.Format("02.01.2006 15:04:05"))
	b.WriteString("[ПРОЦЕССЫ]\r\n")
	if s.ProcError != "" {
		fmt.Fprintf(&b, "Ошибка: %s\r\n", s.ProcError)
	}
	for _, name := range []string{"linkvideo.server.exe", "nimble.exe"} {
		fmt.Fprintf(&b, "%s: %s\r\n", name, formatPIDs(s.Processes[name]))
	}
	b.WriteString("\r\n[ЛОКАЛЬНЫЙ API]\r\n")
	if s.API {
		b.WriteString("127.0.0.1:48518: TCP соединение успешно\r\n")
	} else {
		b.WriteString("127.0.0.1:48518: порт недоступен\r\n")
	}
	b.WriteString("Не является проверкой HTTP, авторизации или успешной записи.\r\n")
	fmt.Fprintf(&b, "\r\n[АРХИВ]\r\nПапка: %s\r\n", s.Archive.Root)
	if s.Archive.Error != "" {
		fmt.Fprintf(&b, "Ошибка папки: %s\r\n", s.Archive.Error)
	}
	for _, c := range s.Archive.Cameras {
		fmt.Fprintf(&b, "%s: .dat=%d, индекс=%t, последний .dat=%s (%s)\r\n", c.Camera, c.DatFiles, c.IndexFound, timestamp(c.LastWrite), ageString(c.LastWrite))
		if c.Warning != "" {
			fmt.Fprintf(&b, "  Проблема просмотра: %s\r\n", c.Warning)
		}
	}
	b.WriteString("Подсчёт .dat только в трёх последних каталогах камеры.\r\n")
	b.WriteString("\r\n[ЛОГИ ЗА ПОСЛЕДНИЙ ЧАС]\r\n")
	if s.Logs.Warning != "" {
		fmt.Fprintf(&b, "Проблема: %s\r\n", s.Logs.Warning)
	} else {
		fmt.Fprintf(&b, "Файл: %s\r\nОшибки: %d; кодек/поток: %d; cron: %d; сеть: %d\r\n", s.Logs.File, s.Logs.Errors, s.Logs.Codec, s.Logs.Cron, s.Logs.Network)
	}
	b.WriteString("\r\n[ОГРАНИЧЕНИЯ]\r\nНе проверяет кадры, целостность видео или облачный архив.\r\nБез управления сервисами и запросов к камерам.\r\nПароли и строки логов не сохраняются в отчёт.\r\n")
	return b.String()
}
func doRefresh() {
	root := strings.TrimSpace(textFrom(pathEdit))
	if root == "" {
		root = likelyArchiveFolder()
	}
	p, err := processSnapshot()
	s := snapshot{Time: time.Now(), Processes: p, API: apiListening(), Archive: newestCameraData(root, 50), Logs: logSnapshot(installLogFolder(), time.Now().Add(-time.Hour))}
	if err != nil {
		s.ProcError = err.Error()
	}
	current = s
	exportText = snapshotReport(s)
	setText(reportEdit, exportText)
	invalidate()
}
func invalidate() { procInvalidateRect.Call(win, 0, 0) }
func saveReport() {
	desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	if info, err := os.Stat(desktop); err != nil || !info.IsDir() {
		desktop = os.TempDir()
	}
	name := filepath.Join(desktop, "LinkVideo_ServerTools_Report_"+time.Now().Format("20060102_150405")+".txt")
	if err := os.WriteFile(name, []byte(exportText), 0600); err != nil {
		procMessageBoxW.Call(win, ptr(err.Error()), ptr("Ошибка экспорта"), 0x10)
		return
	}
	procMessageBoxW.Call(win, ptr("Диагностический отчёт сохранён:\n"+name), ptr("Готово"), 0x40)
}
func toggleAwake() {
	if !awake {
		r, _, _ := procSetThreadExecutionState.Call(ES_CONTINUOUS | ES_SYSTEM_REQUIRED)
		if r == 0 {
			procMessageBoxW.Call(win, ptr("Не удалось отключить автоматический сон"), ptr("Ошибка"), 0x10)
			return
		}
		awake = true
	} else {
		procSetThreadExecutionState.Call(ES_CONTINUOUS)
		awake = false
	}
	invalidate()
}
func hit(actionID string) {
	if extensionAction(actionID) {
		return
	}
	switch actionID {
	case "refresh":
		doRefresh()
		if page == 2 {
			scanArchiveAsync()
		}
		if page == 5 {
			watchState.LastCheck = time.Time{}
			tickWatch()
		}
	case "export":
		saveReport()
	case "awake":
		toggleAwake()
	case "theme":
		dark = !dark
		setEditColors()
		invalidate()
	default:
		if strings.HasPrefix(actionID, "nav") {
			var id int
			fmt.Sscanf(actionID, "nav%d", &id)
			if id >= 0 && id <= 5 {
				page = id
				if page == 2 && len(catalog.Cameras) == 0 && !scanning {
					scanArchiveAsync()
				}
				positionInputs()
				invalidate()
			}
		}
	}
}
func setEditColors() {
	if brushInput != 0 {
		procDeleteObject.Call(brushInput)
	}
	brushInput, _, _ = procCreateSolidBrush.Call(col(sysColor("panelAlt")))
	invalidate()
}
func show(h uintptr, on bool) {
	if on {
		procShowWindow.Call(h, SW_SHOW)
	} else {
		procShowWindow.Call(h, SW_HIDE)
	}
}
func positionInputs() {
	x, w := mainX(), mainW()
	show(pathEdit, page == 2 && !archiveExportView && !archiveSyncView)
	show(reportEdit, page == 3)
	if page == 2 && !archiveExportView && !archiveSyncView {
		procMoveWindow.Call(pathEdit, uintptr(x+23), 184, uintptr(w-460), 37, 1)
	}
	if page == 3 {
		procMoveWindow.Call(reportEdit, uintptr(x+23), 342, uintptr(w-46), uintptr(maxInt32(100, viewHeight-420)), 1)
	}
	positionExtraControls()
	positionNativeSyncControl()
}
func windowProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	if extensionMessage(hwnd, msg, wp, lp) {
		return 0
	}
	switch msg {
	case WM_GETMINMAXINFO:
		mm := (*minmaxinfo)(unsafe.Pointer(lp))
		mm.MinTrackSize.X = 1060
		mm.MinTrackSize.Y = 760
		return 0
	case WM_CREATE:
		win = hwnd
		fontNormal = createFont(-17, 400)
		fontSmall = createFont(-14, 400)
		fontBold = createFont(-17, 600)
		fontBig = createFont(-22, 600)
		fontHero = createFont(-30, 700)
		pathEdit, _, _ = procCreateWindowExW.Call(0, ptr("EDIT"), ptr(startArchivePath()), WS_CHILD|WS_VISIBLE|WS_TABSTOP|ES_AUTOHSCROLL|ES_READONLY, 0, 0, 0, 0, hwnd, 1001, 0, 0)
		reportEdit, _, _ = procCreateWindowExW.Call(0, ptr("EDIT"), ptr(""), WS_CHILD|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY, 0, 0, 0, 0, hwnd, 1002, 0, 0)
		procSendMessageW.Call(pathEdit, WM_SETFONT, fontNormal, 1)
		procSendMessageW.Call(reportEdit, WM_SETFONT, fontNormal, 1)
		setEditColors()
		initExtraControls(hwnd)
		initNativeSyncControl(hwnd)
		positionInputs()
		procSetTimer.Call(hwnd, 1, 15000, 0)
		procSetTimer.Call(hwnd, 2, 3000, 0)
		doRefresh()
		return 0
	case WM_SIZE:
		viewWidth = int32(uint16(lp & 0xFFFF))
		viewHeight = int32(uint16((lp >> 16) & 0xFFFF))
		positionInputs()
		invalidate()
		return 0
	case WM_PAINT:
		var ps paintStruct
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		draw(hdc)
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_CTLCOLOREDIT, WM_CTLCOLORSTATIC, WM_CTLCOLORLISTBOX:
		if lp == pathEdit || lp == reportEdit || lp == archiveList || lp == checkEdit || lp == afterEdit || lp == staleEdit || lp == cooldownEdit || lp == serviceEdit || lp == syncPasswordEdit {
			procSetTextColor.Call(wp, col(sysColor("text")))
			procSetBkColor.Call(wp, col(sysColor("panelAlt")))
			return brushInput
		}
	case WM_COMMAND:
		archiveListChanged(wp, lp)
		return 0
	case WM_LBUTTONDOWN:
		x := int32(int16(lp & 0xFFFF))
		y := int32(int16((lp >> 16) & 0xFFFF))
		if calendarTarget != "" {
			for i := len(areas) - 1; i >= 0; i-- {
				a := areas[i]
				if strings.HasPrefix(a.action, "cal_") && inside(a.r, x, y) {
					calendarAction(a.action)
					return 0
				}
			}
			calendarTarget = ""
			positionInputs()
			invalidate()
			return 0
		}
		for _, a := range areas {
			if inside(a.r, x, y) {
				hit(a.action)
				break
			}
		}
		return 0
	case WM_MOUSEMOVE:
		x := int32(int16(lp & 0xFFFF))
		y := int32(int16((lp >> 16) & 0xFFFF))
		val := ""
		for _, a := range areas {
			if inside(a.r, x, y) {
				val = a.action
				break
			}
		}
		if val != hover {
			hover = val
			invalidate()
		}
		return 0
	case WM_SETCURSOR:
		if hover != "" {
			c, _, _ := procLoadCursorW.Call(0, IDC_HAND)
			procSetCursor.Call(c)
			return 1
		}
	case WM_TIMER:
		if wp == 1 {
			doRefresh()
			tickWatch()
		} else if wp == 2 {
			pollNativeSyncDiagnostics()
		}
		return 0
	case WM_DESTROY:
		if awake {
			procSetThreadExecutionState.Call(ES_CONTINUOUS)
		}
		shutdownDateControls()
		if nativeSyncCancel != nil {
			nativeSyncCancel()
		}
		procKillTimer.Call(hwnd, 1)
		procKillTimer.Call(hwnd, 2)
		if brushInput != 0 {
			procDeleteObject.Call(brushInput)
		}
		for _, f := range []uintptr{fontNormal, fontSmall, fontBold, fontBig, fontHero} {
			if f != 0 {
				procDeleteObject.Call(f)
			}
		}
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
	return r
}
func main() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	procSetProcessDPIAware.Call()
	initDateControls()
	inst, _, _ := procGetModuleHandleW.Call(0)
	class := u("LinkVideoServerTools08")
	wc := wndClass{Size: uint32(unsafe.Sizeof(wndClass{})), WndProc: syscall.NewCallback(windowProc), Instance: inst, ClassName: class}
	r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		procMessageBoxW.Call(0, ptr(fmt.Sprintf("Регистрация окна: %v", e)), ptr("Ошибка"), 0x10)
		return
	}
	win, _, e = procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(class)), ptr("LinkVideo.Server Tools  ·  v0.8"), WS_OVERLAPPEDWINDOW|WS_VISIBLE|WS_CLIPSIBLINGS, 70, 50, 1180, 790, 0, 0, inst, 0)
	if win == 0 {
		procMessageBoxW.Call(0, ptr(fmt.Sprintf("Создание окна: %v", e)), ptr("Ошибка"), 0x10)
		return
	}
	procShowWindow.Call(win, SW_SHOW)
	var m winMessage
	for {
		r, _, _ = procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}