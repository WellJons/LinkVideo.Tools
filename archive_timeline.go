package main

import (
	"fmt"
	"sort"
	"time"
)

// LocalTimeline is derived exclusively from existing and in-bounds Nimble DVR data.
// Its gaps describe missing LOCAL recording, not missing cloud copies.
type LocalTimeline struct {
	Start, End  int64
	AvailableMs int64
	MissingMs   int64
	Ranges      []RecordingRange
	Gaps        []ArchiveGap
}
type ArchiveGap struct{ Start, End int64 }

func timelineForCamera(cam CameraArchive, day time.Time, onlyDay bool) LocalTimeline {
	var out LocalTimeline
	low, high := int64(-1<<63), int64(1<<63-1)
	if onlyDay {
		t := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
		low, high = t.UnixMilli(), t.AddDate(0, 0, 1).UnixMilli()
	}
	var intervals []RecordingRange
	for _, r := range cam.Ranges {
		if r.End <= low || r.Start >= high || r.End <= r.Start {
			continue
		}
		if r.Start < low {
			r.Start = low
		}
		if r.End > high {
			r.End = high
		}
		intervals = append(intervals, r)
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].Start < intervals[j].Start })
	for _, r := range intervals {
		if len(out.Ranges) > 0 && r.Start <= out.Ranges[len(out.Ranges)-1].End {
			p := &out.Ranges[len(out.Ranges)-1]
			if r.End > p.End {
				p.End = r.End
			}
			p.VideoCount += r.VideoCount
			p.AudioCount += r.AudioCount
		} else {
			out.Ranges = append(out.Ranges, r)
		}
	}
	if len(out.Ranges) == 0 {
		return out
	}
	out.Start = out.Ranges[0].Start
	out.End = out.Ranges[len(out.Ranges)-1].End
	for i, r := range out.Ranges {
		out.AvailableMs += r.End - r.Start
		if i > 0 && r.Start > out.Ranges[i-1].End {
			g := ArchiveGap{out.Ranges[i-1].End, r.Start}
			out.Gaps = append(out.Gaps, g)
			out.MissingMs += g.End - g.Start
		}
	}
	return out
}
func humanDuration(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	sec := ms / 1000
	h, m, s := sec/3600, (sec%3600)/60, sec%60
	if h > 0 {
		return fmt.Sprintf("%d ч %d мин", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%d мин %d сек", m, s)
	}
	return fmt.Sprintf("%d сек", s)
}
