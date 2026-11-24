package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PortObservation struct {
	Port      int
	Open      bool
	ConnectMs int64
}

type CameraProbe struct {
	IP              string
	Checked         time.Time
	ICMPSucceeded   int
	ICMPAttempts    int
	Ports           []PortObservation
	RTSPStatus      string
	RTSPError       string
	RTSPPort        int
}

// Only an IP address is accepted: URLs can contain passwords and must not
// end up in reports, subprocess arguments or device discovery callbacks.
func validatedCameraIP(value string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(value))
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.Equal(net.IPv4bcast) {
		return "", fmt.Errorf("введите корректный IPv4-адрес камеры, без RTSP-ссылки и пароля")
	}
	return ip.To4().String(), nil
}

func tcpAttempt(ctx context.Context, ip string, port int, timeout time.Duration) PortObservation {
	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		return PortObservation{Port: port}
	}
	_ = conn.Close()
	return PortObservation{Port: port, Open: true, ConnectMs: time.Since(start).Milliseconds()}
}

// Sending OPTIONS does not read a video stream and cannot verify H.264/H.265.
func rtspOptions(ctx context.Context, ip string, port int) (string, error) {
	d := net.Dialer{Timeout: 1500 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		return "", fmt.Errorf("соединение RTSP недоступно")
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return "", err
	}
	request := fmt.Sprintf("OPTIONS rtsp://%s:%d/ RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: LinkVideo.Tools\r\n\r\n", ip, port)
	if _, err := conn.Write([]byte(request)); err != nil {
		return "", fmt.Errorf("не удалось отправить OPTIONS: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("нет ответа RTSP: %w", err)
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "RTSP/1.") || len(line) > 180 {
		return "", fmt.Errorf("неожиданный ответ: это может быть не RTSP-сервер")
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", fmt.Errorf("неполный ответ RTSP")
	}
	switch parts[1] {
	case "200":
		return "RTSP ответил: 200 OK (видеопоток не проверялся)", nil
	case "401":
		return "RTSP ответил: 401 — требуется авторизация", nil
	case "404":
		return "RTSP ответил: 404 — путь / не найден; сервер доступен", nil
	case "405":
		return "RTSP ответил: 405 — OPTIONS не разрешён", nil
	default:
		return "RTSP ответил: " + parts[1], nil
	}
}

// Ping uses OS ICMP and only counts successes. Locale-dependent output is
// intentionally not parsed, and process startup time is not reported as RTT.
func pingOnce(ctx context.Context, ip string) bool {
	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"-n", "1", "-w", "1000", ip}
	} else {
		args = []string{"-c", "1", "-W", "1", ip}
	}
	command := exec.CommandContext(ctx, "ping", args...)
	return command.Run() == nil
}

func ProbeCamera(ctx context.Context, rawIP string) (CameraProbe, error) {
	ip, err := validatedCameraIP(rawIP)
	if err != nil {
		return CameraProbe{}, err
	}
	result := CameraProbe{IP: ip, Checked: time.Now()}
	ports := []int{80, 443, 554, 8554, 8000, 5000, 37777, 34567}
	type portResult struct{ value PortObservation }
	ch := make(chan portResult, len(ports))
	var wg sync.WaitGroup
	for _, port := range ports {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			ch <- portResult{tcpAttempt(ctx, ip, port, 1100*time.Millisecond)}
		}(port)
	}
	wg.Wait()
	close(ch)
	for item := range ch {
		result.Ports = append(result.Ports, item.value)
	}
	sort.Slice(result.Ports, func(i, j int) bool { return result.Ports[i].Port < result.Ports[j].Port })

	for _, p := range []int{554, 8554} {
		for _, item := range result.Ports {
			if item.Port == p && item.Open {
				result.RTSPPort = p
				result.RTSPStatus, err = rtspOptions(ctx, ip, p)
				if err != nil {
					result.RTSPError = err.Error()
				}
				goto ping
			}
		}
	}
ping:
	for i := 0; i < 3 && ctx.Err() == nil; i++ {
		result.ICMPAttempts++
		if pingOnce(ctx, ip) {
			result.ICMPSucceeded++
		}
	}
	return result, ctx.Err()
}

func probeSummary(p CameraProbe) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[ДИАГНОСТИКА IP] %s, %s\n", p.IP, p.Checked.Format("02.01.2006 15:04:05"))
	fmt.Fprintf(&b, "ICMP: %d из %d ответов\n", p.ICMPSucceeded, p.ICMPAttempts)
	b.WriteString("TCP-порты:\n")
	for _, port := range p.Ports {
		if port.Open {
			fmt.Fprintf(&b, "  %d: доступен, установка TCP-соединения ~%d мс\n", port.Port, port.ConnectMs)
		} else {
			fmt.Fprintf(&b, "  %d: нет TCP-соединения (закрыт/фильтрация/таймаут)\n", port.Port)
		}
	}
	if p.RTSPStatus != "" {
		fmt.Fprintf(&b, "%s (порт %d)\n", p.RTSPStatus, p.RTSPPort)
	} else if p.RTSPError != "" {
		fmt.Fprintf(&b, "RTSP OPTIONS: %s\n", p.RTSPError)
	} else {
		b.WriteString("RTSP: порт 554/8554 не ответил; пользовательский порт не проверен.\n")
	}
	b.WriteString("Важно: открытый порт и ответ OPTIONS не подтверждают наличие кадров, звука, совместимость кодека или работу архива.\n")
	if p.ICMPAttempts > 0 && p.ICMPSucceeded < p.ICMPAttempts {
		b.WriteString("ICMP ответил не на все попытки: возможны фильтрация ICMP или проблемы связи. По этой проверке нельзя установить причину.\n")
	}
	return b.String()
}
