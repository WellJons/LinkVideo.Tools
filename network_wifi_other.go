//go:build !windows

package main

import "errors"

// The Windows client uses Native WLAN API; no pretend Wi-Fi signal data on Linux.
func scanWiFiNetworks() ([]WiFiBSS, error) {
	return nil, errors.New("сканирование Wi-Fi доступно только в сборке Windows")
}
