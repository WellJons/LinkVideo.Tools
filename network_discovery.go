package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// DiscoveredDevice contains only facts from LAN discovery responses.
// IP is the responding host, while AdvertisedIP is untrusted device metadata.
type DiscoveredDevice struct {
	IP           string
	AdvertisedIP string
	Protocol     string
	Name         string
	Model        string
	MAC          string
	Activated    string
	ONVIFURL     string
}

func (d *DiscoveredDevice) merge(other DiscoveredDevice) {
	if !strings.Contains(d.Protocol, other.Protocol) {
		if d.Protocol != "" {
			d.Protocol += ", "
		}
		d.Protocol += other.Protocol
	}
	if d.Name == "" {
		d.Name = other.Name
	}
	if d.Model == "" {
		d.Model = other.Model
	}
	if d.MAC == "" {
		d.MAC = other.MAC
	}
	if d.Activated == "" {
		d.Activated = other.Activated
	}
	if d.ONVIFURL == "" {
		d.ONVIFURL = other.ONVIFURL
	}
	if d.AdvertisedIP == "" {
		d.AdvertisedIP = other.AdvertisedIP
	}
}

func localIPv4s() ([]net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var ips []net.IP
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok || ipnet.IP.To4() == nil {
				continue
			}
			ip := ipnet.IP.To4()
			if !seen[ip.String()] {
				ips = append(ips, ip)
				seen[ip.String()] = true
			}
		}
	}
	if len(ips) == 0 {
		return nil, errors.New("нет активного IPv4-сетевого адаптера")
	}
	return ips, nil
}

type discoveryProtocol struct {
	name  string
	group string
	port  int
	probe [][]byte
	parse func([]byte, string) (DiscoveredDevice, bool)
}

const onvifProbe = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
 xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing">
<s:Header><a:Action s:mustUnderstand="1">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
<a:MessageID>uuid:9dc97513-bbf8-4c8c-938f-180343719865</a:MessageID>
<a:ReplyTo><a:Address>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>
<a:To s:mustUnderstand="1">urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To></s:Header>
<s:Body><Probe xmlns="http://schemas.xmlsoap.org/ws/2005/04/discovery">
<Types xmlns:dn="http://www.onvif.org/ver10/network/wsdl">dn:NetworkVideoTransmitter</Types>
</Probe></s:Body></s:Envelope>`

type onvifMatch struct {
	XAddrs string `xml:"XAddrs"`
	Scopes string `xml:"Scopes"`
}

func parseONVIF(data []byte, source string) (DiscoveredDevice, bool) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := t.(xml.StartElement)
		if !ok || start.Name.Local != "ProbeMatch" {
			continue
		}
		var match onvifMatch
		if dec.DecodeElement(&match, &start) != nil {
			break
		}
		if strings.TrimSpace(match.XAddrs) == "" {
			continue
		}
		dev := DiscoveredDevice{IP: source, Protocol: "ONVIF", ONVIFURL: strings.Fields(match.XAddrs)[0]}
		for _, scope := range strings.Fields(match.Scopes) {
			if strings.Contains(scope, "/name/") && dev.Name == "" {
				dev.Name = strings.ReplaceAll(scope[strings.LastIndex(scope, "/")+1:], "%20", " ")
			}
		}
		return dev, true
	}
	return DiscoveredDevice{}, false
}

type sadpMatch struct {
	IPv4Address string `xml:"IPv4Address"`
	MAC         string `xml:"MAC"`
	Model       string `xml:"DeviceType"`
	Description string `xml:"DeviceDescription"`
	Activated   string `xml:"Activated"`
}

func parseSADP(data []byte, source string) (DiscoveredDevice, bool) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := t.(xml.StartElement)
		if !ok || start.Name.Local != "ProbeMatch" {
			continue
		}
		var m sadpMatch
		if dec.DecodeElement(&m, &start) != nil || (m.MAC == "" && m.IPv4Address == "") {
			continue
		}
		dev := DiscoveredDevice{IP: source, AdvertisedIP: m.IPv4Address,
			Protocol: "SADP", MAC: strings.ToUpper(strings.ReplaceAll(m.MAC, "-", ":")),
			Model: m.Model, Name: m.Description, Activated: m.Activated}
		return dev, true
	}
	return DiscoveredDevice{}, false
}

func dahuaProbe() []byte {
	body := []byte(`{"method":"DHDiscover.search","params":{"mac":"","uni":1}}`)
	buf := make([]byte, 32+len(body))
	binary.LittleEndian.PutUint32(buf[0:], 32)
	binary.LittleEndian.PutUint32(buf[4:], 0x50494844) // DHIP
	binary.LittleEndian.PutUint32(buf[16:], uint32(len(body)))
	binary.LittleEndian.PutUint32(buf[24:], uint32(len(body)))
	copy(buf[32:], body)
	return buf
}

func getString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		for k, v := range m {
			if strings.EqualFold(k, key) {
				if s, ok := v.(string); ok {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func parseDHIP(data []byte, source string) (DiscoveredDevice, bool) {
	if len(data) < 32 || binary.LittleEndian.Uint32(data[4:8]) != 0x50494844 {
		return DiscoveredDevice{}, false
	}
	size := int(binary.LittleEndian.Uint32(data[24:28]))
	if size <= 0 || size > len(data)-32 {
		return DiscoveredDevice{}, false
	}
	var root map[string]any
	if json.Unmarshal(data[32:32+size], &root) != nil {
		return DiscoveredDevice{}, false
	}
	params, ok := root["params"].(map[string]any)
	if !ok {
		return DiscoveredDevice{}, false
	}
	info, ok := params["deviceInfo"].(map[string]any)
	if !ok {
		info = params
	}
	ip := getString(info, "IP", "IPv4Address", "ipAddress")
	mac := getString(info, "MAC", "mac")
	model := getString(info, "DeviceType", "deviceType", "DeviceModel", "type")
	if ip == "" && mac == "" && model == "" && getString(info, "SerialNo") == "" {
		return DiscoveredDevice{}, false
	}
	return DiscoveredDevice{IP: source, AdvertisedIP: ip, Protocol: "DHIP", MAC: mac, Model: model}, true
}

// discoveryTargets uses only read-only multicast discovery probes.
func discoveryTargets() []discoveryProtocol {
	return []discoveryProtocol{
		{"ONVIF", "239.255.255.250", 3702, [][]byte{[]byte(onvifProbe)}, parseONVIF},
		{"SADP", "239.255.255.250", 37020,
			[][]byte{
				[]byte(`<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>9dc97513-bbf8-4c8c-938f-180343719865</Uuid><Types>inquiry</Types></Probe>`),
				[]byte(`<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>9dc97513-bbf8-4c8c-938f-180343719865</Uuid><Types>inquiry_v32</Types></Probe>`),
			}, parseSADP},
		{"DHIP", "239.255.255.251", 37810, [][]byte{dahuaProbe()}, parseDHIP},
	}
}

func scanUDP(ctx context.Context, local net.IP, p discoveryProtocol, timeout time.Duration) []DiscoveredDevice {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: local, Port: 0})
	if err != nil {
		return nil
	}
	defer conn.Close()
	target := &net.UDPAddr{IP: net.ParseIP(p.group), Port: p.port}
	for _, packet := range p.probe {
		_, _ = conn.WriteToUDP(packet, target)
		if p.name == "SADP" {
			_, _ = conn.WriteToUDP(packet, &net.UDPAddr{IP: net.IPv4bcast, Port: p.port})
		}
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetReadDeadline(deadline)
	buf := make([]byte, 65535)
	var devices []DiscoveredDevice
	for ctx.Err() == nil {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		if from == nil || from.IP.To4() == nil {
			continue
		}
		dev, ok := p.parse(buf[:n], from.IP.String())
		if ok {
			devices = append(devices, dev)
		}
	}
	return devices
}

// DiscoverCameras probes each active IPv4 adapter concurrently for a bounded interval.
// No logins, activations, configuration writes, port scans or public network calls.
func DiscoverCameras(ctx context.Context, duration time.Duration) ([]DiscoveredDevice, error) {
	if duration < time.Second {
		duration = time.Second
	}
	if duration > 10*time.Second {
		duration = 10 * time.Second
	}
	ips, err := localIPv4s()
	if err != nil {
		return nil, err
	}
	result := make(chan []DiscoveredDevice, len(ips)*3)
	var wg sync.WaitGroup
	for _, ip := range ips {
		for _, proto := range discoveryTargets() {
			wg.Add(1)
			go func(ip net.IP, proto discoveryProtocol) {
				defer wg.Done()
				result <- scanUDP(ctx, ip, proto, duration)
			}(ip, proto)
		}
	}
	go func() { wg.Wait(); close(result) }()
	seen := map[string]DiscoveredDevice{}
	for batch := range result {
		for _, dev := range batch {
			old, ok := seen[dev.IP]
			if !ok {
				seen[dev.IP] = dev
			} else {
				old.merge(dev)
				seen[dev.IP] = old
			}
		}
	}
	out := make([]DiscoveredDevice, 0, len(seen))
	for _, d := range seen {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := net.ParseIP(out[i].IP).To4(), net.ParseIP(out[j].IP).To4()
		return binary.BigEndian.Uint32(a) < binary.BigEndian.Uint32(b)
	})
	return out, ctx.Err()
}

func discoverySummary(devices []DiscoveredDevice) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[ПОИСК КАМЕР] Найдено: %d\n", len(devices))
	for _, d := range devices {
		fmt.Fprintf(&b, "IP: %s; протокол: %s; модель: %s; MAC: %s\n", d.IP, d.Protocol, d.Model, d.MAC)
		if d.AdvertisedIP != "" && d.AdvertisedIP != d.IP {
			fmt.Fprintf(&b, "  Указанный камерой IP: %s (может отличаться от адреса ответа)\n", d.AdvertisedIP)
		}
		if d.Activated != "" {
			fmt.Fprintf(&b, "  Активация (ответ SADP): %s\n", d.Activated)
		}
	}
	b.WriteString("Поиск видит только устройства, отвечающие на широковещательные/мультикаст-запросы в доступных сегментах сети.\n")
	return b.String()
}
