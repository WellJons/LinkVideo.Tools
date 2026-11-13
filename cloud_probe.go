package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// This module performs a READ-ONLY discovery request. File upload methods
// MUST NOT be inferred from an HTTP 200 without validating server contracts.
type CloudProbeResult struct {
	Status  int
	URLHost string
	Message string
}
type serverPublicConfig struct {
	CloudURL       string `json:"CloudUrl"`
	DatServerProxy string `json:"DatServerProxy"`
	Operator       string `json:"Operator"`
}

func localPublicConfig() serverPublicConfig {
	roots := []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles")}
	for _, root := range roots {
		if root == "" {
			continue
		}
		for _, name := range []string{"config.json", "config"} {
			b, e := os.ReadFile(filepath.Join(root, "LinkVideo.Server", name))
			if e != nil {
				continue
			}
			var c serverPublicConfig
			if json.Unmarshal(b, &c) == nil && c.DatServerProxy != "" {
				return c
			}
		}
	}
	return serverPublicConfig{DatServerProxy: "https://lvs-archive.linkvideo.ru", Operator: "linkvideo"}
}
func probeTimelineGaps(ctx context.Context, client *http.Client, base string, camera int64, from, to int64, operator string) (CloudProbeResult, error) {
	if camera <= 0 || from <= 0 || to <= from || to-from > 31*24*3600 {
		return CloudProbeResult{}, errors.New("некорректная камера или временной диапазон")
	}
	u, e := url.Parse(strings.TrimRight(base, "/") + "/lvs/sync/get-timeline-gaps")
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return CloudProbeResult{}, errors.New("некорректный URL облака")
	}
	q := u.Query()
	q.Set("id_camera", strconv.FormatInt(camera, 10))
	q.Set("start_dt", strconv.FormatInt(from, 10))
	q.Set("end_dt", strconv.FormatInt(to, 10))
	q.Set("id_operator", operator)
	u.RawQuery = q.Encode()
	req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return CloudProbeResult{}, e
	}
	req.Header.Set("Accept", "application/json")
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	resp, e := client.Do(req)
	if e != nil {
		return CloudProbeResult{}, fmt.Errorf("соединение с облаком: %w", e)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	res := CloudProbeResult{Status: resp.StatusCode, URLHost: u.Host}
	// Return only an allowlisted diagnostic summary, never raw API response:
	// an error body could contain credentials, cookies or customer details.
	var body map[string]any
	appCode := -1
	if json.Unmarshal(b, &body) == nil {
		if n, ok := body["code"].(float64); ok {
			appCode = int(n)
		}
		for _, key := range []string{"error", "message", "description"} {
			if s, ok := body[key].(string); ok {
				// Avoid echoing arbitrarily formatted text; server message may contain secrets.
				if strings.Contains(strings.ToLower(s), "token") || strings.Contains(strings.ToLower(s), "токен") {
					res.Message = "Сервер сообщил об ошибке токена"
				}
				break
			}
		}
	}
	switch resp.StatusCode {
	case 200:
		if appCode > 0 {
			res.Message = fmt.Sprintf("HTTP 200, API код %d: запрос не принят приложением", appCode)
		} else {
			res.Message = "HTTP 200: получен ответ API; формат пропусков ещё не подтверждён"
		}
	case 400:
		res.Message = "HTTP 400: сервер отклонил параметры запроса"
	case 404:
		res.Message = "HTTP 404: маршрут не найден на архивном сервере; облачная запись не проверена"
	case 401, 403:
		res.Message = "HTTP " + strconv.Itoa(resp.StatusCode) + ": требуется корректная авторизация"
	case 429:
		res.Message = "HTTP 429: ограничение запросов; повторите позже"
	default:
		if res.Message == "" {
			res.Message = "HTTP " + strconv.Itoa(resp.StatusCode) + ": ответ получен"
		}
	}
	return res, nil
}
func cameraNumericID(name string) (int64, error) {
	if !strings.HasPrefix(strings.ToLower(name), "linkvideo_") {
		return 0, errors.New("неизвестный формат ID камеры")
	}
	n, e := strconv.ParseInt(name[len("linkvideo_"):], 10, 64)
	if e != nil || n <= 0 {
		return 0, errors.New("не удалось определить ID камеры")
	}
	return n, nil
}
