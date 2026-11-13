package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// Limit the client to its own test server by rewriting only the network transport.
type localTransport struct {
	host    string
	backend http.RoundTripper
}

func (l localTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c := r.Clone(r.Context())
	u, _ := url.Parse(l.host)
	c.URL.Scheme = u.Scheme
	c.URL.Host = u.Host
	c.Host = u.Host
	return l.backend.RoundTrip(c)
}
func TestNativeSynchronizeLoginAndTrigger(t *testing.T) {
	seen := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login":
			if r.Method != "POST" {
				t.Error("login method")
			}
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["password"] != "pass" {
				t.Error("login password")
			}
			_, _ = w.Write([]byte(`{"token":"test-token"}`))
		case "/api/sync-with-dat":
			seen++
			if r.Method != "GET" || r.Header.Get("token") != "test-token" {
				t.Errorf("method/token failed")
			}
			if r.URL.Query().Get("start_dt") != "1794310053" || r.URL.Query().Get("end_dt") != "1794310113" {
				t.Errorf("params %v", r.URL.Query())
			}
			w.WriteHeader(200)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	cli := server.Client()
	cli.Transport = localTransport{server.URL, http.DefaultTransport}
	r, e := NativeSynchronize(context.Background(), cli, "http://127.0.0.1:48518", "pass", time.Unix(1794310053, 0), time.Unix(1794310113, 0))
	if e != nil || !r.LocalAccepted || seen != 1 {
		t.Fatalf("result %+v err %v count %d", r, e, seen)
	}
}
func TestNativeSyncRejectsBadInputs(t *testing.T) {
	_, e := NativeSynchronize(context.Background(), nil, "https://example.org", "secret", time.Unix(1, 0), time.Unix(2, 0))
	if e == nil {
		t.Fatal("non-local host allowed")
	}
	_, e = NativeSynchronize(context.Background(), nil, "http://127.0.0.1:48518", "", time.Unix(1, 0), time.Unix(2, 0))
	if e == nil {
		t.Fatal("no password allowed")
	}
}
