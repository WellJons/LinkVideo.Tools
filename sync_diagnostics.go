package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SyncLogProgress describes only facts observable in LinkVideo.Server's logs.
// It deliberately never exposes a raw log line, URL, cookie, password or token.
// An HTTP 200 from the local API is not cloud upload confirmation.
type SyncLogProgress struct {
	LogFiles         int
	TimelineRequests int
	CameraStarts     int
	TokenRequests    int
	TokenRetries     int
	TokenFailures    int
	TokenAcquired    int
	DATSendStarts    int
	INDEXSendStarts  int
	UploadErrors     int
	LastSeen         time.Time
	Phase            string
	Detail           string
	TokenErrorKind   string
	Warning          string
	HasEvidence      bool
}

type syncEvent struct {
	at   time.Time
	code string
	rank int
}

func parseLinkVideoLogLine(line string) (time.Time, string, bool) {
	// e.g. (1234) 2026/11/13 10:49:06 ...
	left := strings.Index(line, ") ")
	if left < 0 {
		return time.Time{}, "", false
	}
	pos := left + 2
	if len(line) < pos+20 {
		return time.Time{}, "", false
	}
	t, err := time.ParseInLocation("2006/01/02 15:04:05", line[pos:pos+19], time.Local)
	if err != nil {
		return time.Time{}, "", false
	}
	return t, line[pos+20:], true
}

// classifySyncLog accepts only allowlisted event classes; do not print the input.
func classifySyncLog(message string) (code string, kind string) {
	s := strings.ToLower(message)
	switch {
	case strings.Contains(s, "failed to get upload token"):
		return "token-failed", classifyTokenFailure(s)
	case strings.Contains(s, "upload token") && strings.Contains(s, "acquired"):
		return "token-ok", ""
	case strings.Contains(s, "attempt ") && strings.Contains(s, "failed") &&
		(strings.Contains(s, "response") || strings.Contains(s, "token") || strings.Contains(s, "json")):
		return "token-retry", classifyTokenFailure(s)
	case strings.Contains(s, "trying to get upload token"):
		return "token-request", ""
	case strings.Contains(s, "sending dat file") || strings.Contains(s, "sending dat"):
		return "dat", ""
	case strings.Contains(s, "sending index file") || strings.Contains(s, "sending dbpart") || strings.Contains(s, "sending index"):
		return "index", ""
	case strings.Contains(s, "failed to upload") || strings.Contains(s, "upload failed") || strings.Contains(s, "upload error"):
		return "upload-error", ""
	case strings.Contains(s, "will request timeline gaps"):
		return "timeline", ""
	case strings.Contains(s, "starting processing for camera"):
		return "started", ""
	default:
		return "", ""
	}
}

func classifyTokenFailure(s string) string {
	switch {
	case strings.Contains(s, "unexpected end of json input"):
		return "Ответ API пустой или JSON обрезан"
	case strings.Contains(s, "json") || strings.Contains(s, "parse response"):
		return "Не удаётся разобрать JSON от API"
	case strings.Contains(s, "timeout") || strings.Contains(s, "deadline"):
		return "Истекло ожидание ответа API"
	case strings.Contains(s, "connection") || strings.Contains(s, "tls"):
		return "Сетевая ошибка при запросе токена"
	default:
		return "Ошибка получения токена (подробности в журнале сервера)"
	}
}

// scanSyncLogReader scans a bounded, possibly truncated, buffer of historical lines.
// A partial first line is harmless. All event details are sanitized by design.
func scanSyncLogReader(r io.Reader, since time.Time, p *SyncLogProgress, events *[]syncEvent) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 8*1024), 1024*1024)
	for scanner.Scan() {
		t, message, ok := parseLinkVideoLogLine(scanner.Text())
		if !ok || t.Before(since) {
			continue
		}
		code, kind := classifySyncLog(message)
		if code == "" {
			continue
		}
		p.HasEvidence = true
		if t.After(p.LastSeen) {
			p.LastSeen = t
		}
		switch code {
		case "timeline":
			p.TimelineRequests++
		case "started":
			p.CameraStarts++
		case "token-request":
			p.TokenRequests++
		case "token-retry":
			p.TokenRetries++
		case "token-failed":
			p.TokenFailures++
		case "token-ok":
			p.TokenAcquired++
		case "dat":
			p.DATSendStarts++
		case "index":
			p.INDEXSendStarts++
		case "upload-error":
			p.UploadErrors++
		}
		if kind != "" {
			p.TokenErrorKind = kind
		}
		*events = append(*events, syncEvent{t, code, eventPriority(code)})
	}
	return scanner.Err()
}

func eventPriority(c string) int {
	switch c {
	case "token-failed", "upload-error":
		return 9
	case "index":
		return 8
	case "dat":
		return 7
	case "token-ok":
		return 6
	case "token-retry":
		return 5
	case "token-request":
		return 4
	case "timeline":
		return 3
	case "started":
		return 2
	default:
		return 0
	}
}

func resolveSyncPhase(p *SyncLogProgress, events []syncEvent) {
	if len(events) == 0 {
		p.Phase = "Нет событий выгрузки за выбранное время"
		p.Detail = "Проверь журнал LinkVideo.Server и время на компьютере"
		return
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].at.Before(events[j].at) || (events[i].at.Equal(events[j].at) && events[i].rank < events[j].rank)
	})
	latest := events[len(events)-1].code
	switch latest {
	case "started":
		p.Phase = "Запущена синхронизация"
		p.Detail = "Запись в облаке пока не подтверждена"
	case "timeline":
		p.Phase = "Запрос недостающих интервалов"
		p.Detail = "Облако ещё не подтвердило получение файлов"
	case "token-request":
		p.Phase = "Повторный запрос upload-token"
		if p.TokenErrorKind != "" {
			p.Detail = "Ранее: " + p.TokenErrorKind
		} else {
			p.Detail = "Ожидание ответа сервера авторизации выгрузки"
		}
	case "token-retry":
		p.Phase = "Ошибка токена — повторная попытка"
		p.Detail = p.TokenErrorKind
	case "token-failed":
		p.Phase = "Не удалось получить upload-token"
		p.Detail = p.TokenErrorKind
	case "token-ok":
		p.Phase = "Upload-token получен"
		p.Detail = "Ожидается подтверждение передачи файлов"
	case "dat":
		p.Phase = "Начата отправка видео (.dat)"
		p.Detail = "Факт приёма облаком ещё не подтверждён"
	case "index":
		p.Phase = "Начата отправка индекса"
		p.Detail = "HTTP-статус облачного ответа оригинальный журнал не подтверждает"
	case "upload-error":
		p.Phase = "Ошибка отправки файлов"
		p.Detail = "Проверь ошибки в журнале оригинального LinkVideo.Server"
	}
}

// ScanSyncLog reads only recent tails, from at most three current log files.
// It never modifies LinkVideo.Server's logs or client archive.
func ScanSyncLog(folder string, since time.Time) SyncLogProgress {
	var p SyncLogProgress
	if since.IsZero() {
		p.Warning = "Время запуска синхронизации неизвестно"
		return p
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		p.Warning = "Журнал LinkVideo.Server не найден или недоступен"
		return p
	}
	type candidate struct {
		name string
		mod  time.Time
	}
	var files []candidate
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".log") {
			continue
		}
		info, e := entry.Info()
		if e == nil && info.ModTime().After(since.Add(-2*time.Minute)) {
			files = append(files, candidate{filepath.Join(folder, entry.Name()), info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	if len(files) > 3 {
		files = files[:3]
	}
	var events []syncEvent
	for _, f := range files {
		fh, e := os.Open(f.name)
		if e != nil {
			continue
		}
		info, e := fh.Stat()
		if e != nil {
			fh.Close()
			continue
		}
		const maxTail int64 = 2 * 1024 * 1024
		off := info.Size() - maxTail
		if off < 0 {
			off = 0
		}
		if _, e = fh.Seek(off, io.SeekStart); e == nil {
			// Never return raw lines; only safe event classifications.
			limited := io.LimitReader(fh, maxTail)
			if off > 0 { // discard the first potentially partial line
				rd := bufio.NewReader(limited)
				_, _ = rd.ReadBytes('\n')
				limited = rd
			}
			if e = scanSyncLogReader(limited, since, &p, &events); e == nil {
				p.LogFiles++
			}
		}
		fh.Close()
	}
	if p.LogFiles == 0 {
		p.Warning = "Нет доступных свежих файлов журналов"
	}
	resolveSyncPhase(&p, events)
	return p
}

// Formatting is deliberately data-only: no URLs, credentials or raw lines.
func (p SyncLogProgress) SafeCounters() string {
	return fmt.Sprintf("Запросов пропусков: %d   Токен: %d запросов / %d повторов / %d ошибок   DAT: %d   Индекс: %d",
		p.TimelineRequests, p.TokenRequests, p.TokenRetries, p.TokenFailures, p.DATSendStarts, p.INDEXSendStarts)
}

// helper for tests; production paths read bounded tails from disk.
func scanSyncLogText(text string, since time.Time) SyncLogProgress {
	var p SyncLogProgress
	var events []syncEvent
	_ = scanSyncLogReader(bytes.NewBufferString(text), since, &p, &events)
	resolveSyncPhase(&p, events)
	return p
}

func (p SyncLogProgress) StatusCode() string {
	if p.TokenFailures > 0 {
		return "token-error-" + strconv.Itoa(p.TokenFailures)
	}
	if p.INDEXSendStarts > 0 {
		return "index-started"
	}
	if p.DATSendStarts > 0 {
		return "dat-started"
	}
	if p.TokenRequests > 0 {
		return "token-requested"
	}
	if p.TimelineRequests > 0 {
		return "gaps-requested"
	}
	return "no-evidence"
}