//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const WM_APP_NETWORK = 0x8007

type networkResponse struct {
	kind    string
	devices []DiscoveredDevice
	wifi    []WiFiBSS
	probe   CameraProbe
	err     error
}

var (
	networkIPControl, networkLogControl uintptr
	networkResults = make(chan networkResponse, 1)
	networkBusy bool
	networkStatus = "Доступны поиск ONVIF / SADP / DHIP, проверка IP и сканирование Wi-Fi."
	networkDevices []DiscoveredDevice
	networkWiFi []WiFiBSS
	networkReport string
	discoveryReport string
	wifiReport string
	probeReport string
	networkDeviceOffset int
	networkWiFiOffset int
)

func initNetworkControls(hwnd uintptr) {
	networkIPControl = nativeEdit(hwnd, 1020, "")
	networkLogControl, _, _ = procCreateWindowExW.Call(0, ptr("EDIT"), ptr("После диагностики здесь появятся результаты."),
		WS_CHILD|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|ES_READONLY, 0, 0, 0, 0, hwnd, 1021, 0, 0)
	procSendMessageW.Call(networkLogControl, WM_SETFONT, fontSmall, 1)
}

func positionNetworkControls() {
	x, w := mainX(), mainW()
	show(networkIPControl, page == 6)
	show(networkLogControl, page == 6 || page == 7)
	if page == 6 {
		procMoveWindow.Call(networkIPControl, uintptr(x+22), 547, 258, 37, 1)
		procMoveWindow.Call(networkLogControl, uintptr(x+20), 619, uintptr(w-40),
			uintptr(maxInt32(68, viewHeight-676)), 1)
	} else if page == 7 {
		procMoveWindow.Call(networkLogControl, uintptr(x+20), 631, uintptr(w-40),
			uintptr(maxInt32(64, viewHeight-686)), 1)
	}
}

func drawDiscoveryPage(h uintptr) {
	x, w := mainX(), mainW()
	drawHeader(h, "Сеть и камеры", "Поиск по LAN без паролей; диагностика выбранного устройства")
	card(h, rr(x, 138, w, 91))
	label(h, "Обнаружение камер и регистраторов", x+21, 149, w-45, 27, fontBold, sysColor("text"))
	label(h, networkStatus, x+23, 187, w-348, 26, fontSmall, sysColor("muted"))
	action(h, "network_discover", rr(x+w-310, 158, 291, 45), "Найти ONVIF / SADP / DHIP", true)

	card(h, rr(x, 241, w, 266))
	label(h, fmt.Sprintf("Обнаруженные устройства: %d", len(networkDevices)), x+22, 251, w-45, 30, fontBold, sysColor("text"))
	action(h, "network_device_prev", rr(x+w-127, 250, 48, 34), "‹", false)
	action(h, "network_device_next", rr(x+w-71, 250, 48, 34), "›", false)
	if len(networkDevices) == 0 {
		label(h, "Нажмите «Найти». Отсутствие ответа не означает, что камер в сети нет.", x+23, 313, w-46, 31, fontNormal, sysColor("muted"))
	}
	for i := 0; i < 4 && networkDeviceOffset+i < len(networkDevices); i++ {
		d := networkDevices[networkDeviceOffset+i]
		y := int32(294+i*49)
		fill(h, rr(x+16, y, w-32, 43), sysColor("panelAlt"), 8)
		label(h, d.IP, x+29, y+4, 158, 33, fontBold, sysColor("text"))
		name := d.Model
		if name == "" {
			name = d.Name
		}
		if name == "" {
			name = "Без модели"
		}
		label(h, name, x+196, y+4, 190, 33, fontNormal, sysColor("text"))
		label(h, d.Protocol, x+398, y+4, w-580, 33, fontSmall, sysColor("muted"))
		action(h, fmt.Sprintf("network_select_%d", networkDeviceOffset+i), rr(x+w-157, y+4, 140, 35), "Проверить IP", false)
	}

	card(h, rr(x, 518, w, maxInt32(188, viewHeight-570)))
	label(h, "Диагностика выбранного IP", x+22, 527, w-44, 27, fontBold, sysColor("text"))
	label(h, "IP-адрес (без URL и пароля)", x+304, 552, w-470, 27, fontSmall, sysColor("muted"))
	action(h, "network_probe", rr(x+w-174, 543, 153, 43), "Проверить", true)
	label(h, "Результаты диагностики", x+22, 587, w-45, 23, fontSmall, sysColor("muted"))
	dateLabel(h)
}

func drawWiFiPage(h uintptr) {
	x, w := mainX(), mainW()
	drawHeader(h, "Wi-Fi диагностика", "Соседние точки доступа, RSSI, каналы, BSSID — через Windows WLAN API")
	card(h, rr(x, 139, w, 88))
	label(h, fmt.Sprintf("Обнаружено BSSID: %d", len(networkWiFi)), x+22, 150, w-45, 32, fontBold, sysColor("text"))
	label(h, networkStatus, x+22, 189, w-235, 22, fontSmall, sysColor("muted"))
	action(h, "network_wifi_scan", rr(x+w-199, 158, 175, 43), "Сканировать Wi-Fi", true)

	card(h, rr(x, 239, w, 358))
	label(h, "Найденные точки доступа", x+20, 247, w-46, 30, fontBold, sysColor("text"))
	action(h, "network_wifi_prev", rr(x+w-125, 246, 46, 34), "‹", false)
	action(h, "network_wifi_next", rr(x+w-69, 246, 46, 34), "›", false)
	if len(networkWiFi) == 0 {
		label(h, "Данные появятся после сканирования. Нужен Wi-Fi-адаптер.", x+22, 302, w-45, 29, fontNormal, sysColor("muted"))
	}
	for i := 0; i < 6 && networkWiFiOffset+i < len(networkWiFi); i++ {
		n := networkWiFi[networkWiFiOffset+i]
		y := int32(289+i*49)
		fill(h, rr(x+16, y, w-32, 43), sysColor("panelAlt"), 8)
		label(h, n.SSID, x+24, y+3, 180, 22, fontBold, sysColor("text"))
		label(h, n.BSSID, x+24, y+22, 180, 18, fontSmall, sysColor("muted"))
		label(h, fmt.Sprintf("%s  •  канал %d", n.Band, n.Channel), x+216, y+4, 225, 30, fontNormal, sysColor("text"))
		label(h, fmt.Sprintf("%d dBm  •  %d%%", n.RSSI, n.Quality), x+462, y+4, 190, 30, fontNormal, sysColor("text"))
	}
	card(h, rr(x, 608, w, maxInt32(103, viewHeight-660)))
	label(h, "Результаты и ограничения диагностики", x+20, 608, w-45, 22, fontSmall, sysColor("muted"))
	dateLabel(h)
}

func beginNetworkOperation(kind string) {
	if networkBusy {
		return
	}
	networkBusy = true
	networkStatus = "Идёт диагностика: " + kind
	invalidate()
	target := textFrom(networkIPControl)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 14*time.Second)
		defer cancel()
		resp := networkResponse{kind: kind}
		switch kind {
		case "discover":
			resp.devices, resp.err = DiscoverCameras(ctx, 4*time.Second)
		case "wifi":
			resp.wifi, resp.err = scanWiFiNetworks()
		case "probe":
			resp.probe, resp.err = ProbeCamera(ctx, target)
		}
		networkResults <- resp
		procPostMessageW.Call(win, WM_APP_NETWORK, 0, 0)
	}()
}

func updateNetworkReport() {
	var b strings.Builder
	b.WriteString("LinkVideo.Tools — диагностика сети и камер\r\n")
	b.WriteString("Протоколы поиска: ONVIF WS-Discovery, Hikvision SADP, Dahua DHIP.\r\n")
	b.WriteString("Проверка камер не выполняет авторизацию, изменение настроек и получение видео.\r\n\r\n")
	for _, section := range []string{discoveryReport, wifiReport, probeReport} {
		if section != "" {
			b.WriteString(section)
			b.WriteString("\r\n")
		}
	}
	networkReport = b.String()
	setText(networkLogControl, networkReport)
	exportText = snapshotReport(current) + "\r\n" + networkReport
	setText(reportEdit, exportText)
}

func networkDone() {
	res := <-networkResults
	networkBusy = false
	if res.err != nil {
		networkStatus = "Ошибка: " + res.err.Error()
	} else {
		switch res.kind {
		case "discover":
			networkDevices = res.devices
			networkDeviceOffset = 0
			networkStatus = fmt.Sprintf("Поиск завершён. Найдено: %d", len(res.devices))
			discoveryReport = discoverySummary(res.devices)
		case "wifi":
			networkWiFi = res.wifi
			networkWiFiOffset = 0
			networkStatus = fmt.Sprintf("Wi-Fi: обнаружено BSSID: %d", len(res.wifi))
			wifiReport = wifiSummary(res.wifi)
		case "probe":
			networkStatus = "Проверка " + res.probe.IP + " завершена"
			probeReport = probeSummary(res.probe)
		}
	}
	updateNetworkReport()
	invalidate()
}

func networkAction(id string) bool {
	switch id {
	case "network_discover":
		beginNetworkOperation("discover")
	case "network_probe":
		beginNetworkOperation("probe")
	case "network_wifi_scan":
		beginNetworkOperation("wifi")
	case "network_device_prev":
		if networkDeviceOffset >= 4 {
			networkDeviceOffset -= 4
		}
		invalidate()
	case "network_device_next":
		if networkDeviceOffset+4 < len(networkDevices) {
			networkDeviceOffset += 4
		}
		invalidate()
	case "network_wifi_prev":
		if networkWiFiOffset >= 6 {
			networkWiFiOffset -= 6
		}
		invalidate()
	case "network_wifi_next":
		if networkWiFiOffset+6 < len(networkWiFi) {
			networkWiFiOffset += 6
		}
		invalidate()
	case "network_save_report":
		if networkReport == "" {
			return true
		}
		desktop := filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
		if fi, err := os.Stat(desktop); err != nil || !fi.IsDir() {
			desktop = os.TempDir()
		}
		out := filepath.Join(desktop, "LinkVideo_Network_"+time.Now().Format("20060102_150405")+".txt")
		if err := os.WriteFile(out, []byte(networkReport), 0600); err != nil {
			showMsg(err.Error(), "Не удалось сохранить", 0x10)
		} else {
			showMsg("Сохранено:\n"+out, "Отчёт", 0x40)
		}
	default:
		if strings.HasPrefix(id, "network_select_") {
			var index int
			if _, err := fmt.Sscanf(id, "network_select_%d", &index); err == nil && index >= 0 && index < len(networkDevices) {
				setText(networkIPControl, networkDevices[index].IP)
				beginNetworkOperation("probe")
			}
		} else {
			return false
		}
	}
	return true
}
