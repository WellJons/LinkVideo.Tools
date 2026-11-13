package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CameraScan struct {
	Camera     string
	Folder     string
	DatFiles   int
	IndexFound bool
	LastWrite  time.Time
	LatestFile string
	Warning    string
}

type ArchiveScan struct {
	Root    string
	Cameras []CameraScan
	Error   string
}

func newestCameraData(root string, maxCameras int) ArchiveScan {
	result := ArchiveScan{Root: root}
	entries, err := os.ReadDir(root)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for _, camera := range entries {
		if !camera.IsDir() || !strings.HasPrefix(strings.ToLower(camera.Name()), "linkvideo_") {
			continue
		}
		if len(result.Cameras) >= maxCameras {
			break
		}
		c := CameraScan{Camera: camera.Name(), Folder: filepath.Join(root, camera.Name())}
		subdirs, err := os.ReadDir(c.Folder)
		if err != nil {
			c.Warning = err.Error()
			result.Cameras = append(result.Cameras, c)
			continue
		}
		// Nimble stores batches in timestamp-named directories. Check the most recent
		// three, never descend indiscriminately into a customer's entire archive.
		var dirs []string
		for _, e := range subdirs {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
		if len(dirs) > 3 {
			dirs = dirs[:3]
		}
		for _, d := range dirs {
			path := filepath.Join(c.Folder, d)
			children, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			for _, f := range children {
				if f.IsDir() {
					continue
				}
				if strings.EqualFold(f.Name(), "nimble-dvr-segments.db") {
					c.IndexFound = true
				}
				if !strings.EqualFold(filepath.Ext(f.Name()), ".dat") {
					continue
				}
				c.DatFiles++
				info, err := f.Info()
				if err != nil {
					continue
				}
				if info.ModTime().After(c.LastWrite) {
					c.LastWrite = info.ModTime()
					c.LatestFile = filepath.Join(path, f.Name())
				}
			}
		}
		result.Cameras = append(result.Cameras, c)
	}
	sort.Slice(result.Cameras, func(i, j int) bool { return result.Cameras[i].Camera < result.Cameras[j].Camera })
	return result
}

func likelyArchiveFolder() string {
	candidates := archiveCandidates()
	if found := chooseArchiveFolder(candidates); found != "" {
		return found
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return `C:\Users\Public\Documents\LinkVideo.Server\Videos`
}

func apiListening() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:48518", 800*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func installLogFolder() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "LinkVideo.Server", "logs"),
		filepath.Join(os.Getenv("ProgramFiles"), "LinkVideo.Server", "logs"),
		`C:\Program Files (x86)\LinkVideo.Server\logs`,
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return candidates[len(candidates)-1]
}

type LogScan struct {
	File      string
	Errors    int
	Cron      int
	Codec     int
	Network   int
	LastEvent time.Time
	Warning   string
}

func logSnapshot(folder string, since time.Time) LogScan {
	result := LogScan{}
	entries, err := os.ReadDir(folder)
	if err != nil {
		result.Warning = err.Error()
		return result
	}
	var newest os.FileInfo
	var filename string
	for _, ent := range entries {
		if ent.IsDir() || strings.ToLower(filepath.Ext(ent.Name())) != ".log" {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if newest == nil || info.ModTime().After(newest.ModTime()) {
			newest = info
			filename = filepath.Join(folder, ent.Name())
		}
	}
	if newest == nil {
		result.Warning = "Файлы .log отсутствуют"
		return result
	}
	result.File = filename
	f, err := os.Open(filename)
	if err != nil {
		result.Warning = err.Error()
		return result
	}
	defer f.Close()
	// Limit reading to the final 256 KiB. Never copy log contents into the report:
	// raw logs can contain API tokens, RTSP passwords and personal details.
	const limit int64 = 256 * 1024
	start := newest.Size() - limit
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, 0); err != nil {
		result.Warning = err.Error()
		return result
	}
	buf := make([]byte, newest.Size()-start)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		result.Warning = err.Error()
		return result
	}
	lines := strings.Split(string(buf[:n]), "\n")
	for _, line := range lines {
		// Log entries begin like "(1234) 2026/11/10 19:02:01 ..."
		i := strings.Index(line, ") ")
		if i < 0 || len(line) < i+2+19 {
			continue
		}
		t, err := time.ParseInLocation("2006/01/02 15:04:05", line[i+2:i+2+19], time.Local)
		if err != nil || t.Before(since) || t.After(time.Now().Add(1*time.Minute)) {
			continue
		}
		if t.After(result.LastEvent) {
			result.LastEvent = t
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "timeout") {
			result.Errors++
			if strings.Contains(lower, "cron") {
				result.Cron++
			}
			if strings.Contains(lower, "codec") || strings.Contains(lower, "avcconfig") {
				result.Codec++
			}
			if strings.Contains(lower, "request") || strings.Contains(lower, "tls") || strings.Contains(lower, "connection") {
				result.Network++
			}
		}
	}
	return result
}

func ageString(t time.Time) string {
	if t.IsZero() {
		return "нет данных"
	}
	d := time.Since(t)
	if d < 0 {
		return "в будущем (проверь время ПК)"
	}
	if d < time.Minute {
		return fmt.Sprintf("%d сек назад", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d мин назад", int(d.Minutes()))
	}
	return fmt.Sprintf("%.1f ч назад", d.Hours())
}
