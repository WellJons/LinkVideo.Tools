package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveScan(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, "linkvideo_301547", "1794310053262")
	if err := os.MkdirAll(d, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "nimble-dvr-segments.db"), []byte("index"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "1794310053262.dat"), []byte("bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	x := newestCameraData(root, 10)
	if x.Error != "" || len(x.Cameras) != 1 || x.Cameras[0].DatFiles != 1 || !x.Cameras[0].IndexFound || x.Cameras[0].LastWrite.IsZero() {
		t.Fatalf("bad scan: %+v", x)
	}
}
func TestAge(t *testing.T) {
	if got := ageString(time.Time{}); got != "нет данных" {
		t.Fatal(got)
	}
}

func TestLogSnapshotDoesNotIncludeSecrets(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	date := now.Format("2006/01/02 15:04:05")
	sample := "(1628) " + date + " cron AddFunc error: expected exactly 5 fields\n" +
		"(1628) " + date + " WatchAndReloadAVCConfig() Failed to get video codec\n" +
		"(1628) " + date + " Authorization: Bearer SECRET_SHOULD_NOT_SHOW\n"
	if err := os.WriteFile(filepath.Join(root, "log.log"), []byte(sample), 0600); err != nil {
		t.Fatal(err)
	}
	s := logSnapshot(root, now.Add(-time.Hour))
	if s.Errors != 2 || s.Cron != 1 || s.Codec != 1 {
		t.Fatalf("unexpected log scan: %+v", s)
	}
}
