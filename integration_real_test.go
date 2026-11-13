//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIntegrationOnRealSample(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, "linkvideo_301547", "1794310053262")
	if e := os.MkdirAll(d, 0755); e != nil {
		t.Fatal(e)
	}
	for _, f := range []string{"nimble-dvr-segments.db", "1794310053262.dat", "1794311532678.dat"} {
		if e := os.Symlink(filepath.Join("/mnt/data", f), filepath.Join(d, f)); e != nil {
			t.Fatal(e)
		}
	}
	cat, e := scanArchiveCatalog(root)
	if e != nil {
		t.Fatal(e)
	}
	if len(cat.Cameras) != 1 {
		t.Fatalf("cameras=%d", len(cat.Cameras))
	}
	for _, r := range cat.Cameras[0].Ranges {
		t.Logf("range %s - %s blocks=%d", time.UnixMilli(r.Start).UTC(), time.UnixMilli(r.End).UTC(), r.VideoCount)
	}
	if len(cat.Cameras[0].Ranges) < 3 {
		t.Fatalf("too few ranges: %d", len(cat.Cameras[0].Ranges))
	}

	for i, r := range cat.Cameras[0].Ranges {
		out := filepath.Join(t.TempDir(), fmt.Sprintf("sample_%d.mp4", i))
		clipEnd := r.Start + 50000
		if clipEnd > r.End {
			clipEnd = r.End
		}
		err := ExportRecordingRange(context.Background(), cat, 0, i, r.Start, clipEnd, "/usr/bin/ffmpeg", out)
		if err != nil {
			t.Fatalf("group %d: %v", i, err)
		}
		if st, e := os.Stat(out); e != nil || st.Size() < 100000 {
			t.Fatalf("group %d file missing: %v", i, e)
		}
		b, e := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", out).CombinedOutput()
		if e != nil {
			t.Fatal(e, string(b))
		}
		if !strings.Contains(string(b), "video") {
			t.Fatalf("group %d video missing: %s", i, b)
		}
		t.Logf("group %d result streams: %s", i, strings.TrimSpace(string(b)))
	}

}
