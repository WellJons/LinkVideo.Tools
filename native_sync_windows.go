//go:build windows

package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const ES_PASSWORD = 0x0020

var (
	syncPasswordEdit  uintptr
	nativeSyncBusy    bool
	nativeSyncNotice  = "Штатная синхронизация ещё не запускалась"
	nativeSyncResults = make(chan nativeSyncReply, 1)
)

type nativeSyncReply struct {
	result NativeSyncResult
	err    error
}

func initNativeSyncControl(hwnd uintptr) {
	syncPasswordEdit, _, _ = procCreateWindowExW.Call(0, ptr("EDIT"), ptr(""), WS_CHILD|WS_TABSTOP|WS_BORDER|ES_AUTOHSCROLL|ES_PASSWORD, 0, 0, 0, 0, hwnd, 1015, 0, 0)
	procSendMessageW.Call(syncPasswordEdit, WM_SETFONT, fontNormal, 1)
}
func positionNativeSyncControl() {
	if syncPasswordEdit == 0 {
		return
	}
	active := page == 2 && archiveSyncView && calendarTarget == ""
	show(syncPasswordEdit, active)
	if active {
		x, w := mainX(), mainW()
		procMoveWindow.Call(syncPasswordEdit, uintptr(x+24), 587, uintptr(w-48), 42, 1)
	}
}
func drawArchiveSync(h uintptr) {
	drawHeader(h, "Выгрузка архива", "Запуск штатной синхронизации LinkVideo.Server 1.1.12 через локальный API")
	x, w := mainX(), mainW()
	action(h, "archive_back", rr(x, 137, 170, 40), "‹  К архиву", false)
	cam, r, ok := selectedArchive()
	card(h, rr(x, 192, w, 128))
	label(h, "ТЕСТОВАЯ ВЫГРУЗКА В ОБЛАКО", x+22, 198, w-50, 22, fontSmall, sysColor("muted"))
	if ok {
		label(h, cameraDisplayName(cam.Name), x+22, 229, w-40, 29, fontBig, sysColor("text"))
		label(h, fmt.Sprintf("Выбранный участок: %s — %s", time.UnixMilli(r.Start).Local().Format("15:04:05"), time.UnixMilli(r.End).Local().Format("15:04:05")), x+22, 262, w-40, 24, fontNormal, sysColor("muted"))
	}
	label(h, "Важно: штатный метод синхронизирует ВСЕ настроенные камеры за выбранное время.", x+22, 291, w-40, 21, fontSmall, sysColor("warn"))
	card(h, rr(x, 331, w, 194))
	label(h, "Период повторной синхронизации", x+22, 338, w-44, 30, fontBold, sysColor("text"))
	half := (w - 60) / 2
	label(h, "Начало", x+24, 376, half, 20, fontSmall, sysColor("muted"))
	label(h, "Конец", x+w/2+10, 376, half, 20, fontSmall, sysColor("muted"))
	action(h, "calendar_start", rr(x+23, 398, half*59/100, 44), formatDay(startDate)+"  ▾", false)
	action(h, "calendar_end", rr(x+w/2+7, 398, half*59/100, 44), formatDay(endDate)+"  ▾", false)
	label(h, "Время (чч:мм:сс)", x+24, 446, half, 19, fontSmall, sysColor("muted"))
	label(h, "Время (чч:мм:сс)", x+w/2+9, 446, half, 19, fontSmall, sysColor("muted"))
	label(h, "Перезаписи существующих данных не гарантируются: решение принимает штатный алгоритм LinkVideo.Server.", x+24, 491, w-50, 28, fontSmall, sysColor("muted"))
	card(h, rr(x, 537, w, viewHeight-615))
	label(h, "Локальный пароль LinkVideo.Server", x+22, 545, w-45, 26, fontBold, sysColor("text"))
	label(h, "Пароль используется однократно и не сохраняется. Операция запускается только по подтверждению.", x+22, 633, w-45, 24, fontSmall, sysColor("muted"))
	status := nativeSyncNotice
	if nativeSyncBusy {
		status = "Выполняется штатная синхронизация — не закрывайте программу"
	}
	label(h, "Статус: "+status, x+22, 661, w-46, 38, fontSmall, sysColor("muted"))
	if nativeSyncBusy {
		action(h, "native_sync_busy", rr(x+w-235, viewHeight-65, 235, 44), "Выполняется…", false)
	} else {
		action(h, "native_sync_start", rr(x+w-235, viewHeight-65, 235, 44), "Запустить выгрузку", true)
	}
}
func startNativeSync() {
	if nativeSyncBusy || scanning || exportBusy {
		return
	}
	_, r, ok := selectedArchive()
	if !ok {
		showMsg("Сначала выберите локальный участок с записью.", "Выгрузка", 0x30)
		return
	}
	from, e1 := combinedPickerDateTime(startDate, startClock)
	to, e2 := combinedPickerDateTime(endDate, endClock)
	if e1 != nil || e2 != nil || !to.After(from) {
		showMsg("Некорректная дата или время.", "Выгрузка", 0x30)
		return
	}
	if from.UnixMilli() < r.Start-1000 || to.UnixMilli() > r.End+1000 {
		showMsg("Указанный диапазон должен находиться внутри локального участка.", "Выгрузка", 0x30)
		return
	}
	if to.Sub(from) > 24*time.Hour {
		showMsg("Максимальный интервал — 24 часа.", "Выгрузка", 0x30)
		return
	}
	pass := strings.TrimSpace(textFrom(syncPasswordEdit))
	if pass == "" {
		showMsg("Введите локальный пароль LinkVideo.Server.", "Выгрузка", 0x30)
		return
	}
	msg := fmt.Sprintf("Запустить штатную синхронизацию LinkVideo.Server за период\n%s — %s?\n\nВНИМАНИЕ: запрос действует на ВСЕ камеры, настроенные в этом экземпляре сервера, а не только на выбранную. Он может начать сетевую выгрузку в облако.\n\nИсходные файлы не изменяются нашей утилитой.", from.Format("02.01.2006 15:04:05"), to.Format("02.01.2006 15:04:05"))
	if showMsg(msg, "Подтверждение выгрузки", MB_YESNO_ICONWARN) != 6 {
		return
	}
	setText(syncPasswordEdit, "")
	nativeSyncBusy = true
	nativeSyncNotice = "Авторизация и запрос штатной синхронизации…"
	invalidate()
	go func(pass string) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		res, e := NativeSynchronize(ctx, nil, "http://127.0.0.1:48518", pass, from, to)
		nativeSyncResults <- nativeSyncReply{res, e}
		procPostMessageW.Call(win, WM_APP_NATIVE_SYNC, 0, 0)
	}(pass)
}
func completeNativeSync() {
	r := <-nativeSyncResults
	nativeSyncBusy = false
	if r.err != nil {
		nativeSyncNotice = "Ошибка локального API: " + r.err.Error()
		showMsg(nativeSyncNotice, "Ошибка синхронизации", 0x30)
	} else {
		nativeSyncNotice = fmt.Sprintf("Локальный API вернул HTTP %d за %s. Проверяйте журнал и облачный архив — завершение отправки ещё НЕ подтверждено.", r.result.LocalHTTPStatus, r.result.Duration.Round(time.Second))
		showMsg("Штатный LinkVideo.Server ответил HTTP 200.\n\nЭто подтверждает только обработку локального запроса, но НЕ успешную загрузку архива в облако. Проверьте журнал LinkVideo.Server и временную шкалу сайта.", "Запрос синхронизации выполнен", 0x40)
	}
	invalidate()
}
