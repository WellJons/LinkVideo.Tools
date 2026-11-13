package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// NativeSyncResult confirms the local API response, NOT cloud upload completion.
type NativeSyncResult struct {
	LocalHTTPStatus int
	LocalAccepted   bool
	Duration        time.Duration
}

var errLocalAuthorization = errors.New("локальный сервер отклонил пароль или сессию")

// NativeSynchronize delegates the operation to LinkVideo.Server 1.1.12.
// The local endpoint operates on ALL configured cameras, not just the
// selected one. No direct cloud requests or writes to Nimble files occur.
func NativeSynchronize(ctx context.Context, httpClient *http.Client, base string, password string, from, to time.Time) (NativeSyncResult, error) {
	var out NativeSyncResult
	if !from.Before(to) || to.Sub(from) > 24*time.Hour || from.Unix() < 1 {
		return out, errors.New("неверный диапазон: не более 24 часов")
	}
	if password == "" {
		return out, errors.New("нужен локальный пароль LinkVideo.Server")
	}
	if base != "http://127.0.0.1:48518" && base != "http://localhost:48518" {
		return out, errors.New("разрешён только локальный LinkVideo.Server на 48518")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	authData, err := json.Marshal(struct {
		Password string `json:"password"`
	}{password})
	if err != nil {
		return out, err
	}
	loginReq, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/auth/login", bytes.NewReader(authData))
	if err != nil {
		return out, err
	}
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := httpClient.Do(loginReq)
	if err != nil {
		return out, fmt.Errorf("соединение с локальным сервером: %w", err)
	}
	if loginResp.StatusCode != http.StatusOK {
		loginResp.Body.Close()
		if loginResp.StatusCode == http.StatusUnauthorized || loginResp.StatusCode == http.StatusConflict {
			return out, errLocalAuthorization
		}
		return out, fmt.Errorf("вход в локальный сервер: HTTP %d", loginResp.StatusCode)
	}
	var payload struct {
		Token string `json:"token"`
	}
	err = json.NewDecoder(io.LimitReader(loginResp.Body, 4096)).Decode(&payload)
	loginResp.Body.Close()
	if err != nil || payload.Token == "" {
		return out, errors.New("локальный сервер не вернул токен авторизации")
	}

	u, err := url.Parse(base + "/api/sync-with-dat")
	if err != nil {
		return out, err
	}
	q := u.Query()
	q.Set("start_dt", strconv.FormatInt(from.Unix(), 10))
	q.Set("end_dt", strconv.FormatInt(to.Unix(), 10))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("token", payload.Token)
	req.Header.Set("Accept", "application/json")
	begin := time.Now()
	response, err := httpClient.Do(req)
	out.Duration = time.Since(begin)
	if err != nil {
		return out, fmt.Errorf("выполнение штатной синхронизации: %w", err)
	}
	defer response.Body.Close()
	// Avoid returning raw response content or header values: secrets may be present.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8192))
	out.LocalHTTPStatus = response.StatusCode
	out.LocalAccepted = response.StatusCode == http.StatusOK
	if response.StatusCode == http.StatusUnauthorized {
		return out, errLocalAuthorization
	}
	if response.StatusCode != http.StatusOK {
		return out, fmt.Errorf("локальный API вернул HTTP %d", response.StatusCode)
	}
	return out, nil
}
