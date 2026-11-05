//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestPHPDiscovery(t *testing.T) {
	dir := root(t)
	mise := filepath.Join(dir, "home", ".local", "share", "mise")
	shim := filepath.Join(mise, "shims", "php")
	php := filepath.Join(mise, "installs", "php", "8.4.3", "bin", "php")
	for _, path := range []string{shim, php} {
		writeFile(t, path, "")
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, "home", ".config", "mise", "config.toml"), "[tools]\nphp = '8.4.3'\n")
	env := homeEnv(dir, filepath.Dir(shim))
	r := &run.Fake{}
	r.Stdout("Loaded Configuration File: /etc/php.ini\nAdditional .ini files parsed: (none)", php, "--ini")
	r.Stdout("[PHP Modules]\nCore\nredis\n", php, "-m")
	got, err := PHP(t.Context(), r, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	var copy *inventory.Installation
	for i := range got.Installations {
		if got.Installations[i].RealPath == php {
			copy = &got.Installations[i]
		}
	}
	if copy == nil || copy.Version != "8.4.3" || !copy.Default || copy.Source.Manager != "mise" || copy.PHP == nil || len(copy.PHP.Extensions) != 2 || copy.PHP.Err != "" {
		t.Fatalf("copy %+v; state %+v", copy, got)
	}
	if inventory.TerminalCopy(got.Installations) < 0 {
		t.Fatal("shim default unresolved")
	}
	for _, call := range r.Calls() {
		if call.Args[0] == shim {
			t.Fatalf("ran shim: %v", call.Args)
		}
	}
}

func TestPHPFailedConfig(t *testing.T) {
	dir := root(t)
	path := filepath.Join(dir, "bin", "php")
	writeFile(t, path, "")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &run.Fake{}
	r.Stdout("PHP 8.3.16 (cli)\nCopyright", path, "-v")
	got, err := PHP(t.Context(), r, homeEnv(dir, filepath.Dir(path)), sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Installations[0].Version != "8.3.16" || got.Installations[0].PHP.Err == "" {
		t.Fatalf("copy %+v", got.Installations[0])
	}
}

func TestComposerCommands(t *testing.T) {
	dir := root(t)
	path := filepath.Join(dir, "bin", "composer")
	writeFile(t, path, "")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &run.Fake{}
	prefix := []string{path, "--no-plugins", "--no-scripts", "--no-interaction"}
	r.Stdout("Composer version 2.8.5 2025-01-21", append(prefix, "--version")...)
	composerHome := filepath.Join(dir, "composer")
	r.Stdout(composerHome, append(prefix, "config", "--global", "home")...)
	got, err := Composer(t.Context(), r, homeEnv(dir, filepath.Dir(path)), sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installations) != 1 || got.Installations[0].Version != "2.8.5" || got.Vars["COMPOSER_HOME"] != composerHome {
		t.Fatalf("state %+v", got)
	}
	for _, call := range r.Calls() {
		if call.Dir != filepath.Join(dir, "home") {
			t.Fatalf("used project directory: %+v", call)
		}
	}
}

// A version in the path and COMPOSER_HOME leave nothing to run Composer
// for.
func TestComposerWithoutRunning(t *testing.T) {
	dir := root(t)
	path := filepath.Join(dir, "opt", "homebrew", "Cellar", "composer", "2.8.5", "bin", "composer")
	writeFile(t, path, "")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	composerHome := filepath.Join(dir, "composer")
	env := append(homeEnv(dir, filepath.Dir(path)), "HOMEBREW_PREFIX="+filepath.Join(dir, "opt", "homebrew"), "COMPOSER_HOME="+composerHome)
	r := &run.Fake{}
	got, err := Composer(t.Context(), r, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installations) != 1 || got.Installations[0].Version != "2.8.5" || got.Vars["COMPOSER_HOME"] != composerHome {
		t.Fatalf("state %+v", got)
	}
	if calls := r.Calls(); len(calls) != 0 {
		t.Errorf("ran %+v", calls)
	}
}
