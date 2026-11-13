package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexAndRanges(t *testing.T) {
	d := filepath.Join(t.TempDir(), "linkvideo_100", "123")
	if e := os.MkdirAll(d, 0755); e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(d, "100.dat")
	if e := os.WriteFile(source, make([]byte, 60), 0600); e != nil {
		t.Fatal(e)
	}
	jsonData := `{"file":"100.dat","time":1000000,"offset":0,"size":10,"period":1,"type":".m4mv"},
{"file":"100.dat","time":1000010,"offset":10,"size":10,"period":1,"type":".m4v","duration":1800000},
{"file":"100.dat","time":1020010,"offset":20,"size":10,"period":1,"type":".m4v","duration":1800000},
{"file":"100.dat","time":1100000,"offset":30,"size":10,"period":1,"type":".m4v","duration":1800000},
{"file":"missing.dat","time":1120000,"offset":0,"size":10,"period":1,"type":".m4v","duration":1800000},
{"file":"100.dat","time":1200000,"offset":45,"size":20,"period":1,"type":".m4v","duration":1800000}`
	if e := os.WriteFile(filepath.Join(d, "nimble-dvr-segments.db"), []byte(jsonData), 0600); e != nil {
		t.Fatal(e)
	}
	catalog, e := scanArchiveCatalog(filepath.Dir(filepath.Dir(d)))
	if e != nil {
		t.Fatal(e)
	}
	c := catalog.Cameras[0]
	if c.Count != 3 || len(c.Ranges) != 2 {
		t.Fatalf("got %d valid video, %d intervals; want 3 video, 2 intervals", c.Count, len(c.Ranges))
	}
	if c.Ranges[0].VideoCount != 2 || c.Ranges[1].VideoCount != 1 {
		t.Fatalf("split wrong: %+v", c.Ranges)
	}
}
func TestRestartCooldown(t *testing.T) {
	now := time.Now()
	s := WatchState{LastRestart: now.Add(-5 * time.Minute)}
	if s.allowed(now, WatchSettings{CooldownMinutes: 30}) == nil {
		t.Fatal("cooldown was not enforced")
	}
	s.LastRestart = now.Add(-31 * time.Minute)
	if e := s.allowed(now, WatchSettings{CooldownMinutes: 30}); e != nil {
		t.Fatal(e)
	}
}
