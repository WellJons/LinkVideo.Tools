package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeInput(t *testing.T) {
	_, e := probeTimelineGaps(context.Background(), nil, "https://test.invalid", 301547, 100, 100, "linkvideo")
	if e == nil {
		t.Fatal("equal times accepted")
	}
	_, e = probeTimelineGaps(context.Background(), nil, "http://test.invalid", 301547, 100, 200, "linkvideo")
	if e == nil {
		t.Fatal("http accepted")
	}
}
func TestProbeQuery(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lvs/sync/get-timeline-gaps" || r.URL.Query().Get("start_dt") != "1794310053" || r.URL.Query().Get("id_camera") != "301547" || r.URL.Query().Get("id_operator") != "linkvideo" {
			t.Errorf("incorrect request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		w.Write([]byte(`{"error":"Token actual-value-must-not-be-echoed"}`))
	}))
	defer s.Close()
	res, err := probeTimelineGaps(context.Background(), s.Client(), s.URL, 301547, 1794310053, 1794310233, "linkvideo")
	if err != nil || res.Status != 403 || res.Message != "HTTP 403: требуется корректная авторизация" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestCloudApplicationError(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":3,"error":"Токен не существует"}`))
	}))
	defer s.Close()
	res, e := probeTimelineGaps(context.Background(), s.Client(), s.URL, 301547, 1794310053, 1794310233, "linkvideo")
	if e != nil || res.Status != 200 || res.Message != "HTTP 200, API код 3: запрос не принят приложением" {
		t.Fatalf("%+v %v", res, e)
	}
}
