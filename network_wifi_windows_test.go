//go:build windows

package main

import (
 "testing"
 "unsafe"
)

func TestNativeWLANStructLayout(t *testing.T) {
 if n := unsafe.Sizeof(wlanInterfaceInfo{}); n != 532 { t.Fatalf("WLAN_INTERFACE_INFO size: %d", n) }
 if n := unsafe.Sizeof(wlanBSSEntry{}); n != 360 { t.Fatalf("WLAN_BSS_ENTRY size: %d", n) }
}
