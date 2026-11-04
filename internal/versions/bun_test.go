package versions

import "testing"

func TestParseNpmLatest(t *testing.T) {
	for _, tt := range []struct {
		data, want string
		ok         bool
	}{
		{`{"name":"bun","version":"1.2.8","bin":{"bun":"bin/bun.exe"}}`, "1.2.8", true},
		{`{"name":"bun"}`, "", false},
		{`{`, "", false},
	} {
		got, err := ParseNpmLatest([]byte(tt.data))
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("ParseNpmLatest(%s) = %q, %v", tt.data, got, err)
		}
	}
}
