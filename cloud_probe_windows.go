//go:build windows

package main

import (
	"context"
	"fmt"
	"time"
)

type cloudReply struct {
	result CloudProbeResult
	err    error
}

func startCloudProbe() {
	if cloudBusy || scanning {
		return
	}
	if selectedCamera < 0 || selectedCamera >= len(catalog.Cameras) || selectedRange < 0 {
		showMsg("Выберите участок, на котором есть локальная запись.", "Облако", 0x30)
		return
	}
	from, e1 := combinedPickerDateTime(startDate, startClock)
	to, e2 := combinedPickerDateTime(endDate, endClock)
	if e1 != nil || e2 != nil || !to.After(from) {
		showMsg("Укажите корректные начало и конец.", "Облако", 0x30)
		return
	}
	rangeNow := catalog.Cameras[selectedCamera].Ranges[selectedRange]
	if from.UnixMilli() < rangeNow.Start-1000 || to.UnixMilli() > rangeNow.End+1000 {
		showMsg("Диапазон должен находиться внутри выбранного локального участка.", "Облако", 0x30)
		return
	}
	cameraID, e := cameraNumericID(catalog.Cameras[selectedCamera].Name)
	if e != nil {
		showMsg(e.Error(), "Облако", 0x30)
		return
	}
	if showMsg(fmt.Sprintf("Проверить, отвечает ли облачный API на запрос интервала камеры %d?\n\nЭто только чтение: отправки .dat и изменения архива не будет.", cameraID), "Проверка облака", MB_YESNO_ICONWARN) != 6 {
		return
	}
	conf := localPublicConfig()
	cloudBusy = true
	cloudNotice = "Запрос к облаку…"
	invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 16*time.Second)
		defer cancel()
		res, err := probeTimelineGaps(ctx, nil, conf.DatServerProxy, cameraID, from.Unix(), to.Unix(), conf.Operator)
		cloudResults <- cloudReply{res, err}
		procPostMessageW.Call(win, WM_APP_CLOUD, 0, 0)
	}()
}
