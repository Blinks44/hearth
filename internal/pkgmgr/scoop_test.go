package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestScoopManagers(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, installed, latest, metadata string
		available                         string
		known                             bool
	}{
		{"update", "22.1", "24.0", `{"bucket":"main"}`, "24.0", true},
		{"equal", "24.0", "24.0", `{"bucket":"main"}`, "", true},
		{"newer installed", "25.0", "24.0", `{"bucket":"main"}`, "", true},
		{"four parts", "1.2.3.4", "1.2.3.5", `{"bucket":"main"}`, "1.2.3.5", true},
		{"held", "22.1", "24.0", `{"bucket":"main","hold":true}`, "", true},
		{"missing bucket", "22.1", "24.0", `{}`, "", false},
		{"invalid source", "22.1", "24.0", `{"bucket":"../main"}`, "", false},
		{"broken metadata", "22.1", "24.0", `broken`, "", false},
		{"nightly", "nightly", "nightly", `{"bucket":"main"}`, "", false},
		{"unknown version scheme", "nightly-20260101", "nightly", `{"bucket":"main"}`, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			current := filepath.Join(root, "apps", "nodejs", "current")
			writeScoopFile(t, filepath.Join(current, "manifest.json"), `{"version":"`+tt.installed+`"}`)
			writeScoopFile(t, filepath.Join(current, "install.json"), tt.metadata)
			writeScoopFile(t, filepath.Join(root, "buckets", "main", "bucket", "nodejs.json"), `{"version":"`+tt.latest+`"}`)
			env := sysenv.Env{"SCOOP=" + root, "SCOOP_GLOBAL=" + root}
			got := scoopManagers(env)
			if len(got) != 1 || len(got[0].Packages) != 1 {
				t.Fatalf("%+v", got)
			}
			m := got[0]
			if m.UpdatesKnown != tt.known || m.Packages[0].Version != tt.installed || m.Packages[0].Available != tt.available {
				t.Fatalf("%+v", m)
			}
			if !tt.known && len(m.Warnings) == 0 {
				t.Fatal("missing explanation")
			}
		})
	}
}

func TestScoopGlobalAndOverrides(t *testing.T) {
	t.Parallel()
	user, global := t.TempDir(), t.TempDir()
	current := filepath.Join(global, "apps", "nodejs", "current")
	writeScoopFile(t, filepath.Join(current, "manifest.json"), `{"version":"wrong"}`)
	writeScoopFile(t, filepath.Join(current, "scoop-manifest.json"), `{"version":"22.1"}`)
	writeScoopFile(t, filepath.Join(current, "install.json"), `{"bucket":"wrong"}`)
	writeScoopFile(t, filepath.Join(current, "scoop-install.json"), `{"bucket":"main"}`)
	bucket := filepath.Join(user, "buckets", "main", "bucket", "nodejs.json")
	writeScoopFile(t, bucket, `{"version":"24.0"}`)
	env := sysenv.Env{"SCOOP=" + user, "SCOOP_GLOBAL=" + global}
	got := scoopManagers(env)
	if len(got) != 1 || !got[0].UpdatesKnown || got[0].Packages[0].Available != "24.0" {
		t.Fatalf("%+v", got)
	}
	if err := os.Remove(bucket); err != nil {
		t.Fatal(err)
	}
	got = scoopManagers(env)
	if got[0].UpdatesKnown || got[0].Packages[0].Available != "" || len(got[0].Warnings) != 1 {
		t.Fatalf("%+v", got)
	}
	// Legacy buckets store manifests directly in the bucket root.
	writeScoopFile(t, filepath.Join(user, "buckets", "main", "nodejs.json"), `{"version":"24.0"}`)
	got = scoopManagers(env)
	if !got[0].UpdatesKnown || got[0].Packages[0].Available != "24.0" {
		t.Fatalf("%+v", got)
	}
}

func TestCompareDottedVersions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, a, b string
		cmp        int
		ok         bool
	}{
		{"numeric ordering", "1.10", "1.9", 1, true},
		{"leading zeros", "001.02", "1.2", 0, true},
		{"trailing zero", "1.2.0", "1.2", 0, true},
		{"large numbers", "999999999999999999999999", "99999999999999999999999", 1, true},
		{"less", "1.2", "1.3", -1, true},
		{"prerelease", "1.2-rc1", "1.2", 0, false},
		{"empty", "", "1.2", 0, false},
		{"empty segment", "1..2", "1.2", 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := compareDottedVersions(tt.a, tt.b)
			if got != tt.cmp || ok != tt.ok {
				t.Fatalf("got %d,%v", got, ok)
			}
		})
	}
}

func writeScoopFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}
