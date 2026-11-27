package versions

import (
	"testing"
	"time"
)

func TestRustTrain(t *testing.T) {
	lines, err := parseCycles("Rust", []byte(`[{"cycle":"1.99","releaseDate":"2026-10-01","eol":false,"latest":"1.99.1"},{"cycle":"1.98","latest":"1.98.1","eol":"2026-10-01"}]`))
	if err != nil {
		t.Fatal(err)
	}
	r := Rust{Lines: lines}
	got, err := r.Train()
	if err != nil {
		t.Fatal(err)
	}
	if got.Stable != "1.99.1" || got.Beta != "1.100.0" || got.Nightly != "1.101.0" || got.NextStable.Format("2006-01-02") != "2026-11-12" {
		t.Fatalf("train %+v", got)
	}
	if _, err := (&Rust{}).Train(); err == nil {
		t.Fatal("train of no lines")
	}
}

func TestRustEnded(t *testing.T) {
	now := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	r := &Rust{Lines: []Cycle{
		{Cycle: "1.100", Latest: "1.100.0"},
		{Cycle: "1.99", Latest: "1.99.1", End: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)},
		{Cycle: "1.98", Latest: "1.98.1", End: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		{Cycle: "1.75", Latest: "1.75.0", EOL: true},
	}}
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"1.100.0", false},
		{"1.99.1", false}, // previous 6-week minor is an ordinary update, not EOL
		{"1.98.1", true},  // two minors behind
		{"1.75.0", true},  // much older EOL
	} {
		if got := RustEnded(r, tc.version, now); got != tc.want {
			t.Errorf("RustEnded(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
