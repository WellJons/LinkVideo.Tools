package main

import (
	"fmt"
	"sort"
	"strings"
)

type WiFiBSS struct {
	Adapter    string
	SSID       string
	BSSID      string
	Channel    int
	Frequency  int
	RSSI       int
	Quality    uint32
	Band       string
}

func frequencyChannel(mhz int) (int, string) {
	switch {
	case mhz == 2484:
		return 14, "2.4 ГГц"
	case mhz >= 2412 && mhz <= 2472:
		return (mhz - 2407) / 5, "2.4 ГГц"
	case mhz >= 5000 && mhz < 5925:
		return (mhz - 5000) / 5, "5 ГГц"
	case mhz >= 5955 && mhz <= 7115:
		return (mhz - 5950) / 5, "6 ГГц"
	default:
		return 0, "неизвестно"
	}
}

func wifiSummary(networks []WiFiBSS) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[WI-FI] Обнаружено точек доступа (BSSID): %d\n", len(networks))
	copyNetworks := append([]WiFiBSS(nil), networks...)
	sort.Slice(copyNetworks, func(i, j int) bool { return copyNetworks[i].RSSI > copyNetworks[j].RSSI })
	count24 := 0
	for _, n := range copyNetworks {
		if n.Band == "2.4 ГГц" {
			count24++
		}
		fmt.Fprintf(&b, "SSID: %s; BSSID: %s; %s канал %d; RSSI %d dBm; качество %d%%; адаптер: %s\n",
			n.SSID, n.BSSID, n.Band, n.Channel, n.RSSI, n.Quality, n.Adapter)
	}
	fmt.Fprintf(&b, "Обнаружено BSSID в 2.4 ГГц: %d\n", count24)
	b.WriteString("RSSI и качество относятся к точкам доступа, видимым компьютеру, а не к уровню сигнала каждой камеры.\n")
	b.WriteString("Разовая проверка не доказывает перегрузку эфира, реальный роуминг камеры или причину потерь пакетов.\n")
	return b.String()
}
