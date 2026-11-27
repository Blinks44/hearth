package versions

import (
	"reflect"
	"testing"
)

func TestParseGoReleases(t *testing.T) {
	data := `[{"version":"go1.27.3","stable":true,"files":[]},{"version":"go1.28rc1","stable":false},{"version":"go1.26.10","stable":true}]`
	got, err := ParseGoReleases([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1.27.3", "1.26.10"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := ParseGoReleases([]byte("{")); err == nil {
		t.Error("broken list parsed")
	}
}

func TestGoEnded(t *testing.T) {
	g := &Go{Releases: []string{"1.24.2", "1.23.8"}}
	tests := []struct {
		version string
		want    bool
	}{
		{"1.24.0", false},
		{"1.23.1", false},
		{"1.25.0", false},
		{"1.22.9", true},
		{"go1.21.5", true},
		{"", false},
		{"bad", false},
	}
	for _, tt := range tests {
		if got := g.Ended(tt.version); got != tt.want {
			t.Errorf("Ended(%q) = %v, want %v", tt.version, got, tt.want)
		}
	}
	var nilGo *Go
	if nilGo.Ended("1.20.0") {
		t.Error("nil Go should not report ended")
	}
}
