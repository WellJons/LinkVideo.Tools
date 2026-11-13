//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const ES_PASSWORD = 0x0020

var (
	syncPasswordEdit       uintptr
	nativeSyncBusy         bool
	nativeSyncTracking     bool
	nativeSyncStarted      time.Time
	nativeSyncObserveUntil time.Time
	nativeSyncCancel       context.CancelFunc
	nativeSyncResultText   = "Локальный запрос ещё не выполнялся"
	nativeSyncLogs         SyncLogProgress
	nativeSyncResults      = make(chan nativeSyncReply, 1)
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
	visible := page == 2 && archiveSyncView && calendarTarget == ""
	show(syncPasswordEdit, visible)
	if visible {
		x, w := mainX(), mainW()
		procMoveWindow.Call(syncPasswordEdit, uintptr(x+22), 455, uintptr(w-44), 33, 1)
	}
}

func drawArchiveSync(h uintptr) {
	drawHeader(h, "Выгрузка архива", "Запуск штатной синхронизации и диагностика по журналу LinkVideo.Server")
	x, w := mainX(), mainW()
	action(h, "archive_back", rr(x, 136, 170, 36), "‹  К архиву", false)
	cam, r, ok := selectedArchive()
	card(h, rr(x, 178, w, 90))
	label(h, "КАМЕРА", x+20, 181, w-43, 19, fontSmall, sysColor("muted"))
	if ok {
		label(h, cameraDisplayName(cam.Name), x+20, 198, w-44, 27, fontBig, sysColor("text"))
		label(h, fmt.Sprintf("Локально: %s — %s", time.UnixMilli(r.Start).Local().Format("02.01 15:04:05"), time.UnixMilli(r.End).Local().Format("02.01 15:04:05")), x+20, 227, w-43, 21, fontNormal, sysColor("muted"))
	} else {
		label(h, "Сначала выберите локальный участок", x+20, 199, w-44, 28, fontNormal, sysColor("muted"))
	}
	label(h, "ВНИМАНИЕ: штатный API обрабатывает ВСЕ камеры этого сервера", x+20, 248, w-44, 18, fontSmall, sysColor("warn"))

	card(h, rr(x, 278, w, 140))
	label(h, "Период синхронизации", x+20, 282, w-44, 27, fontBold, sysColor("text"))
	half := (w - 60) / 2
	label(h, "Начало", x+22, 310, half, 19, fontSmall, sysColor("muted"))
	label(h, "Конец", x+w/2+8, 310, half, 19, fontSmall, sysColor("muted"))
	action(h, "calendar_start", rr(x+21, 329, half*65/100, 36), formatDay(startDate)+"  ▾", false)
	action(h, "calendar_end", rr(x+w/2+5, 329, half*65/100, 36), formatDay(endDate)+"  ▾", false)
	label(h, "Время (чч:мм:сс)", x+22, 367, half, 17, fontSmall, sysColor("muted"))
	label(h, "Время (чч:мм:сс)", x+w/2+7, 367, half, 17, fontSmall, sysColor("muted"))

	card(h, rr(x, 427, w, 69))
	label(h, "Локальный пароль LinkVideo.Server", x+21, 430, w-44, 24, fontBold, sysColor("text"))

	diagBottom := viewHeight - 66
	if diagBottom < 638 {
		diagBottom = 638
	}
	card(h, rr(x, 505, w, diagBottom-505))
	phase := "Ожидается запуск"
	detail := "Статусы будут определяться по журналам оригинального сервера"
	counters := "Пропуски: —  Токен: —  DAT: —  Индекс: —"
	if nativeSyncTracking || !nativeSyncStarted.IsZero() {
		phase = nativeSyncLogs.Phase
		detail = nativeSyncLogs.Detail
		counters = fmt.Sprintf("Пропуски: %d  |  Токен: %d запросов, %d ошибок  |  DAT: %d  |  Индекс: %d",
			nativeSyncLogs.TimelineRequests, nativeSyncLogs.TokenRequests, nativeSyncLogs.TokenFailures, nativeSyncLogs.DATSendStarts, nativeSyncLogs.INDEXSendStarts)
		if nativeSyncLogs.Warning != "" {
			detail = nativeSyncLogs.Warning
		}
	}
	label(h, "Диагностика выгрузки", x+20, 509, w-43, 24, fontBold, sysColor("text"))
	phaseColor := sysColor("text")
	if nativeSyncLogs.Phase == "Не удалось получить upload-token" || nativeSyncLogs.Phase == "Ошибка токена — повторная попытка" || nativeSyncLogs.Phase == "Ошибка отправки файлов" {
		phaseColor = sysColor("warn")
	}
	label(h, phase, x+20, 533, w-43, 26, fontNormal, phaseColor)
	label(h, detail, x+20, 562, w-43, 22, fontSmall, sysColor("muted"))
	label(h, counters, x+20, 587, w-43, 19, fontSmall, sysColor("muted"))
	if diagBottom > 627 {
		label(h, nativeSyncResultText, x+20, 610, w-43, 21, fontSmall, sysColor("muted"))
	}
	if diagBottom > 653 {
		label(h, "Логи сопоставляются по времени; фоновые задачи могут учитываться тоже", x+20, 634, w-43, 18, fontSmall, sysColor("muted"))
	}

	btnY := viewHeight - 57
	if nativeSyncBusy {
		action(h, "native_sync_cancel", rr(x+w-242, btnY, 242, 42), "Прервать ожидание", false)
	} else if nativeSyncTracking {
		action(h, "native_sync_stop_watch", rr(x+w-242, btnY, 242, 42), "Закончить наблюдение", false)
	} else {
		action(h, "native_sync_start", rr(x+w-242, btnY, 242, 42), "Запустить синхронизацию", true)
	}
}

func startNativeSync() {
	if nativeSyncBusy || nativeSyncTracking || scanning || exportBusy {
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
		showMsg("Диапазон должен находиться внутри локального участка.", "Выгрузка", 0x30)
		return
	}
	if to.Sub(from) > 24*time.Hour {
		showMsg("Максимальный интервал — 24 часа.", "Выгрузка", 0x30)
		return
	}
	pass := textFrom(syncPasswordEdit)
	if pass == "" {
		showMsg("Введите локальный пароль LinkVideo.Server.", "Выгрузка", 0x30)
		return
	}
	msg := fmt.Sprintf("Запустить штатную синхронизацию LinkVideo.Server за период\n%s — %s?\n\nВНИМАНИЕ: запрос действует на ВСЕ настроенные камеры.\nДалее диагностика будет читать сообщения из журналов, которые могут относиться и к фоновым задачам.\n\nИсходные .dat нашей утилитой не изменяются.", from.Format("02.01.2006 15:04:05"), to.Format("02.01.2006 15:04:05"))
	if showMsg(msg, "Подтверждение синхронизации", MB_YESNO_ICONWARN) != 6 {
		return
	}
	setText(syncPasswordEdit, "")
	nativeSyncBusy = true
	nativeSyncTracking = true
	nativeSyncStarted = time.Now().Add(-2 * time.Second)
	nativeSyncObserveUntil = time.Time{}
	nativeSyncLogs = SyncLogProgress{Phase: "Запрос отправлен; ожидаем журнал", Detail: "Наблюдаем за выдачей токена и передачей файлов"}
	nativeSyncResultText = "Ожидание локального API ограничено 2 минутами"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	nativeSyncCancel = cancel
	invalidate()
	go func(pass string) {
		defer cancel()
		res, e := NativeSynchronize(ctx, nil, "http://127.0.0.1:48518", pass, from, to)
		nativeSyncResults <- nativeSyncReply{res, e}
		procPostMessageW.Call(win, WM_APP_NATIVE_SYNC, 0, 0)
	}(pass)
}

func pollNativeSyncDiagnostics() {
	if !nativeSyncTracking {
		return
	}
	if !nativeSyncBusy && !nativeSyncObserveUntil.IsZero() && time.Now().After(nativeSyncObserveUntil) {
		nativeSyncTracking = false
		if nativeSyncLogs.DATSendStarts == 0 && nativeSyncLogs.INDEXSendStarts == 0 {
			nativeSyncResultText = "Наблюдение завершено: подтверждения передачи файлов нет"
		} else {
			nativeSyncResultText = "Наблюдение завершено: приём файлов облаком не подтверждён"
		}
		invalidate()
		return
	}
	next := ScanSyncLog(installLogFolder(), nativeSyncStarted)
	// Avoid unnecessary paints while idle; no raw lines or credentials ever shown.
	if next.StatusCode() != nativeSyncLogs.StatusCode() || next.Phase != nativeSyncLogs.Phase || next.SafeCounters() != nativeSyncLogs.SafeCounters() || next.Warning != nativeSyncLogs.Warning || next.Detail != nativeSyncLogs.Detail {
		nativeSyncLogs = next
		invalidate()
	}
}

func stopNativeSyncWait() {
	if !nativeSyncBusy {
		return
	}
	if showMsg("Прервать только ожидание ответа в LinkVideo.Tools?\n\nЭто НЕ остановит фоновую синхронизацию в LinkVideo.Server. Наблюдение за журналом продолжится.", "Отменить ожидание", MB_YESNO_ICONWARN) != 6 {
		return
	}
	nativeSyncResultText = "Ожидание отменяется; задача на LinkVideo.Server может продолжаться"
	if nativeSyncCancel != nil {
		nativeSyncCancel()
	}
	invalidate()
}

func completeNativeSync() {
	r := <-nativeSyncResults
	nativeSyncBusy = false
	nativeSyncCancel = nil
	nativeSyncObserveUntil = time.Now().Add(3 * time.Minute)
	if r.err != nil {
		switch {
		case errors.Is(r.err, context.Canceled):
			nativeSyncResultText = "Ожидание отменено; запуск задания на сервере не отменён"
		case errors.Is(r.err, context.DeadlineExceeded):
			nativeSyncResultText = "Тайм-аут ответа API (2 мин); смотри этапы по журналу"
		default:
			nativeSyncResultText = "Локальный запрос: " + r.err.Error()
		}
	} else {
		nativeSyncResultText = fmt.Sprintf("Локальный API HTTP %d (%s). Облако НЕ подтверждено", r.result.LocalHTTPStatus, r.result.Duration.Round(time.Second))
	}
	pollNativeSyncDiagnostics()
	invalidate()
}