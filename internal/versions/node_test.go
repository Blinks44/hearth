package versions

import (
	"os"
	"reflect"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func nodeFixture(t *testing.T) Node {
	t.Helper()
	releases, err := ParseNodeIndex(fixture(t, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	lines, err := ParseNodeSchedule(fixture(t, "schedule.json"))
	if err != nil {
		t.Fatal(err)
	}
	return Node{Releases: releases, Lines: lines}
}

func TestParseNodeIndex(t *testing.T) {
	n := nodeFixture(t)
	// The 27.0.0 alpha is a prerelease, left out.
	if len(n.Releases) != 16 {
		t.Fatalf("%d releases", len(n.Releases))
	}
	want := []Release{
		{Version: "26.13.0", Date: "2026-10-28", LTS: "Lithium"},
		{Version: "26.12.1", Date: "2026-10-21", Security: true},
	}
	if got := n.Releases[:2]; !reflect.DeepEqual(got, want) {
		t.Errorf("first releases %+v, want %+v", got, want)
	}
	if _, err := ParseNodeIndex([]byte("{")); err == nil {
		t.Error("broken index parsed")
	}
}

func TestParseNodeSchedule(t *testing.T) {
	n := nodeFixture(t)
	day := func(s string) time.Time { return date(s) }
	want := Line{Start: day("2024-04-24"), LTS: day("2024-10-29"), Maintenance: day("2025-10-21"), End: day("2027-04-30"), Codename: "Jod"}
	if got := n.Lines[22]; got != want {
		t.Errorf("line 22 %+v, want %+v", got, want)
	}
	// An odd line never becomes LTS.
	if l := n.Lines[23]; !l.LTS.IsZero() || l.Codename != "" || l.End.IsZero() {
		t.Errorf("line 23 %+v", l)
	}
	if _, ok := n.Lines[0]; ok {
		t.Error("a 0.x line kept")
	}
}

func TestLatest(t *testing.T) {
	n := nodeFixture(t)
	var got []string
	for _, r := range n.Latest(date("2026-11-04")) {
		got = append(got, r.Version)
	}
	// 25, 23, 20 and 18 have ended; 0.12 is not in the schedule.
	if want := []string{"26.13.0", "24.22.2", "22.23.4"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Latest = %q, want %q", got, want)
	}
	if r, ok := n.Recommended(); !ok || r.Version != "26.13.0" {
		t.Errorf("Recommended = %+v, %v", r, ok)
	}
}

func TestMajor(t *testing.T) {
	for v, want := range map[string]int{"22.12.0": 22, "v8.1.0": 8, "": -1, "x.1": -1} {
		if got := Major(v); got != want {
			t.Errorf("Major(%q) = %d, want %d", v, got, want)
		}
	}
}
