package versions

import (
	"testing"
	"time"
)

func TestNewestLTS(t *testing.T) {
	// As endoflife.date lists Temurin: lts a bool, or the date a line
	// became long-term.
	data := `[{"cycle":"27","releaseDate":"2026-09-22","eol":"2027-03-23","latest":"27.0.1.0.2+1","lts":false},
		{"cycle":"26","releaseDate":"2026-03-23","eol":"2026-09-22","latest":"26.0.2.1+1","lts":false},
		{"cycle":"25","releaseDate":"2025-09-16","eol":"2031-09-30","latest":"25.0.5+8","lts":true},
		{"cycle":"21","releaseDate":"2023-09-19","eol":"2029-09-30","latest":"21.0.9+10","lts":"2023-09-19"}]`
	lines, err := parseCycles("Temurin", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	j := Java{Lines: lines}
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	if got := j.Newest(now); got != "27.0.1.0.2+1" {
		t.Errorf("Newest = %q", got)
	}
	if got := j.NewestLTS(now); got != "25.0.5+8" {
		t.Errorf("NewestLTS = %q", got)
	}
	if l, _ := j.Line("21.0.9"); !l.LTS {
		t.Errorf("21 with an LTS date is not LTS: %+v", l)
	}
	var none *Cycles
	if got := none.NewestLTS(now); got != "" {
		t.Errorf("NewestLTS of nothing = %q", got)
	}
}

func TestCycleEnded(t *testing.T) {
	data := `[{"cycle":"3.14","releaseDate":"2025-10-07","eol":"2030-10-31","latest":"3.14.0","lts":false},
		{"cycle":"3.9","releaseDate":"2020-10-05","eol":"2025-10-31","latest":"3.9.24","lts":false},
		{"cycle":"3.8","releaseDate":"2019-10-14","eol":true,"latest":"3.8.20","lts":false}]`
	lines, err := parseCycles("Python", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	c := &Cycles{Lines: lines}
	if l, ok := c.Line("3.14.0"); !ok || l.Ended(now) {
		t.Errorf("3.14 should be supported: %+v ok=%v", l, ok)
	}
	if l, ok := c.Line("3.9.24"); !ok || !l.Ended(now) {
		t.Errorf("3.9 should be ended by date: %+v ok=%v", l, ok)
	}
	if l, ok := c.Line("3.8.20"); !ok || !l.EOL || !l.Ended(now) {
		t.Errorf("3.8 should be ended by bool eol: %+v ok=%v", l, ok)
	}
	if got := c.Newest(now); got != "3.14.0" {
		t.Errorf("Newest = %q, want 3.14.0", got)
	}
}
