package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveProxyPreferredOverBillingCloud(t *testing.T) {
	temp := t.TempDir()
	dir := filepath.Join(temp, "LinkVideo.Server")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	config := `{"CloudUrl":"https://api.b2o.goodline.info/ords/mobile/vc2", "DatServerProxy":"https://lvs-archive.linkvideo.ru", "Operator":"linkvideo"}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ProgramFiles(x86)", temp)
	t.Setenv("ProgramFiles", "")
	got := localPublicConfig()
	if got.DatServerProxy != "https://lvs-archive.linkvideo.ru" || got.Operator != "linkvideo" {
		t.Fatalf("archive config not used: %+v", got)
	}
}

func TestCloud404DoesNotClaimArchiveMissing(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }))
	defer ts.Close()
	result, err := probeTimelineGaps(context.Background(), ts.Client(), ts.URL, 301547, 100, 200, "linkvideo")
	if err != nil || result.Status != 404 || !strings.Contains(result.Message, "маршрут не найден") {
		t.Fatalf("%+v %v", result, err)
	}
}
