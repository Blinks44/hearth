package versions

import (
	"testing"
	"time"
)

func TestParseDotnet(t *testing.T) {
	for _, tt := range []struct {
		name, data string
		bad        bool
	}{
		{"index", `{"releases-index":[{"latest-sdk":"9.0.200","release-type":"sts","support-phase":"active","eol-date":"2026-05-12"}]}`, false},
		{"empty", `{"releases-index":[]}`, true},
		{"invalid", `{`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDotnet([]byte(tt.data))
			if (err != nil) != tt.bad {
				t.Fatal(err)
			}
			if !tt.bad && (len(got) != 1 || got[0].Version != "9.0.200" || got[0].Support != "sts" || got[0].End != "2026-05-12") {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestDotnetLineAndEnded(t *testing.T) {
	d := &Dotnet{Lines: []DotnetLine{
		{Version: "9.0.200", Support: "sts", Phase: "active", End: "2026-11-10"},
		{Version: "8.0.404", Support: "lts", Phase: "active", End: "2026-11-10"},
		{Version: "6.0.428", Support: "lts", Phase: "eol", End: "2024-11-12"},
	}}
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	if l, ok := d.Line("8.0.100"); !ok || l.Version != "8.0.404" || l.Ended(now) {
		t.Errorf("8.0.100: %+v ok=%v", l, ok)
	}
	if l, ok := d.Line("6.0.100"); !ok || !l.Ended(now) {
		t.Errorf("6.0.100 should be eol: %+v ok=%v", l, ok)
	}
	later := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if l, _ := d.Line("9.0.100"); !l.Ended(later) {
		t.Errorf("9.0 should be ended after eol-date: %+v", l)
	}
}
