//go:build windows

package main

import (
	"fmt"
	"strings"
	"time"
)

// Archive UI deliberately keeps the controls required by MP4 export, but
// presents them in a separate step instead of cramming them into the timeline.
var (
	archiveExportView bool
	archiveSyncView   bool
	archiveListOffset int
	calendarTarget    string
	calendarMonth     time.Time
)

func archiveDay() time.Time {
	t, err := dateTimeValue(filterDate)
	if err != nil {
		return time.Now()
	}
	return t
}
func formatDay(h uintptr) string {
	t, e := dateTimeValue(h)
	if e != nil {
		return "Выбрать дату"
	}
	return t.Format("02.01.2006")
}
func selectedArchive() (CameraArchive, RecordingRange, bool) {
	if selectedCamera < 0 || selectedCamera >= len(catalog.Cameras) {
		return CameraArchive{}, RecordingRange{}, false
	}
	c := catalog.Cameras[selectedCamera]
	if selectedRange < 0 || selectedRange >= len(c.Ranges) {
		return c, RecordingRange{}, false
	}
	return c, c.Ranges[selectedRange], true
}
func drawArchiveNew(h uintptr) {
	if archiveSyncView {
		drawArchiveSync(h)
	} else if archiveExportView {
		drawArchiveExport(h)
	} else {
		drawArchiveOverview(h)
	}
	if calendarTarget != "" {
		drawCalendar(h)
	}
}
func drawArchiveOverview(h uintptr) {
	x, w := mainX(), mainW()
	drawHeader(h, "Архив камер", "Найденные периоды записи — только подтверждённые данные на диске")
	card(h, rr(x, 138, w, 84))
	label(h, "ПАПКА АРХИВА", x+21, 147, w-30, 20, fontSmall, sysColor("muted"))
	path := strings.TrimSpace(textFrom(pathEdit))
	if path == "" {
		path = "Путь не определён"
	}
	label(h, path, x+21, 175, w-396, 28, fontNormal, sysColor("text"))
	action(h, "auto_path", rr(x+w-373, 164, 92, 42), "Авто", false)
	action(h, "browse_path", rr(x+w-273, 164, 120, 42), "Папка…", false)
	action(h, "scan", rr(x+w-145, 164, 130, 42), "Сканировать", true)

	camW := (w * 58) / 100
	card(h, rr(x, 233, camW-6, 78))
	label(h, "КАМЕРА", x+21, 241, camW-30, 20, fontSmall, sysColor("muted"))
	name := "Найдите локальные записи"
	if selectedCamera >= 0 && selectedCamera < len(catalog.Cameras) {
		name = cameraDisplayName(catalog.Cameras[selectedCamera].Name)
	}
	label(h, name, x+21, 264, camW-164, 34, fontBig, sysColor("text"))
	action(h, "camera_prev", rr(x+camW-141, 252, 52, 43), "‹", false)
	action(h, "camera_next", rr(x+camW-82, 252, 55, 43), "›", false)

	dateX := x + camW + 7
	card(h, rr(dateX, 233, w-camW-7, 78))
	label(h, "ДАТА ЗАПИСИ", dateX+17, 241, 200, 20, fontSmall, sysColor("muted"))
	action(h, "day_prev", rr(dateX+16, 262, 41, 39), "‹", false)
	action(h, "calendar_filter", rr(dateX+64, 262, w-camW-144, 39), formatDay(filterDate)+"   ▾", false)
	action(h, "day_next", rr(x+w-62, 262, 43, 39), "›", false)

	card(h, rr(x, 322, w, 178))
	label(h, "Записи за выбранный день", x+21, 328, w/2, 36, fontBold, sysColor("text"))
	day := archiveDay()
	var tl LocalTimeline
	if selectedCamera >= 0 && selectedCamera < len(catalog.Cameras) {
		tl = timelineForCamera(catalog.Cameras[selectedCamera], day, true)
	}
	statsColor := sysColor("text")
	if len(tl.Ranges) == 0 {
		statsColor = sysColor("muted")
	}
	label(h, "ЗАПИСАНО", x+22, 367, 140, 18, fontSmall, sysColor("muted"))
	label(h, humanDuration(tl.AvailableMs), x+22, 387, 255, 35, fontBig, statsColor)
	label(h, "РАЗРЫВЫ МЕЖДУ ЗАПИСЯМИ", x+285, 367, w-310, 18, fontSmall, sysColor("muted"))
	label(h, humanDuration(tl.MissingMs), x+285, 387, w-320, 35, fontBig, sysColor("text"))
	bx, bw, by := x+22, w-44, int32(437)
	fill(h, rr(bx, by, bw, 16), sysColor("border"), 6)
	startOfDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	low, span := startOfDay.UnixMilli(), startOfDay.AddDate(0, 0, 1).UnixMilli()-startOfDay.UnixMilli()
	if span <= 0 {
		span = 86400000
	}
	for _, r := range tl.Ranges {
		left := bx + int32((r.Start-low)*int64(bw)/span)
		right := bx + int32((r.End-low)*int64(bw)/span)
		if right <= left {
			right = left + 1
		}
		if left < bx {
			left = bx
		}
		if right > bx+bw {
			right = bx + bw
		}
		if right > left {
			fill(h, rr(left, by, right-left, 16), sysColor("good"), 0)
		}
	}
	for i := 0; i <= 4; i++ {
		px := bx + bw*int32(i)/4
		tx := px - 22
		if i == 0 {
			tx = bx
		} else if i == 4 {
			tx = bx + bw - 50
		}
		label(h, fmt.Sprintf("%02d:00", i*6), tx, by+22, 55, 20, fontSmall, sysColor("muted"))
	}
	label(h, "Зелёный — локальная запись. Серый — запись не подтверждена.", bx, by+44, bw, 17, fontSmall, sysColor("muted"))

	// At most three rows fit in the default-height window; pagination keeps
	// the panel readable at 100–150% Windows display scaling.
	listTop := int32(511)
	listBottom := viewHeight - 83
	card(h, rr(x, listTop, w, listBottom-listTop))
	label(h, "Фрагменты и пропуски", x+21, listTop+7, 390, 32, fontBold, sysColor("text"))
	total := len(displayedRanges)
	visibleRows := maxInt(2, minInt(3, int((listBottom-(listTop+44))/49)))
	if total > visibleRows {
		label(h, fmt.Sprintf("%d–%d из %d", archiveListOffset+1, minInt(archiveListOffset+visibleRows, total), total), x+w-282, listTop+11, 95, 24, fontSmall, sysColor("muted"))
		action(h, "rows_prev", rr(x+w-167, listTop+7, 60, 32), "‹", false)
		action(h, "rows_next", rr(x+w-100, listTop+7, 70, 32), "›", false)
	}
	if total == 0 {
		label(h, "Записей за этот день не найдено. Выберите другую дату.", x+24, listTop+70, w-45, 42, fontNormal, sysColor("muted"))
	}
	maxRows := minInt(visibleRows, total-archiveListOffset)
	for i := 0; i < maxRows; i++ {
		id := archiveListOffset + i
		ri := displayedRanges[id]
		y := listTop + 44 + int32(i)*49
		selected := ri >= 0 && ri == selectedRange
		bg := sysColor("panelAlt")
		if selected {
			bg = sysColor("accentSoft")
		}
		fill(h, rr(x+18, y, w-36, 43), bg, 9)
		if ri >= 0 && selectedCamera >= 0 && selectedCamera < len(catalog.Cameras) {
			r := catalog.Cameras[selectedCamera].Ranges[ri]
			dot(h, x+33, y+17, sysColor("good"))
			label(h, fmt.Sprintf("%s – %s", time.UnixMilli(r.Start).Local().Format("15:04:05"), time.UnixMilli(r.End).Local().Format("15:04:05")), x+57, y+6, 290, 30, fontBold, sysColor("text"))
			label(h, humanDuration(r.End-r.Start), x+353, y+6, 190, 30, fontNormal, sysColor("muted"))
			label(h, "Локально", x+w-130, y+6, 110, 30, fontSmall, sysColor("good"))
		} else if id > 0 && id+1 < len(displayedRanges) && selectedCamera < len(catalog.Cameras) {
			// A gap entry always appears immediately before the next range.
			next := displayedRanges[id+1]
			prev := displayedRanges[id-1]
			if next >= 0 && prev >= 0 {
				a := catalog.Cameras[selectedCamera].Ranges[prev].End
				b := catalog.Cameras[selectedCamera].Ranges[next].Start
				dot(h, x+33, y+17, sysColor("warn"))
				label(h, fmt.Sprintf("%s – %s", time.UnixMilli(a).Local().Format("15:04:05"), time.UnixMilli(b).Local().Format("15:04:05")), x+57, y+6, 290, 30, fontNormal, sysColor("muted"))
				label(h, humanDuration(b-a), x+353, y+6, 190, 30, fontNormal, sysColor("muted"))
				label(h, "Пробел", x+w-130, y+6, 110, 30, fontSmall, sysColor("warn"))
			}
		}
		areas = append(areas, region{rr(x+18, y, w-36, 43), fmt.Sprintf("archive_row_%d", id)})
	}
	label(h, archiveNotice, x+18, viewHeight-73, w-455, 24, fontSmall, sysColor("muted"))
	action(h, "archive_export", rr(x+w-422, viewHeight-75, 193, 44), "Скачать MP4", false)
	action(h, "archive_sync", rr(x+w-219, viewHeight-75, 219, 44), "Выгрузить в облако", true)
}
func drawArchiveExport(h uintptr) {
	drawHeader(h, "Экспорт записи", "Выберите точные границы видео или проверьте подключение к архивному API")
	x, w := mainX(), mainW()
	action(h, "archive_back", rr(x, 137, 165, 39), "‹   К архиву", false)
	cam, r, ok := selectedArchive()
	card(h, rr(x, 190, w, 100))
	label(h, "ВЫБРАННАЯ КАМЕРА", x+22, 196, w-45, 20, fontSmall, sysColor("muted"))
	camLabel := "Фрагмент не выбран"
	if ok {
		camLabel = cameraDisplayName(cam.Name)
	}
	label(h, camLabel, x+22, 220, w-46, 28, fontBig, sysColor("text"))
	if ok {
		label(h, fmt.Sprintf("Локальная запись: %s — %s  ·  %s", time.UnixMilli(r.Start).Local().Format("02.01.2006 15:04:05"), time.UnixMilli(r.End).Local().Format("15:04:05"), humanDuration(r.End-r.Start)), x+22, 254, w-46, 23, fontNormal, sysColor("muted"))
	}
	card(h, rr(x, 303, w, 222))
	label(h, "Границы экспорта", x+22, 313, w-45, 33, fontBold, sysColor("text"))
	label(h, "Начало", x+24, 356, w/2-30, 19, fontSmall, sysColor("muted"))
	label(h, "Конец", x+w/2+9, 356, w/2-36, 19, fontSmall, sysColor("muted"))
	leftWidth := (w - 60) / 2
	action(h, "calendar_start", rr(x+23, 384, leftWidth*59/100, 47), formatDay(startDate)+"   ▾", false)
	action(h, "calendar_end", rr(x+w/2+7, 384, leftWidth*59/100, 47), formatDay(endDate)+"   ▾", false)
	label(h, "Время", x+24, 449, 150, 23, fontSmall, sysColor("muted"))
	label(h, "Время", x+w/2+9, 449, 150, 23, fontSmall, sysColor("muted"))
	label(h, "Точность обрезки ограничена видеофрагментами Nimble (~20 секунд).", x+23, 490, w-45, 21, fontSmall, sysColor("muted"))
	card(h, rr(x, 537, w, 97))
	label(h, "Сохранить на компьютер", x+21, 545, 440, 27, fontBold, sysColor("text"))
	label(h, "MP4 создаётся локально. Исходные .dat и индекс не изменяются.", x+21, 576, w-285, 35, fontSmall, sysColor("muted"))
	action(h, "make_mp4", rr(x+w-231, 564, 209, 48), "Создать MP4", true)
	card(h, rr(x, 626, w, viewHeight-648))
	label(h, "Диагностика архивного API", x+20, 630, w-272, 30, fontBold, sysColor("text"))
	label(h, "Статус: "+cloudNotice, x+20, 658, w-278, 37, fontSmall, sysColor("muted"))
	label(h, "Только запрос. Файлы не отправляются.", x+20, viewHeight-54, w-275, 24, fontSmall, sysColor("muted"))
	action(h, "cloud_probe", rr(x+w-240, 668, 217, 43), "Проверить API", false)
}

func setArchiveDay(d time.Time) {
	setDateTime(filterDate, d)
	dateFilterActive = true
	archiveListOffset = 0
	refreshArchiveList()
	invalidate()
}
func archiveViewAction(id string) bool {
	switch id {
	case "native_sync_start":
		startNativeSync()
		return true
	case "native_sync_cancel":
		stopNativeSyncWait()
		return true
	case "native_sync_stop_watch":
		nativeSyncTracking = false
		nativeSyncResultText = "Наблюдение остановлено вручную; это не отменяет задания сервера"
		invalidate()
		return true
	case "archive_sync":
		_, _, ok := selectedArchive()
		if !ok {
			showMsg("Выберите участок с локальной записью.", "Выгрузка", 0x30)
			return true
		}
		archiveSyncView = true
		archiveExportView = false
		positionInputs()
		invalidate()
		return true
	case "archive_export":
		_, _, ok := selectedArchive()
		if !ok {
			showMsg("Выберите зелёный участок с записью.", "Экспорт", 0x30)
			return true
		}
		archiveExportView = true
		archiveSyncView = false
		positionInputs()
		invalidate()
		return true
	case "archive_back":
		archiveSyncView = false
		archiveExportView = false
		positionInputs()
		invalidate()
		return true
	case "day_prev":
		setArchiveDay(archiveDay().AddDate(0, 0, -1))
		return true
	case "day_next":
		setArchiveDay(archiveDay().AddDate(0, 0, 1))
		return true
	case "rows_prev":
		archiveListOffset -= archivePageSize()
		if archiveListOffset < 0 {
			archiveListOffset = 0
		}
		invalidate()
		return true
	case "rows_next":
		archiveListOffset += archivePageSize()
		if archiveListOffset >= len(displayedRanges) {
			archiveListOffset = maxInt(0, len(displayedRanges)-archivePageSize())
		}
		invalidate()
		return true
	case "calendar_filter", "calendar_start", "calendar_end":
		calendarTarget = strings.TrimPrefix(id, "calendar_")
		var target uintptr
		switch calendarTarget {
		case "filter":
			target = filterDate
		case "start":
			target = startDate
		case "end":
			target = endDate
		}
		calendarMonth, _ = dateTimeValue(target)
		positionInputs()
		if calendarMonth.IsZero() {
			calendarMonth = time.Now()
		}
		invalidate()
		return true
	}
	if strings.HasPrefix(id, "archive_row_") {
		var index int
		if _, e := fmt.Sscanf(id, "archive_row_%d", &index); e == nil {
			selectArchiveRange(index)
			invalidate()
		}
		return true
	}
	return false
}
func archivePageSize() int { return maxInt(2, minInt(3, int(((viewHeight-83)-(511+44))/49))) }
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func drawCalendar(h uintptr) {
	x, w := mainX(), mainW()
	px, py := x+w-360, int32(303)
	if calendarTarget == "start" {
		px = x + 25
		py = 330
	}
	if calendarTarget == "end" {
		px = x + w - 341
		py = 330
	}
	const cw, ch = int32(320), int32(312)
	outline(h, rr(px, py, cw, ch), sysColor("border"), 18, sysColor("panel"))
	action(h, "cal_prev", rr(px+17, py+17, 40, 36), "‹", false)
	action(h, "cal_next", rr(px+263, py+17, 40, 36), "›", false)
	monthNames := []string{"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"}
	center(h, fmt.Sprintf("%s %d", monthNames[int(calendarMonth.Month())-1], calendarMonth.Year()), rr(px+59, py+19, 202, 32), fontBold, sysColor("text"))
	weeks := []string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}
	for i, name := range weeks {
		center(h, name, rr(px+13+int32(i)*42, py+60, 42, 25), fontSmall, sysColor("muted"))
	}
	first := time.Date(calendarMonth.Year(), calendarMonth.Month(), 1, 0, 0, 0, 0, calendarMonth.Location())
	offset := (int(first.Weekday()) + 6) % 7
	// Strictly render the current month; no misleading neighbouring-month dates.
	for d := 1; d <= 31; d++ {
		dt := time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, first.Location())
		if dt.Month() != first.Month() {
			break
		}
		slot := offset + d - 1
		row, col := slot/7, slot%7
		rx := px + 13 + int32(col)*42
		ry := py + 93 + int32(row)*31
		bg := sysColor("panel")
		fg := sysColor("text")
		var control uintptr
		switch calendarTarget {
		case "filter":
			control = filterDate
		case "start":
			control = startDate
		case "end":
			control = endDate
		}
		chosen, err := dateTimeValue(control)
		if err == nil && chosen.Year() == dt.Year() && chosen.YearDay() == dt.YearDay() {
			bg = sysColor("accent")
			fg = "#FFFFFF"
		}
		fill(h, rr(rx+4, ry+1, 34, 29), bg, 8)
		if calendarHasArchive(dt) && bg == sysColor("panel") {
			fill(h, rr(rx+4, ry+1, 34, 29), sysColor("accentSoft"), 8)
		}
		center(h, fmt.Sprint(d), rr(rx+4, ry+1, 34, 26), fontSmall, fg)
		if calendarHasArchive(dt) {
			fill(h, rr(rx+19, ry+26, 5, 5), sysColor("good"), 5)
		}
		areas = append(areas, region{rr(rx+1, ry, 40, 31), fmt.Sprintf("cal_day_%d", d)})
	}
	action(h, "cal_today", rr(px+17, py+267, 126, 34), "Сегодня", false)
	action(h, "cal_close", rr(px+156, py+267, 146, 34), "Закрыть", false)
}
func calendarAction(id string) bool {
	if calendarTarget == "" {
		return false
	}
	switch id {
	case "cal_close":
		calendarTarget = ""
		positionInputs()
		invalidate()
		return true
	case "cal_prev":
		calendarMonth = calendarMonth.AddDate(0, -1, 0)
		invalidate()
		return true
	case "cal_next":
		calendarMonth = calendarMonth.AddDate(0, 1, 0)
		invalidate()
		return true
	case "cal_today":
		calendarChoose(time.Now())
		return true
	}
	if strings.HasPrefix(id, "cal_day_") {
		var d int
		if _, e := fmt.Sscanf(id, "cal_day_%d", &d); e == nil {
			calendarChoose(time.Date(calendarMonth.Year(), calendarMonth.Month(), d, 0, 0, 0, 0, calendarMonth.Location()))
		}
		return true
	}
	return false
}
func calendarChoose(t time.Time) {
	switch calendarTarget {
	case "filter":
		setArchiveDay(t)
	case "start":
		setDateTime(startDate, t)
	case "end":
		setDateTime(endDate, t)
	}
	calendarTarget = ""
	positionInputs()
	invalidate()
}

// Highlight only dates for which the local index and referenced video bytes exist.
func calendarHasArchive(day time.Time) bool {
	if selectedCamera < 0 || selectedCamera >= len(catalog.Cameras) {
		return false
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).UnixMilli()
	end := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, day.Location()).UnixMilli()
	for _, r := range catalog.Cameras[selectedCamera].Ranges {
		if r.Start < end && r.End > start {
			return true
		}
	}
	return false
}