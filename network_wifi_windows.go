//go:build windows

package main

import (
	"fmt"
	"sort"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

var (
	wlanDLL              = syscall.NewLazyDLL("wlanapi.dll")
	wlanOpenHandle       = wlanDLL.NewProc("WlanOpenHandle")
	wlanCloseHandle      = wlanDLL.NewProc("WlanCloseHandle")
	wlanEnumInterfaces   = wlanDLL.NewProc("WlanEnumInterfaces")
	wlanGetNetworkBSS    = wlanDLL.NewProc("WlanGetNetworkBssList")
	wlanFreeMemory       = wlanDLL.NewProc("WlanFreeMemory")
)

type wlanInterfaceInfo struct {
	GUID        [16]byte
	Description [256]uint16
	State       uint32
}

type wlanSSID struct {
	Length uint32
	Bytes  [32]byte
}

// The layout follows Windows WLAN_BSS_ENTRY (wlanapi.h), including padding.
type wlanBSSEntry struct {
	SSID          wlanSSID
	PHYID         uint32
	BSSID         [6]byte
	Pad1          [2]byte
	BSSType       uint32
	PHYType       uint32
	RSSI          int32
	Quality       uint32
	InRegDomain   uint8
	Pad2          [3]byte
	BeaconPeriod  uint16
	Pad3          [2]byte
	Timestamp     uint64
	HostTimestamp uint64
	Capability    uint16
	Pad4          [2]byte
	FrequencyKHz  uint32
	RatesCount    uint32
	Rates         [126]uint16
	IEOffset      uint32
	IESize        uint32
}

func wlanStatusError(name string, status uintptr) error {
	return fmt.Errorf("%s вернул код Windows %d", name, status)
}

func scanWiFiNetworks() ([]WiFiBSS, error) {
	var version uint32
	var handle uintptr
	code, _, _ := wlanOpenHandle.Call(2, 0, uintptr(unsafe.Pointer(&version)), uintptr(unsafe.Pointer(&handle)))
	if code != 0 {
		return nil, wlanStatusError("WlanOpenHandle", code)
	}
	defer wlanCloseHandle.Call(handle, 0)

	var interfaces uintptr
	code, _, _ = wlanEnumInterfaces.Call(handle, 0, uintptr(unsafe.Pointer(&interfaces)))
	if code != 0 {
		return nil, wlanStatusError("WlanEnumInterfaces", code)
	}
	if interfaces == 0 {
		return nil, fmt.Errorf("WLAN API не вернул список адаптеров")
	}
	defer wlanFreeMemory.Call(interfaces)

	count := *(*uint32)(unsafe.Pointer(interfaces))
	if count > 128 {
		return nil, fmt.Errorf("неожиданное количество Wi-Fi адаптеров")
	}
	var all []WiFiBSS
	const headerSize = uintptr(8)
	for i := uint32(0); i < count; i++ {
		p := unsafe.Add(unsafe.Pointer(interfaces), headerSize+uintptr(i)*unsafe.Sizeof(wlanInterfaceInfo{}))
		info := (*wlanInterfaceInfo)(p)
		adapter := syscall.UTF16ToString(info.Description[:])
		var list uintptr
		// pDot11Ssid=nil, BSS type=any (3), security filter=false.
		code, _, _ = wlanGetNetworkBSS.Call(handle, uintptr(unsafe.Pointer(&info.GUID)), 0, 3, 0, 0, uintptr(unsafe.Pointer(&list)))
		if code != 0 || list == 0 {
			continue
		}
		total := *(*uint32)(unsafe.Add(unsafe.Pointer(list), 4))
		if total > 2048 {
			wlanFreeMemory.Call(list)
			continue
		}
		for j := uint32(0); j < total; j++ {
			entry := (*wlanBSSEntry)(unsafe.Add(unsafe.Pointer(list), 8+uintptr(j)*unsafe.Sizeof(wlanBSSEntry{})))
			n := int(entry.SSID.Length)
			if n < 0 || n > len(entry.SSID.Bytes) {
				continue
			}
			rawSSID := entry.SSID.Bytes[:n]
			ssid := string(rawSSID)
			if !utf8.Valid(rawSSID) {
				ssid = strings.ToValidUTF8(ssid, "?")
			}
			if ssid == "" {
				ssid = "(скрытая сеть)"
			}
			freqMHz := int(entry.FrequencyKHz / 1000)
			channel, band := frequencyChannel(freqMHz)
			bssid := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
				entry.BSSID[0], entry.BSSID[1], entry.BSSID[2],
				entry.BSSID[3], entry.BSSID[4], entry.BSSID[5])
			all = append(all, WiFiBSS{Adapter: adapter, SSID: ssid, BSSID: bssid,
				Channel: channel, Frequency: freqMHz, RSSI: int(entry.RSSI),
				Quality: entry.Quality, Band: band})
		}
		wlanFreeMemory.Call(list)
	}
	// Scanning is observational: the operating system supplies BSS scan cache.
	sort.Slice(all, func(i, j int) bool { return all[i].RSSI > all[j].RSSI })
	return all, nil
}
