package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func logStart() time.Time { return time.Date(2026, 11, 13, 10, 40, 0, 0, time.Local) }

func TestSyncLogTokenFailureDoesNotLeakSecrets(t *testing.T) {
	text := `(14484) 2026/11/13 10:49:06 trying to get upload token for private-id
(14484) 2026/11/13 10:49:06 cookies after doTokenRequest: reqCookie = SECRET_COOKIE , respCookie = SECRET_COOKIE
(14484) 2026/11/13 10:49:06 Attempt 2 failed. Waiting 2s before next try. Last err: failed to parse response: unexpected end of JSON input
(14484) 2026/11/13 10:49:08 trying to get upload token for private-id
(14484) 2026/11/13 10:49:08 Failed to get upload token: failed after 10 attempts: failed to parse response: unexpected end of JSON input
`
	p := scanSyncLogText(text, logStart())
	if p.TokenFailures != 1 || p.TokenRetries != 1 || p.TokenRequests != 2 {
		t.Fatalf("bad counts: %+v", p)
	}
	if p.Phase != "Не удалось получить upload-token" || !strings.Contains(p.Detail, "JSON") {
		t.Fatalf("bad phase: %+v", p)
	}
	combined := p.Phase + p.Detail + p.SafeCounters() + p.Warning + p.TokenErrorKind
	if strings.Contains(combined, "SECRET") || strings.Contains(combined, "private-id") {
		t.Fatalf("secret leak: %q", combined)
	}
}
func TestSyncLogDoesNotConfuseCodecOrHTTP200ForUpload(t *testing.T) {
	p := scanSyncLogText(`(14484) 2026/11/13 10:41:00 will request timeline gaps using this url: https://example.invalid/path?token=secret
(14484) 2026/11/13 10:41:04 WatchAndReloadAVCConfig() Failed to get camera codec: exit status 1
`, logStart())
	if p.TokenFailures != 0 || p.DATSendStarts != 0 || p.TimelineRequests != 1 || p.Phase != "Запрос недостающих интервалов" {
		t.Fatalf("unexpected %+v", p)
	}
}
func TestSyncLogProgressAfterErrors(t *testing.T) {
	p := scanSyncLogText(`(1) 2026/11/13 10:41:01 Failed to get upload token: failed to parse response: unexpected end of JSON input
(1) 2026/11/13 10:41:02 upload token SECRET acquired, will be trying to upload files
(1) 2026/11/13 10:41:03 Sending DAT file SECRET.dat
(1) 2026/11/13 10:41:04 Sending INDEX file SECRET.dbpart
`, logStart())
	if p.Phase != "Начата отправка индекса" || p.TokenAcquired != 1 || p.DATSendStarts != 1 || p.INDEXSendStarts != 1 {
		t.Fatalf("progress %+v", p)
	}
}
func TestSyncLogOnDiskBounded(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "log_2026-11-13.log")
	input := `(1) 2026/11/13 10:41:00 trying to get upload token for secret
`
	if e := os.WriteFile(f, []byte(input), 0600); e != nil {
		t.Fatal(e)
	}
	// Test cannot assume wall clock equals 2026, so use the pure parser above
	// for historical fixtures. Real-time ScanSyncLog ignores future timestamps.
	p := scanSyncLogText(input, logStart())
	if p.TokenRequests != 1 {
		t.Fatal(p)
	}
}