package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestPHPWindowsScoop(t *testing.T) {
	dir := t.TempDir()
	php := filepath.Join(dir, "scoop", "apps", "php", "8.4.3", "php.exe")
	writeFile(t, php, "")
	env := sysenv.Env{"USERPROFILE=" + dir, "PATH=", "PATHEXT=.EXE"}
	r := &run.Fake{}
	r.Stdout("Loaded Configuration File: (none)\nAdditional .ini files parsed: (none)", php, "--ini")
	r.Stdout("[PHP Modules]\nCore\n", php, "-m")
	got, err := PHP(t.Context(), r, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installations) != 1 || got.Installations[0].Version != "8.4.3" || got.Installations[0].Source.Manager != "scoop" || got.Installations[0].PHP.Err != "" {
		t.Fatalf("state %+v", got)
	}
}
