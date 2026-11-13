package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type WatchSettings struct {
	CheckSeconds      int         `json:"check_seconds"`
	StartAfterSeconds int         `json:"start_after_seconds"`
	StaleMinutes      int         `json:"stale_minutes"`
	CooldownMinutes   int         `json:"cooldown_minutes"`
	ServiceName       string      `json:"service_name"`
	AutoRestart       bool        `json:"auto_restart"`
	RestartOnStale    bool        `json:"restart_on_stale"`
	RestartHistory    []time.Time `json:"restart_history,omitempty"`
	ArchiveMode       string      `json:"archive_mode,omitempty"`
	ArchivePath       string      `json:"archive_path,omitempty"`
}

func defaults() WatchSettings {
	return WatchSettings{CheckSeconds: 30, StartAfterSeconds: 120, StaleMinutes: 5, CooldownMinutes: 30, ServiceName: "", AutoRestart: false, RestartOnStale: false, ArchiveMode: "auto"}
}
func clamp(n, a, b int) int {
	if n < a {
		return a
	}
	if n > b {
		return b
	}
	return n
}
func (s *WatchSettings) sanitize() {
	s.CheckSeconds = clamp(s.CheckSeconds, 15, 3600)
	s.StartAfterSeconds = clamp(s.StartAfterSeconds, 30, 86400)
	s.StaleMinutes = clamp(s.StaleMinutes, 2, 1440)
	s.CooldownMinutes = clamp(s.CooldownMinutes, 5, 1440)
	if s.ArchiveMode != "manual" {
		s.ArchiveMode = "auto"
	}
	s.ArchivePath = strings.TrimSpace(s.ArchivePath)
	s.ServiceName = strings.TrimSpace(s.ServiceName)
	if len(s.ServiceName) > 128 {
		s.ServiceName = ""
	}
	recent := make([]time.Time, 0, len(s.RestartHistory))
	for _, x := range s.RestartHistory {
		if x.After(time.Now().Add(-24*time.Hour)) && x.Before(time.Now().Add(time.Minute)) {
			recent = append(recent, x)
		}
	}
	if len(recent) > 10 {
		recent = recent[len(recent)-10:]
	}
	s.RestartHistory = recent
}
func settingsPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "LinkVideo.ServerTools", "settings.json")
}
func loadSettings() WatchSettings {
	s := defaults()
	b, e := os.ReadFile(settingsPath())
	if e == nil {
		_ = json.Unmarshal(b, &s)
	}
	s.sanitize()
	return s
}
func saveSettings(s WatchSettings) error {
	s.sanitize()
	p := settingsPath()
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0600)
}

type WatchState struct {
	LastCheck   time.Time
	AbsentSince time.Time
	LastRestart time.Time
	Attempts    []time.Time
	StaleCount  int
	LastMessage string
	LastError   string
	Working     bool
}

func initialWatchState(c WatchSettings) WatchState {
	st := WatchState{Attempts: append([]time.Time{}, c.RestartHistory...)}
	for _, x := range st.Attempts {
		if x.After(st.LastRestart) {
			st.LastRestart = x
		}
	}
	return st
}
func (s *WatchState) recordRestart(t time.Time) {
	s.LastRestart = t
	s.Attempts = append(s.Attempts, t)
}
func (s *WatchState) allowed(t time.Time, conf WatchSettings) error {
	if !s.LastRestart.IsZero() && t.Sub(s.LastRestart) < time.Duration(conf.CooldownMinutes)*time.Minute {
		return fmt.Errorf("защитная пауза после перезапуска: %v", time.Duration(conf.CooldownMinutes)*time.Minute-t.Sub(s.LastRestart))
	}
	cut := t.Add(-24 * time.Hour)
	n := 0
	for _, r := range s.Attempts {
		if r.After(cut) {
			n++
		}
	}
	if n >= 3 {
		return errors.New("лимит 3 перезапуска за 24 часа")
	}
	return nil
}
func isOfflineLongEnough(s *WatchState, now time.Time, conf WatchSettings, running bool) bool {
	if running {
		s.AbsentSince = time.Time{}
		return false
	}
	if s.AbsentSince.IsZero() {
		s.AbsentSince = now
		return false
	}
	return now.Sub(s.AbsentSince) >= time.Duration(conf.StartAfterSeconds)*time.Second
}
func stoppedRecording(cams []CameraScan, mins int) bool {
	if len(cams) == 0 {
		return false
	}
	now := time.Now()
	recent := 0
	stale := 0
	for _, c := range cams {
		if c.LastWrite.IsZero() {
			continue
		}
		if now.Sub(c.LastWrite) < 24*time.Hour {
			recent++
		}
		if now.Sub(c.LastWrite) >= time.Duration(mins)*time.Minute {
			stale++
		}
	}
	// Guard: don't assume a disabled camera should be recording forever.
	return recent > 0 && stale == len(cams)
}
func discoverNimbleService() (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("поиск службы поддерживается только в Windows")
	}
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-CimInstance Win32_Service | Where-Object { $_.PathName -match '(?i)nimble' } | Select-Object -First 1 -ExpandProperty Name`)
	out, e := cmd.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("поиск службы: %v", e)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", errors.New("служба Nimble не найдена")
	}
	if strings.ContainsAny(name, "\r\n") {
		return "", errors.New("служба определена неоднозначно")
	}
	return name, nil
}
func runSC(timeout time.Duration, args ...string) (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("управление службой доступно только в Windows")
	}
	// The service name is one argument, not parsed through cmd.exe.
	cmd := exec.Command("sc.exe", args...)
	done := make(chan error, 1)
	var output []byte
	go func() { var err error; output, err = cmd.CombinedOutput(); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			return string(output), fmt.Errorf("sc %v: %v", args[0], err)
		}
		return string(output), nil
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return "", errors.New("sc.exe: превышено время ожидания")
	}
}
func serviceStatus(name string) (string, error) { return runSC(12*time.Second, "query", name) }
func restartService(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("сначала укажите имя службы Nimble")
	}
	if strings.ContainsAny(name, "\r\n\x00") {
		return errors.New("некорректное имя службы")
	}
	status, err := serviceStatus(name)
	if err != nil {
		return fmt.Errorf("служба %q не найдена или недоступна: %w", name, err)
	}
	description, err := runSC(12*time.Second, "qc", name)
	if err != nil {
		return fmt.Errorf("не удалось проверить путь службы: %w", err)
	}
	if !strings.Contains(strings.ToLower(name+" "+description), "nimble") {
		return errors.New("проверка безопасности: эта служба не похожа на Nimble")
	}
	// The service may already be stopped: start it without first stopping.
	if strings.Contains(status, "STATE") && (strings.Contains(status, "STOPPED") || strings.Contains(status, "STATE              : 1 ")) {
		_, err = runSC(20*time.Second, "start", name)
		return err
	}
	_, err = runSC(20*time.Second, "stop", name)
	if err != nil {
		return fmt.Errorf("остановка службы: %w", err)
	}
	for i := 0; i < 20; i++ {
		time.Sleep(time.Second)
		q, e := serviceStatus(name)
		if e == nil && (strings.Contains(q, "STOPPED") || strings.Contains(q, "STATE              : 1 ")) {
			_, e = runSC(20*time.Second, "start", name)
			return e
		}
	}
	return errors.New("служба не остановилась за 20 секунд; запуск не выполнен")
}
