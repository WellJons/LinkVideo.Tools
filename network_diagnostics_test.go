package main

import (
	"context"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func TestParseONVIF(t *testing.T) {
	xml := `<Envelope xmlns:d="urn:discover"><Body><d:ProbeMatches><d:ProbeMatch><d:XAddrs>http://192.168.1.30/onvif/device_service</d:XAddrs><d:Scopes>onvif://www.onvif.org/name/Camera%20One</d:Scopes></d:ProbeMatch></d:ProbeMatches></Body></Envelope>`
	d, ok := parseONVIF([]byte(xml), "192.168.1.30")
	if !ok || d.Protocol != "ONVIF" || d.Name != "Camera One" {
		t.Fatalf("wrong ONVIF response: %#v, %v", d, ok)
	}
	if _, ok := parseONVIF([]byte(`<Envelope/>`), "192.168.1.31"); ok {
		t.Fatal("empty ONVIF response detected as a camera")
	}
}

func TestParseSADP(t *testing.T) {
	data := []byte(`<ProbeMatch><IPv4Address>10.3.2.1</IPv4Address><MAC>aa-bb-cc-dd-ee-ff</MAC><DeviceType>DS-2CD</DeviceType><Activated>false</Activated></ProbeMatch>`)
	d, ok := parseSADP(data, "192.168.1.50")
	if !ok || d.IP != "192.168.1.50" || d.AdvertisedIP != "10.3.2.1" || d.Activated != "false" || d.MAC != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("SADP parsing failed: %#v, %v", d, ok)
	}
}

func TestParseDHIP(t *testing.T) {
	probe := dahuaProbe()
	if len(probe) < 33 || string(probe[4:8]) != "DHIP" {
		t.Fatalf("invalid discovery frame: %x", probe)
	}
	body := []byte(`{"params":{"deviceInfo":{"IP":"10.1.1.66","SerialNo":"SN123","DeviceType":"Dahua"}}}`)
	resp := make([]byte, 32+len(body))
	copy(resp[0:32], probe[0:32])
	binary.LittleEndian.PutUint32(resp[16:20], uint32(len(body)))
	binary.LittleEndian.PutUint32(resp[24:28], uint32(len(body)))
	copy(resp[32:], body)
	d, ok := parseDHIP(resp, "192.168.0.5")
	if !ok || d.AdvertisedIP != "10.1.1.66" || d.Protocol != "DHIP" {
		t.Fatalf("DHIP parsing failed: %#v, %v", d, ok)
	}
	if _, ok := parseDHIP(resp[:len(resp)-2], "192.168.0.5"); ok {
		t.Fatal("accepted truncated DHIP response")
	}
}

func TestDeviceMerge(t *testing.T) {
	a := DiscoveredDevice{IP: "192.168.1.10", Protocol: "ONVIF", Name: "Camera"}
	a.merge(DiscoveredDevice{IP: a.IP, Protocol: "SADP", Model: "DS-2CD"})
	a.merge(DiscoveredDevice{IP: a.IP, Protocol: "SADP"})
	if a.Protocol != "ONVIF, SADP" || a.Name != "Camera" || a.Model != "DS-2CD" {
		t.Fatalf("merged incorrectly: %#v", a)
	}
}

func TestAddressValidation(t *testing.T) {
	for _, invalid := range []string{"", "rtsp://192.168.1.1/live?password=123", "239.255.255.250", "0.0.0.0", "255.255.255.255", "example.com"} {
		if _, err := validatedCameraIP(invalid); err == nil {
			t.Errorf("accepted %q", invalid)
		}
	}
	if ip, err := validatedCameraIP(" 192.168.1.20 "); err != nil || ip != "192.168.1.20" {
		t.Errorf("rejected valid address: %q %v", ip, err)
	}
}

func TestWiFiBands(t *testing.T) {
	for _, tc := range []struct{ freq, channel int; band string }{
		{2412, 1, "2.4 ГГц"}, {2462, 11, "2.4 ГГц"}, {2484, 14, "2.4 ГГц"}, {5180, 36, "5 ГГц"}, {5955, 1, "6 ГГц"},
	} {
		c, b := frequencyChannel(tc.freq)
		if c != tc.channel || b != tc.band {
			t.Errorf("%d MHz -> %d %s", tc.freq, c, b)
		}
	}
}

func TestReportQualifiers(t *testing.T) {
	text := probeSummary(CameraProbe{IP: "192.168.1.2", Checked: time.Unix(0, 0), RTSPStatus: "RTSP ответил: 401", ICMPSucceeded: 1, ICMPAttempts: 3})
	if !strings.Contains(text, "не подтверждают") || !strings.Contains(text, "401") {
		t.Fatalf("missing important diagnostic qualifier: %s", text)
	}
	d, err := DiscoverCameras(context.Background(), 500*time.Millisecond)
	_ = d
	// DiscoverCameras can fail on test runners with no physical network;
	// this verifies only that it terminates, not that cameras exist.
	_ = err
}
