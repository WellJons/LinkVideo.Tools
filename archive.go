package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Nimble uses newline-delimited JSON records, not an SQLite database.
// One record points to a byte range inside a .dat file.
type DVRRow struct {
	File     string `json:"file"`
	Time     int64  `json:"time"` // wall-clock UTC milliseconds
	Offset   int64  `json:"offset"`
	Size     int64  `json:"size"`
	Period   int    `json:"period"`
	Type     string `json:"type"`
	Duration int64  `json:"duration"` // ticks, normally video 90000 Hz, AAC 8000 Hz
	Props    struct {
		Timescale int64 `json:"timescale"`
	} `json:"props"`
	path string
}

type RecordingRange struct {
	Start, End int64
	Period     int
	VideoCount int
	AudioCount int
	Blocks     []DVRRow
}

type CameraArchive struct {
	Name     string
	Ranges   []RecordingRange
	Count    int
	Problems []string
}

type ArchiveCatalog struct {
	Root    string
	Cameras []CameraArchive
	Warning string
}

func parseNimbleIndex(path string) ([]DVRRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []DVRRow
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		l := strings.TrimSpace(strings.TrimSuffix(scanner.Text(), ","))
		if !strings.HasPrefix(l, "{") {
			continue
		}
		var r DVRRow
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			continue
		}
		if r.File == "" || r.Size <= 0 || r.Time <= 0 || r.Offset < 0 || filepath.Base(r.File) != r.File || strings.ContainsAny(r.File, `/\`) {
			continue
		}
		r.path = filepath.Join(filepath.Dir(path), r.File)
		rows = append(rows, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rows, nil
}

func isMedia(t string) bool { return t == ".m4v" || t == ".m4a" }
func rowEndMs(r DVRRow) int64 {
	if !isMedia(r.Type) || r.Duration <= 0 {
		return r.Time
	}
	ticks := int64(90000)
	if r.Type == ".m4a" {
		ticks = 8000
	}
	if r.Props.Timescale > 0 {
		ticks = r.Props.Timescale
	}
	return r.Time + r.Duration*1000/ticks
}
func fileRangeValid(r DVRRow, cache map[string]int64) bool {
	v, ok := cache[r.path]
	if !ok {
		st, err := os.Stat(r.path)
		if err != nil || !st.Mode().IsRegular() {
			v = -1
		} else {
			v = st.Size()
		}
		cache[r.path] = v
	}
	return r.Offset >= 0 && r.Size > 0 && v >= 0 && r.Offset <= v && r.Size <= v-r.Offset
}
func scanArchiveCatalog(root string) (ArchiveCatalog, error) {
	cat := ArchiveCatalog{Root: root}
	dirs, err := os.ReadDir(root)
	if err != nil {
		return cat, err
	}
	sizes := map[string]int64{}
	for _, dir := range dirs {
		if !dir.IsDir() || !strings.HasPrefix(strings.ToLower(dir.Name()), "linkvideo_") {
			continue
		}
		cam := CameraArchive{Name: dir.Name()}
		parent := filepath.Join(root, dir.Name())
		sub, err := os.ReadDir(parent)
		if err != nil {
			cam.Problems = append(cam.Problems, "Нельзя открыть папку камеры")
			cat.Cameras = append(cat.Cameras, cam)
			continue
		}
		var rows []DVRRow
		for _, entry := range sub {
			if !entry.IsDir() {
				continue
			}
			idx := filepath.Join(parent, entry.Name(), "nimble-dvr-segments.db")
			if _, err := os.Stat(idx); err != nil {
				continue
			}
			got, err := parseNimbleIndex(idx)
			if err != nil {
				cam.Problems = append(cam.Problems, "Ошибка чтения индекса: "+entry.Name())
				continue
			}
			rows = append(rows, got...)
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].Time != rows[j].Time {
				return rows[i].Time < rows[j].Time
			}
			return rows[i].Type < rows[j].Type
		})
		// Index may contain entries for files that have been deleted: do not report those as available.
		var valid []DVRRow
		for _, r := range rows {
			if fileRangeValid(r, sizes) {
				valid = append(valid, r)
			}
		}
		video := make([]DVRRow, 0)
		for _, r := range valid {
			if r.Type == ".m4v" && r.Duration > 0 {
				video = append(video, r)
			}
		}
		var ranges []RecordingRange
		const gapToleranceMs = int64(3500)
		for _, r := range video {
			if len(ranges) == 0 || ranges[len(ranges)-1].Period != r.Period || r.Time > ranges[len(ranges)-1].End+gapToleranceMs {
				ranges = append(ranges, RecordingRange{Start: r.Time, End: rowEndMs(r), Period: r.Period, VideoCount: 1})
			} else {
				p := &ranges[len(ranges)-1]
				if rowEndMs(r) > p.End {
					p.End = rowEndMs(r)
				}
				p.VideoCount++
			}
		}
		// Attach only records that exist, have compatible period, and intersect the video interval.
		for i := range ranges {
			p := &ranges[i]
			for _, r := range valid {
				if r.Period != p.Period {
					continue
				}
				switch r.Type {
				case ".m4v", ".m4a":
					if r.Time <= p.End+2000 && rowEndMs(r) >= p.Start-2000 {
						p.Blocks = append(p.Blocks, r)
						if r.Type == ".m4a" {
							p.AudioCount++
						}
					}
				case ".m4mv", ".m4ma":
					// Initialisation data chosen during export, even when stored in another file.
				}
			}
		}
		cam.Ranges = ranges
		cam.Count = len(video)
		if len(rows) > 0 && len(video) == 0 {
			cam.Problems = append(cam.Problems, "Есть индекс, но видеосегменты на диске отсутствуют")
		}
		cat.Cameras = append(cat.Cameras, cam)
	}
	sort.Slice(cat.Cameras, func(i, j int) bool { return cat.Cameras[i].Name < cat.Cameras[j].Name })
	return cat, nil
}

// Export creates two temporary valid fragmented MP4 inputs from indexed byte ranges,
// then muxes them into a standard MP4 with video copied, sound transcoded if necessary.
// It NEVER modifies the Nimble source index or .dat files.
func ExportRecording(ctx context.Context, cat ArchiveCatalog, cameraIndex, rangeIndex int, ffmpegPath, outFile string) error {
	return ExportRecordingRange(ctx, cat, cameraIndex, rangeIndex, 0, 0, ffmpegPath, outFile)
}

func ExportRecordingRange(ctx context.Context, cat ArchiveCatalog, cameraIndex, rangeIndex int, fromMs, toMs int64, ffmpegPath, outFile string) error {
	if cameraIndex < 0 || cameraIndex >= len(cat.Cameras) {
		return errors.New("камера не выбрана")
	}
	cam := cat.Cameras[cameraIndex]
	if rangeIndex < 0 || rangeIndex >= len(cam.Ranges) {
		return errors.New("период не выбран")
	}
	interval := cam.Ranges[rangeIndex]
	if interval.VideoCount == 0 {
		return errors.New("в выбранном периоде нет видео")
	}
	if fromMs == 0 {
		fromMs = interval.Start
	}
	if toMs == 0 {
		toMs = interval.End
	}
	if fromMs >= toMs || fromMs < interval.Start-1000 || toMs > interval.End+1000 {
		return errors.New("заданный диапазон выходит за пределы непрерывной записи")
	}
	if _, err := os.Stat(outFile); err == nil {
		return errors.New("выходной MP4 уже существует — перезапись запрещена")
	}
	tmp, err := os.MkdirTemp("", "linkvideo-archive-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	// Locate the closest compatible preceding init records within all the camera indexes.
	initVideo, initAudio, err := findInitRecords(cat.Root, cam.Name, interval.Start, interval.Period)
	if err != nil {
		return err
	}
	vName := filepath.Join(tmp, "video.mp4")
	aName := filepath.Join(tmp, "audio.mp4")
	videoRows := []DVRRow{initVideo}
	audioRows := []DVRRow{}
	if initAudio.Size > 0 {
		audioRows = append(audioRows, initAudio)
	}
	for _, r := range interval.Blocks {
		if r.Time >= toMs || rowEndMs(r) <= fromMs {
			continue
		}
		switch r.Type {
		case ".m4v":
			videoRows = append(videoRows, r)
		case ".m4a":
			audioRows = append(audioRows, r)
		}
	}
	if len(videoRows) < 2 {
		return errors.New("нет видеоблоков в заданном диапазоне")
	}
	if err := buildFMP4(ctx, vName, videoRows); err != nil {
		return fmt.Errorf("видео: %w", err)
	}
	useAudio := len(audioRows) > 1
	if useAudio {
		if err := buildFMP4(ctx, aName, audioRows); err != nil {
			useAudio = false
		}
	}
	if err := os.MkdirAll(filepath.Dir(outFile), 0755); err != nil {
		return err
	}
	partial := outFile + ".partial.mp4"
	defer os.Remove(partial)
	args := []string{"-hide_banner", "-nostdin", "-v", "error", "-y", "-i", vName}
	if useAudio {
		args = append(args, "-i", aName, "-map", "0:v:0", "-map", "1:a:0", "-c:v", "copy", "-c:a", "aac", "-af", "aresample=async=1:first_pts=0")
	} else {
		args = append(args, "-map", "0:v:0", "-c:v", "copy", "-an")
	}
	args = append(args, "-movflags", "+faststart", partial)
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	cmd.Dir = filepath.Dir(ffmpegPath) // DLLs shipped beside LinkVideo.Server's FFmpeg.
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg: %v (%s)", err, truncateText(string(out), 1600))
	}
	stat, err := os.Stat(partial)
	if err != nil || stat.Size() < 1024 {
		return errors.New("FFmpeg не создал корректный файл")
	}
	if err := os.Rename(partial, outFile); err != nil {
		return err
	}
	return nil
}
func truncateText(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func buildFMP4(ctx context.Context, dest string, rows []DVRRow) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, r := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		input, err := os.Open(r.path)
		if err != nil {
			return err
		}
		_, err = input.Seek(r.Offset, io.SeekStart)
		if err == nil {
			_, err = io.CopyN(f, input, r.Size)
		}
		input.Close()
		if err != nil {
			return err
		}
	}
	return f.Sync()
}
func findInitRecords(root, camName string, started int64, period int) (DVRRow, DVRRow, error) {
	var vid, aud DVRRow
	sub, err := os.ReadDir(filepath.Join(root, camName))
	if err != nil {
		return vid, aud, err
	}
	cache := map[string]int64{}
	for _, d := range sub {
		if !d.IsDir() {
			continue
		}
		rows, err := parseNimbleIndex(filepath.Join(root, camName, d.Name(), "nimble-dvr-segments.db"))
		if err != nil {
			continue
		}
		for _, r := range rows {
			if r.Period != period || r.Time > started || !fileRangeValid(r, cache) {
				continue
			}
			switch r.Type {
			case ".m4mv":
				if r.Time >= vid.Time {
					vid = r
				}
			case ".m4ma":
				if r.Time >= aud.Time {
					aud = r
				}
			}
		}
	}
	if vid.Size == 0 {
		return vid, aud, errors.New("не найдена инициализация видео .m4mv — экспорт невозможен")
	}
	return vid, aud, nil
}
func locateFFmpeg() string {
	candidates := []string{filepath.Join(os.Getenv("ProgramFiles(x86)"), "LinkVideo.Server", "ffmpeg.exe"), filepath.Join(os.Getenv("ProgramFiles"), "LinkVideo.Server", "ffmpeg.exe"), `C:\Program Files (x86)\LinkVideo.Server\ffmpeg.exe`}
	for _, p := range candidates {
		if i, err := os.Stat(p); err == nil && !i.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return ""
}
func archiveExportDir() string {
	if p := os.Getenv("USERPROFILE"); p != "" {
		return filepath.Join(p, "Videos", "LinkVideoExports")
	}
	if h, e := os.UserHomeDir(); e == nil {
		return filepath.Join(h, "Videos", "LinkVideoExports")
	}
	return filepath.Join(os.TempDir(), "LinkVideoExports")
}
func exportFilename(cam string, r RecordingRange) string {
	return fmt.Sprintf("%s_%s_%s.mp4", cam, time.UnixMilli(r.Start).Local().Format("20060102_150405"), time.UnixMilli(r.End).Local().Format("150405"))
}
