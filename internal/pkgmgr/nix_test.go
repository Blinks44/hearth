package pkgmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestParseNixProfile(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, file string
		count      int
	}{
		{"v1", "nix-profile-v1.json", 1},
		{"v2 multiple outputs", "nix-profile-v2.json", 1},
		{"v3 named elements", "nix-profile-v3.json", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			got, err := parseNixProfile(data)
			if err != nil || len(got) != tt.count {
				t.Fatalf("%+v, %v", got, err)
			}
			if got[0].Name != "hello" || got[0].Version != "2.12.1" || got[0].Kind != "profile" || !strings.HasSuffix(got[0].Path, "-hello-2.12.1") {
				t.Fatalf("%+v", got)
			}
		})
	}
	for _, tt := range []struct{ name, input string }{
		{"broken", "{"},
		{"future", `{"version":4,"elements":{}}`},
		{"missing", `{"version":3}`},
		{"null", `{"version":3,"elements":null}`},
		{"wrong shape", `{"version":3,"elements":[]}`},
		{"no paths", `{"version":3,"elements":{"hello":{}}}`},
		{"relative path", `{"version":2,"elements":[{"storePaths":["relative"]}]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := parseNixProfile([]byte(tt.input)); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
	for _, input := range []string{`{"version":1,"elements":[]}`, `{"version":3,"elements":{}}`} {
		got, err := parseNixProfile([]byte(input))
		if err != nil || len(got) != 0 {
			t.Fatalf("empty profile: %+v, %v", got, err)
		}
	}
}

func TestNixProfiles(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	state := t.TempDir()
	env := sysenv.Env{"HOME=" + home, "XDG_STATE_HOME=" + state}
	probe := func(args ...string) ([]byte, error) { return nil, fmt.Errorf("unexpected probe: %v", args) }
	m := inventory.PackageManager{}
	if nixProfiles(&m, env, probe) {
		t.Fatal("absent profile suppresses classic probe")
	}
	profile := filepath.Join(state, "nix", "profile")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "nix-profile-v3.json"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(profile, "manifest.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, ".nix-profile")
	if err := os.MkdirAll(alias, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(alias, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if !nixProfiles(&m, env, probe) || len(m.Packages) != 1 || len(m.Warnings) != 0 || m.UpdatesKnown {
		t.Fatalf("%+v", m)
	}
	if err := os.Remove(filepath.Join(alias, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	m = inventory.PackageManager{}
	if !nixProfiles(&m, env, probe) || len(m.Packages) != 0 || len(m.Warnings) == 0 {
		t.Fatalf("%+v", m)
	}
}
