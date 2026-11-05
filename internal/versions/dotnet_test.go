package versions

import "testing"

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
