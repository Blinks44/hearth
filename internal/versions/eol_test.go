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
