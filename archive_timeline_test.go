package main

import (
	"testing"
	"time"
)

func TestTimelineGaps(t *testing.T) {
	c := CameraArchive{Ranges: []RecordingRange{{Start: 100000, End: 120000}, {Start: 140000, End: 160000}, {Start: 162000, End: 200000}}}
	p := timelineForCamera(c, time.Time{}, false)
	if p.AvailableMs != 78000 || p.MissingMs != 22000 || len(p.Gaps) != 2 {
		t.Fatalf("%+v", p)
	}
	if humanDuration(202600) != "3 мин 22 сек" {
		t.Fatal(humanDuration(202600))
	}
}
func TestDayFilter(t *testing.T) {
	d := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	c := CameraArchive{Ranges: []RecordingRange{{Start: d.Add(-time.Minute).UnixMilli(), End: d.Add(time.Minute).UnixMilli()}, {Start: d.Add(3 * time.Minute).UnixMilli(), End: d.Add(5 * time.Minute).UnixMilli()}}}
	p := timelineForCamera(c, d, true)
	if p.AvailableMs != 180000 || p.MissingMs != 120000 {
		t.Fatalf("%+v", p)
	}
}
