//go:build !windows

package pkgmgr

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDetectNixProfiles(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		classic bool
	}{
		{"aliases", false}, {"classic and modern", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			profile := filepath.Join(home, ".local", "state", "nix", "profile")
			if err := os.MkdirAll(profile, 0o755); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join("testdata", "nix-profile-v3.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(profile, "manifest.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			classic := filepath.Join(home, ".nix-profile")
			if tt.classic {
				if err := os.MkdirAll(classic, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(classic, "manifest.nix"), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(profile, classic); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(home, "nix-env")
			if err := os.WriteFile(bin, nil, 0o755); err != nil {
				t.Fatal(err)
			}
			f := &run.Fake{}
			f.Stdout("nix-env (Nix) 2.28.0", bin, "--version")
			query := []string{bin, "--profile", classic, "--query", "--installed", "--json"}
			f.Stdout(`{"key":{"name":"git-2.48","pname":"git","version":"2.48","outputs":{"out":"/nix/store/git"}}}`, query...)
			env := sysenv.Env{"HOME=" + home, "PATH=" + home}
			st, err := Detect(t.Context(), f, env, env)
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if tt.classic {
				want++
			}
			found := false
			for _, m := range st.PackageManagers {
				if m.ID != "nix" {
					continue
				}
				found = true
				if len(m.Packages) != want || len(m.Warnings) != 0 || m.UpdatesKnown {
					t.Fatalf("%+v", m)
				}
			}
			if !found {
				t.Fatal("Nix not detected")
			}
			for _, c := range f.Calls() {
				if c.Args[0] == bin && !slices.Equal(c.Args, []string{bin, "--version"}) && (!tt.classic || !slices.Equal(c.Args, query)) {
					t.Fatalf("unexpected Nix command: %v", c.Args)
				}
			}
		})
	}
}
