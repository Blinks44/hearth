package detect

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestPHPWindowsScoop(t *testing.T) {
	dir, err := sysenv.RealPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	php := filepath.Join(dir, "scoop", "apps", "php", "8.4.3", "php.exe")
	writeFile(t, php, "")
	env := sysenv.Env{"USERPROFILE=" + dir, "PATH=" + filepath.Dir(php), "PATHEXT=.EXE"}
	r := &run.Fake{}
	r.Stdout("Loaded Configuration File: (none)\nAdditional .ini files parsed: (none)", php, "--ini")
	r.Stdout("[PHP Modules]\nCore\n", php, "-m")
	got, err := PHP(t.Context(), r, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installations) != 1 || got.Installations[0].Version != "8.4.3" || got.Installations[0].Source.Manager != "scoop" || got.Installations[0].PHP == nil || got.Installations[0].PHP.Err != "" {
		t.Fatalf("state %+v", got)
	}
}

// Scoop's shim on PATH runs the version current is a junction to: one
// copy, the one the terminal runs.
func TestPHPWindowsScoopShim(t *testing.T) {
	dir, err := sysenv.RealPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	apps := filepath.Join(dir, "scoop", "apps", "php")
	php := filepath.Join(apps, "8.5.12-1", "php.exe")
	writeFile(t, php, "")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(apps, "current"), filepath.Dir(php)).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v: %s", err, out)
	}
	shims := filepath.Join(dir, "scoop", "shims")
	writeFile(t, filepath.Join(shims, "php.exe"), "")
	writeFile(t, filepath.Join(shims, "php.shim"), `path = "`+filepath.Join(apps, "current", "php.exe")+`"`+"\r\n")
	env := sysenv.Env{"USERPROFILE=" + dir, "PATH=" + shims, "PATHEXT=.EXE"}
	got, err := PHP(t.Context(), &run.Fake{}, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installations) != 1 {
		t.Fatalf("copies %+v", got.Installations)
	}
	in := got.Installations[0]
	if in.RealPath != php || in.Version != "8.5.12" || in.OnPath != 1 || in.Source.Shim || in.Source.Manager != "scoop" {
		t.Errorf("copy %+v", in)
	}
}

func TestScoopShimTarget(t *testing.T) {
	shims := t.TempDir()
	exe, cmd, other := filepath.Join(shims, "php.exe"), filepath.Join(shims, "composer.cmd"), filepath.Join(shims, "tool.cmd")
	writeFile(t, exe, "")
	writeFile(t, filepath.Join(shims, "php.shim"), `path = "C:\scoop\apps\php\current\php.exe"`+"\r\n")
	writeFile(t, cmd, "@rem C:\\scoop\\apps\\composer\\current\\composer.ps1\r\n@echo off\r\n")
	writeFile(t, other, "@echo off\r\n")
	for path, want := range map[string]string{
		exe:   `C:\scoop\apps\php\current\php.exe`,
		cmd:   `C:\scoop\apps\composer\current\composer.ps1`,
		other: "",
	} {
		if got := scoopShimTarget(path); got != want {
			t.Errorf("%s: got %q, want %q", filepath.Base(path), got, want)
		}
	}
}
