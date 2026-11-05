package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestLocalManagers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	current := filepath.Join(root, "apps", "nodejs", "current")
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "manifest.json"), []byte(`{"version":"22.1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"SCOOP=" + root, "SCOOP_GLOBAL=" + root}
	got := localManagers(env)
	if len(got) != 1 || len(got[0].Packages) != 1 || got[0].Packages[0].Version != "22.1" || got[0].UpdatesKnown {
		t.Fatalf("%+v", got)
	}
	if err := os.WriteFile(filepath.Join(current, "manifest.json"), []byte("broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	got = localManagers(env)
	if len(got) != 1 || len(got[0].Packages) != 0 || len(got[0].Warnings) != 1 {
		t.Fatalf("%+v", got)
	}
}
