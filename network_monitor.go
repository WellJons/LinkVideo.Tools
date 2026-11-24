package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type NetworkSample struct {
	At           time.Time
	ICMPResponds bool
}

type NetworkMonitorHistory struct {
	IP       string
	Started  time.Time
	Samples  []NetworkSample
}

// The operator explicitly starts/stops this check. It never changes camera
// settings and uses one ICMP probe at each interval, not a packet flood.
func MonitorCamera(ctx context.Context, rawIP string, interval time.Duration, sample func(NetworkSample)) error {
	ip, err := validatedCameraIP(rawIP)
	if err != nil {
		return err
	}
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		obs := NetworkSample{At: time.Now(), ICMPResponds: pingOnce(ctx, ip)}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sample(obs)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func monitorSummary(h NetworkMonitorHistory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[МОНИТОРИНГ] IP %s, старт %s\n",
		h.IP, h.Started.Format("02.01.2006 15:04:05"))
	good, longestFailed, streak := 0, 0, 0
	var starts []string
	lastStart := time.Time{}
	for _, s := range h.Samples {
		if s.ICMPResponds {
			good++
			if streak > longestFailed {
				longestFailed = streak
			}
			streak = 0
			lastStart = time.Time{}
		} else {
			if streak == 0 {
				lastStart = s.At
				if len(starts) < 30 {
					starts = append(starts, s.At.Format("15:04:05"))
				}
			}
			streak++
		}
	}
	if streak > longestFailed {
		longestFailed = streak
	}
	fmt.Fprintf(&b, "Попыток ICMP: %d; ответы: %d; без ответа: %d\n",
		len(h.Samples), good, len(h.Samples)-good)
	fmt.Fprintf(&b, "Максимальная серия проверок без ответа: %d\n", longestFailed)
	if len(h.Samples) >= 2 {
		fmt.Fprintf(&b, "Наблюдение: %s — %s\n",
			h.Samples[0].At.Format("15:04:05"),
			h.Samples[len(h.Samples)-1].At.Format("15:04:05"))
	}
	if len(starts) > 0 {
		fmt.Fprintf(&b, "Начало серий без ответа (первые 30): %s\n", strings.Join(starts, ", "))
	}
	b.WriteString("ICMP может блокироваться камерой или маршрутизатором; отсутствие ответов не доказывает отключение Wi-Fi или сбой сервера.\n")
	return b.String()
}
