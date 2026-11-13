//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	WM_APP_CATALOG     = 0x8001
	WM_APP_EXPORT      = 0x8002
	WM_APP_RESTART     = 0x8003
	WM_APP_DISCOVERY   = 0x8004
	WM_APP_CLOUD       = 0x8005
	WM_APP_NATIVE_SYNC = 0x8006
	WS_BORDER          = 0x00800000
	LBS_NOTIFY         = 0x0001
	LB_RESETCONTENT    = 0x0184
	LB_ADDSTRING       = 0x0180
	LB_GETCURSEL       = 0x0188
	LB_SETCURSEL       = 0x0186
	WM_CTLCOLORLISTBOX = 0x0134
	MB_YESNO_ICONWARN  = 0x34
)

var (
	archiveList, filterDate, startDate, startClock, endDate, endClock uintptr
	checkEdit, afterEdit, staleEdit, cooldownEdit, serviceEdit        uintptr
	catalog                                                           ArchiveCatalog
	selectedCamera                                                    int
	displayedRanges                                                   []int
	selectedRange                                                     = -1
	dateFilterActive                                                  bool
	scanning                                                          bool
	exportBusy                                                        bool
	archiveNotice                                                     = "Нажмите «Найти записи», чтобы прочитать индексы Nimble."
	watchConf                                                         = loadSettings()
	watchState                                                        WatchState
	catalogResults                                                    = make(chan catalogResponse, 1)
	exportResults                                                     = make(chan error, 1)
	restartResults                                                    = make(chan error, 1)
	discoveryResults                                                  = make(chan discoveryResponse, 1)
	cloudResults                                                      = make(chan cloudReply, 1)
	cloudBusy                                                         bool
	cloudNotice                                                       = "Проверка облака ещё не выполнялась"
)

type catalogResponse struct {
	cat ArchiveCatalog
	err error
}
type discoveryResponse struct {
	name string
	err  error
}

func nativeEdit(hwnd uintptr, id int, initial string) uintptr {
	h, _, _ := procCreateWindowExW.Call(0, ptr("EDIT"), ptr(initial), WS_CHILD|WS_TABSTOP|WS_BORDER|ES_AUTOHSCROLL, 0, 0, 0, 0, hwnd, uintptr(id), 0, 0)
	procSendMessageW.Call(h, WM_SETFONT, fontNormal, 1)
	return h
}
func initExtraControls(hwnd uintptr) {
	archiveList, _, _ = procCreateWindowExW.Call(0, ptr("LISTBOX"), ptr(""), WS_CHILD|WS_TABSTOP|WS_VSCROLL|WS_BORDER|LBS_NOTIFY, 0, 0, 0, 0, hwnd, 1003, 0, 0)
	procSendMessageW.Call(archiveList, WM_SETFONT, fontSmall, 1)
	filterDate = dateControl(hwnd, 1004, false)
	startDate = dateControl(hwnd, 1005, false)
	startClock = dateControl(hwnd, 1006, true)
	endDate = dateControl(hwnd, 1007, false)
	endClock = dateControl(hwnd, 1008, true)
	checkEdit = nativeEdit(hwnd, 1010, strconv.Itoa(watchConf.CheckSeconds))
	afterEdit = nativeEdit(hwnd, 1011, strconv.Itoa(watchConf.StartAfterSeconds))
	staleEdit = nativeEdit(hwnd, 1012, strconv.Itoa(watchConf.StaleMinutes))
	cooldownEdit = nativeEdit(hwnd, 1013, strconv.Itoa(watchConf.CooldownMinutes))
	serviceEdit = nativeEdit(hwnd, 1014, watchConf.ServiceName)
}
func archivePositions() (listTop, listHeight, pickerLabel, pickerTop int32) {
	// Reserve a horizontal availability timeline above the list and a fixed
	// footer for date/time controls, cloud diagnostic and MP4 actions.
	listTop = 493
	listHeight = viewHeight - 672
	if listHeight < 80 {
		listHeight = 80
	}
	pickerLabel = viewHeight - 166
	pickerTop = viewHeight - 143
	return
}
func positionExtraControls() {
	x, w := mainX(), mainW()
	// The archive page owns its click targets and overlays. Keep hidden
	// native date fields only as lossless date state for export.
	for _, h := range []uintptr{archiveList, filterDate, startDate, endDate} {
		show(h, false)
	}
	clocksVisible := page == 2 && (archiveExportView || archiveSyncView) && calendarTarget == ""
	show(startClock, clocksVisible)
	show(endClock, clocksVisible)
	if clocksVisible {
		half := (w - 60) / 2
		clockY := int32(470)
		if archiveSyncView {
			clockY = 465
		}
		procMoveWindow.Call(startClock, uintptr(x+23), uintptr(clockY), uintptr(half), 43, 1)
		procMoveWindow.Call(endClock, uintptr(x+w/2+7), uintptr(clockY), uintptr(half), 43, 1)
	}
	isWatch := page == 5
	for _, h := range []uintptr{checkEdit, afterEdit, staleEdit, cooldownEdit, serviceEdit} {
		show(h, isWatch)
	}
	if isWatch {
		for i, h := range []uintptr{checkEdit, afterEdit, staleEdit, cooldownEdit} {
			procMoveWindow.Call(h, uintptr(x+25+int32(i)*(w-50)/4), 338, uintptr((w-90)/4), 34, 1)
		}
		procMoveWindow.Call(serviceEdit, uintptr(x+25), 447, uintptr(w-285), 34, 1)
	}
}
func startArchivePath() string {
	if watchConf.ArchiveMode == "manual" && watchConf.ArchivePath != "" {
		return watchConf.ArchivePath
	}
	return likelyArchiveFolder()
}
func persistArchivePath(mode, root string) error {
	watchConf.ArchiveMode = mode
	watchConf.ArchivePath = root
	return saveSettings(watchConf)
}

func scanArchiveAsync() {
	if scanning {
		return
	}
	scanning = true
	archiveNotice = "Поиск локальных записей..."
	invalidate()
	root := strings.TrimSpace(textFrom(pathEdit))
	if root == "" {
		root = likelyArchiveFolder()
	}
	go func() {
		c, e := scanArchiveCatalog(root)
		catalogResults <- catalogResponse{c, e}
		procPostMessageW.Call(win, WM_APP_CATALOG, 0, 0)
	}()
}
func refreshArchiveList() {
	procSendMessageW.Call(archiveList, LB_RESETCONTENT, 0, 0)
	displayedRanges = nil
	selectedRange = -1
	if selectedCamera >= len(catalog.Cameras) {
		selectedCamera = 0
	}
	if selectedCamera < 0 || len(catalog.Cameras) == 0 {
		return
	}
	cam := catalog.Cameras[selectedCamera]
	var filter time.Time
	if dateFilterActive {
		if dt, err := dateTimeValue(filterDate); err == nil {
			filter = time.Date(dt.Year(), dt.Month(), dt.Day(), 0, 0, 0, 0, time.Local)
		}
	}
	var lastEnd int64
	for i, r := range cam.Ranges {
		if !filter.IsZero() {
			if r.End <= filter.UnixMilli() || r.Start >= filter.AddDate(0, 0, 1).UnixMilli() {
				continue
			}
		}
		start, end := time.UnixMilli(r.Start).Local(), time.UnixMilli(r.End).Local()
		if lastEnd > 0 && r.Start > lastEnd+1000 {
			g0, g1 := time.UnixMilli(lastEnd).Local(), start
			line := fmt.Sprintf("НЕТ ЛОКАЛЬНО   %s — %s   •   %s", g0.Format("15:04:05"), g1.Format("15:04:05"), humanDuration(r.Start-lastEnd))
			procSendMessageW.Call(archiveList, LB_ADDSTRING, 0, ptr(line))
			displayedRanges = append(displayedRanges, -1)
		}
		line := fmt.Sprintf("ЕСТЬ ЗАПИСЬ   %s — %s   •   %s", start.Format("02.01 15:04:05"), end.Format("15:04:05"), humanDuration(r.End-r.Start))
		if r.AudioCount == 0 {
			line += "   •   без звука"
		}
		procSendMessageW.Call(archiveList, LB_ADDSTRING, 0, ptr(line))
		displayedRanges = append(displayedRanges, i)
		if r.End > lastEnd {
			lastEnd = r.End
		}
	}
	if archiveListOffset >= len(displayedRanges) {
		archiveListOffset = 0
	}
	for i, ri := range displayedRanges {
		if ri >= 0 {
			procSendMessageW.Call(archiveList, LB_SETCURSEL, uintptr(i), 0)
			selectArchiveRange(i)
			break
		}
	}
}
func selectArchiveRange(displayIndex int) {
	if displayIndex < 0 || displayIndex >= len(displayedRanges) {
		return
	}
	selectedRange = displayedRanges[displayIndex]
	if selectedRange < 0 {
		archiveNotice = "На выбранном сером участке локальной записи нет — MP4 и выгрузка невозможны."
		invalidate()
		return
	}
	r := catalog.Cameras[selectedCamera].Ranges[selectedRange]
	setDateTime(startDate, time.UnixMilli(r.Start).Local())
	setDateTime(startClock, time.UnixMilli(r.Start).Local())
	setDateTime(endDate, time.UnixMilli(r.End).Local())
	setDateTime(endClock, time.UnixMilli(r.End).Local())
	invalidate()
}
func archiveListChanged(wp, lp uintptr) {
	if lp != archiveList || uint16(wp&0xffff) != 1003 || uint16((wp>>16)&0xffff) != 1 {
		return
	}
	id, _, _ := procSendMessageW.Call(archiveList, LB_GETCURSEL, 0, 0)
	if id != ^uintptr(0) {
		selectArchiveRange(int(id))
	}
}
func exportSelected() {
	if exportBusy || scanning {
		return
	}
	if selectedCamera >= len(catalog.Cameras) || selectedRange < 0 {
		showMsg("Сначала найдите и выберите период записи.", "Нет записи", 0x30)
		return
	}
	start, e1 := combinedPickerDateTime(startDate, startClock)
	end, e2 := combinedPickerDateTime(endDate, endClock)
	if e1 != nil || e2 != nil {
		showMsg("Выберите действительные даты и время начала/конца", "Неверное время", 0x30)
		return
	}
	ff := locateFFmpeg()
	if ff == "" {
		showMsg("Не найден ffmpeg.exe. Нужна установленная LinkVideo.Server с FFmpeg в каталоге программы.", "FFmpeg", 0x30)
		return
	}
	cam := catalog.Cameras[selectedCamera]
	filename := exportFilename(cam.Name, RecordingRange{Start: start.UnixMilli(), End: end.UnixMilli()})
	out := filepath.Join(archiveExportDir(), filename)
	if _, err := os.Stat(out); err == nil {
		showMsg("Такой MP4 уже существует. Он не будет перезаписан: "+out, "Файл существует", 0x30)
		return
	}
	exportBusy = true
	archiveNotice = "Экспорт: " + filename
	invalidate()
	ci, ri, from, to := selectedCamera, selectedRange, start.UnixMilli(), end.UnixMilli()
	localCat := catalog
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Minute)
		defer cancel()
		err := ExportRecordingRange(ctx, localCat, ci, ri, from, to, ff, out)
		if err == nil {
			archiveNoticeNoThreadUnsafe(out)
		}
		exportResults <- err
		procPostMessageW.Call(win, WM_APP_EXPORT, 0, 0)
	}()
}

// Paths are communicated through a channel, not by mutating UI state from goroutines.
var successfulOutput = make(chan string, 1)

func archiveNoticeNoThreadUnsafe(p string) { successfulOutput <- p }
func exportDone() {
	exportBusy = false
	err := <-exportResults
	if err != nil {
		archiveNotice = "Ошибка сборки MP4"
		showMsg(err.Error(), "MP4: ошибка", 0x10)
	} else {
		p := <-successfulOutput
		archiveNotice = "Готово: " + p
		showMsg("MP4 создан:\n"+p, "Экспорт завершён", 0x40)
	}
	invalidate()
}
func showMsg(message, title string, flags uintptr) uintptr {
	v, _, _ := procMessageBoxW.Call(win, ptr(message), ptr(title), flags)
	return v
}

func drawNimble(h uintptr) {
	drawHeader(h, "Мониторинг Nimble", "Проверки процесса и локальной записи; управление через Windows-службу")
	x, w := mainX(), mainW()
	card(h, rr(x, 137, w, 125))
	status := "Nimble не запущен"
	color := sysColor("bad")
	if len(current.Processes["nimble.exe"]) > 0 {
		status = "Nimble запущен"
		color = sysColor("good")
	}
	dot(h, x+22, 176, color)
	label(h, status, x+42, 158, w-55, 40, fontBig, sysColor("text"))
	label(h, "Последняя проверка: "+timestamp(watchState.LastCheck), x+22, 211, w-40, 27, fontNormal, sysColor("muted"))
	card(h, rr(x, 275, w, 239))
	label(h, "Интервалы проверки и перезапуска", x+22, 286, w-46, 33, fontBold, sysColor("text"))
	fields := []string{"Проверка, сек", "Нет процесса, сек", "Нет записи, мин", "Пауза после рестарта, мин"}
	for i, s := range fields {
		cw := (w - 50) / 4
		label(h, s, x+25+int32(i)*cw, 318, cw-10, 18, fontSmall, sysColor("muted"))
	}
	label(h, "Имя службы Windows (не процесса)", x+25, 393, w-50, 23, fontNormal, sysColor("text"))
	action(h, "discover_service", rr(x+w-238, 443, 208, 40), "Найти службу", false)
	card(h, rr(x, 528, w, 170))
	label(h, "Реакция на сбой", x+22, 540, w-40, 28, fontBold, sysColor("text"))
	auto := "Автозапуск: ВЫКЛ"
	if watchConf.AutoRestart {
		auto = "Автозапуск: ВКЛ"
	}
	action(h, "toggle_auto", rr(x+22, 575, 230, 41), auto, watchConf.AutoRestart)
	stale := "По простою записи: ВЫКЛ"
	if watchConf.RestartOnStale {
		stale = "По простою записи: ВКЛ"
	}
	action(h, "toggle_stale", rr(x+266, 575, 260, 41), stale, watchConf.RestartOnStale)
	action(h, "save_watch", rr(x+w-158, 575, 138, 41), "Сохранить", true)
	action(h, "manual_restart", rr(x+22, 634, 216, 42), "Перезапуск вручную", false)
	message := watchState.LastMessage
	if message == "" {
		message = "Автоперезапуск по умолчанию выключен."
	}
	label(h, message, x+256, 637, w-279, 35, fontSmall, sysColor("muted"))
	dateLabel(h)
}
func saveWatchInputs() error {
	vals := []struct {
		h    uintptr
		name string
	}{{checkEdit, "Проверка"}, {afterEdit, "Ожидание"}, {staleEdit, "Простой"}, {cooldownEdit, "Пауза"}}
	nums := make([]int, len(vals))
	for i, v := range vals {
		n, e := strconv.Atoi(strings.TrimSpace(textFrom(v.h)))
		if e != nil {
			return fmt.Errorf("%s: нужно целое число", v.name)
		}
		nums[i] = n
	}
	s := watchConf
	s.CheckSeconds = nums[0]
	s.StartAfterSeconds = nums[1]
	s.StaleMinutes = nums[2]
	s.CooldownMinutes = nums[3]
	s.ServiceName = textFrom(serviceEdit)
	s.sanitize()
	watchConf = s
	if err := saveSettings(s); err != nil {
		return err
	}
	setText(checkEdit, strconv.Itoa(s.CheckSeconds))
	setText(afterEdit, strconv.Itoa(s.StartAfterSeconds))
	setText(staleEdit, strconv.Itoa(s.StaleMinutes))
	setText(cooldownEdit, strconv.Itoa(s.CooldownMinutes))
	watchState.LastMessage = "Параметры сохранены"
	invalidate()
	return nil
}
func restartAsync(reason string) {
	if watchState.Working {
		return
	}
	if err := watchState.allowed(time.Now(), watchConf); err != nil {
		watchState.LastMessage = err.Error()
		invalidate()
		return
	}
	if strings.TrimSpace(watchConf.ServiceName) == "" {
		showMsg("Укажи имя службы Nimble или нажми «Найти службу».", "Не задана служба", 0x30)
		return
	}
	watchState.Working = true
	now := time.Now()
	watchState.recordRestart(now)
	watchConf.RestartHistory = append(watchConf.RestartHistory, now)
	_ = saveSettings(watchConf)
	watchState.LastMessage = "Перезапуск службы: " + reason
	invalidate()
	service := watchConf.ServiceName
	go func() {
		err := restartService(service)
		restartResults <- err
		procPostMessageW.Call(win, WM_APP_RESTART, 0, 0)
	}()
}
func tickWatch() {
	t := time.Now()
	if !watchState.LastCheck.IsZero() && t.Sub(watchState.LastCheck) < time.Duration(watchConf.CheckSeconds)*time.Second {
		return
	}
	watchState.LastCheck = t
	running := len(current.Processes["nimble.exe"]) > 0
	shouldStart := isOfflineLongEnough(&watchState, t, watchConf, running)
	stale := stoppedRecording(current.Archive.Cameras, watchConf.StaleMinutes)
	if stale {
		watchState.StaleCount++
	} else {
		watchState.StaleCount = 0
	}
	if !watchConf.AutoRestart || watchState.Working {
		return
	}
	if shouldStart {
		restartAsync("процесс отсутствует дольше порога")
		return
	}
	if running && watchConf.RestartOnStale && watchState.StaleCount >= 2 {
		restartAsync("нет новых .dat у всех камер")
	}
}
func extensionAction(id string) bool {
	if archiveViewAction(id) {
		return true
	}
	switch id {
	case "scan":
		scanArchiveAsync()
		return true
	case "filter":
		dateFilterActive = true
		refreshArchiveList()
		invalidate()
		return true
	case "clear_filter":
		dateFilterActive = false
		refreshArchiveList()
		invalidate()
		return true
	case "auto_path":
		root := likelyArchiveFolder()
		setText(pathEdit, root)
		if err := persistArchivePath("auto", ""); err != nil {
			showMsg(err.Error(), "Настройки", 0x10)
		}
		catalog = ArchiveCatalog{}
		scanArchiveAsync()
		return true
	case "browse_path":
		root := folderDialog(win, textFrom(pathEdit))
		root = normalizedVideoRoot(root)
		if root != "" {
			setText(pathEdit, root)
			if err := persistArchivePath("manual", root); err != nil {
				showMsg(err.Error(), "Настройки", 0x10)
			}
			catalog = ArchiveCatalog{}
			scanArchiveAsync()
		}
		return true
	case "camera_prev", "camera_next":
		if len(catalog.Cameras) > 0 {
			if id == "camera_prev" {
				selectedCamera = (selectedCamera + len(catalog.Cameras) - 1) % len(catalog.Cameras)
			} else {
				selectedCamera = (selectedCamera + 1) % len(catalog.Cameras)
			}
			archiveListOffset = 0
			refreshArchiveList()
			invalidate()
		}
		return true
	case "cloud_probe":
		startCloudProbe()
		return true
	case "make_mp4":
		exportSelected()
		return true
	case "open_exports":
		_ = os.MkdirAll(archiveExportDir(), 0755)
		_ = exec.Command("explorer.exe", archiveExportDir()).Start()
		return true
	case "save_watch":
		if err := saveWatchInputs(); err != nil {
			showMsg(err.Error(), "Параметры", 0x10)
		}
		return true
	case "toggle_auto":
		if !watchConf.AutoRestart {
			if showMsg("Автоперезапуск будет управлять Windows-службой Nimble при сбое. Нужны права администратора. Включить?", "Подтверждение", MB_YESNO_ICONWARN) != 6 {
				return true
			}
		}
		watchConf.AutoRestart = !watchConf.AutoRestart
		_ = saveSettings(watchConf)
		invalidate()
		return true
	case "toggle_stale":
		if !watchConf.RestartOnStale {
			if showMsg("Простой камер иногда бывает штатным. Автоматически перезапускать Nimble при отсутствии записи у всех камер?", "Осторожно", MB_YESNO_ICONWARN) != 6 {
				return true
			}
		}
		watchConf.RestartOnStale = !watchConf.RestartOnStale
		_ = saveSettings(watchConf)
		invalidate()
		return true
	case "manual_restart":
		if err := saveWatchInputs(); err != nil {
			showMsg(err.Error(), "Параметры", 0x10)
			return true
		}
		if showMsg("Перезапустить службу Nimble? Возможен кратковременный обрыв записи.", "Подтвердить перезапуск", MB_YESNO_ICONWARN) == 6 {
			restartAsync("вручную")
		}
		return true
	case "discover_service":
		watchState.LastMessage = "Поиск службы Nimble..."
		invalidate()
		go func() {
			n, e := discoverNimbleService()
			discoveryResults <- discoveryResponse{n, e}
			procPostMessageW.Call(win, WM_APP_DISCOVERY, 0, 0)
		}()
		return true
	}
	return false
}
func extensionMessage(hwnd uintptr, msg uint32, wp, lp uintptr) bool {
	switch msg {
	case WM_APP_CATALOG:
		res := <-catalogResults
		scanning = false
		if res.err != nil {
			archiveNotice = "Не удалось прочитать архив: " + res.err.Error()
		} else {
			catalog = res.cat
			if len(catalog.Cameras) > 0 && len(catalog.Cameras[0].Ranges) > 0 && !dateFilterActive {
				last := catalog.Cameras[0].Ranges[len(catalog.Cameras[0].Ranges)-1]
				setDateTime(filterDate, time.UnixMilli(last.End-1000).Local())
				dateFilterActive = true
			}
			archiveNotice = fmt.Sprintf("Найдено камер: %d. Выберите период записи.", len(catalog.Cameras))
			refreshArchiveList()
		}
		invalidate()
		return true
	case WM_APP_EXPORT:
		exportDone()
		return true
	case WM_APP_RESTART:
		e := <-restartResults
		watchState.Working = false
		if e != nil {
			watchState.LastError = e.Error()
			watchState.LastMessage = "Ошибка службы: " + e.Error()
			showMsg(e.Error(), "Nimble", 0x10)
		} else {
			watchState.LastMessage = "Команда перезапуска службы завершена"
		}
		invalidate()
		return true
	case WM_APP_NATIVE_SYNC:
		completeNativeSync()
		return true
	case WM_APP_CLOUD:
		r := <-cloudResults
		cloudBusy = false
		if r.err != nil {
			cloudNotice = "Ошибка подключения или неверные параметры"
			showMsg(r.err.Error(), "Проверка облака", 0x30)
		} else {
			cloudNotice = r.result.Message
			showMsg(r.result.Message+"\n\nЭто НЕ проверка наличия архива в облаке. Файлы не отправлялись.", "Ответ облака", 0x40)
		}
		invalidate()
		return true
	case WM_APP_DISCOVERY:
		r := <-discoveryResults
		if r.err != nil {
			watchState.LastMessage = r.err.Error()
		} else {
			watchConf.ServiceName = r.name
			setText(serviceEdit, r.name)
			watchState.LastMessage = "Найдена служба: " + r.name
		}
		invalidate()
		return true
	}
	return false
}
