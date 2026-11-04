package sysenv

import (
	"os"
	"testing"
)

func TestParseOSRelease(t *testing.T) {
	tests := []struct {
		file string
		want OS
	}{
		{"os-release-ubuntu", OS{Name: "Ubuntu", Version: "24.04"}},
		{"os-release-fedora", OS{Name: "Fedora Linux", Version: "41"}},
		{"os-release-arch", OS{Name: "Arch Linux"}}, // rolling: no VERSION_ID
		{"os-release-empty", OS{Name: "Linux"}},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + tt.file)
			if err != nil {
				t.Fatal(err)
			}
			if got := parseOSRelease(data); got != tt.want {
				t.Errorf("parseOSRelease = %+v, want %+v", got, tt.want)
			}
		})
	}
}
