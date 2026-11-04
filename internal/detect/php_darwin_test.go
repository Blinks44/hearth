package detect

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestPHPBrewAndHerd(t *testing.T) {
	dir := root(t)
	brew := filepath.Join(dir, "brew")
	home := filepath.Join(dir, "home")
	php := filepath.Join(brew, "Cellar", "php@8.2", "8.2.27_1", "bin", "php")
	herdBin := filepath.Join(home, "Library", "Application Support", "Herd", "bin")
	herd := filepath.Join(herdBin, "php83")
	// Herd's versioned PHPs may link into the app; its php wrapper picks a
	// version itself.
	herdReal := filepath.Join(dir, "Herd.app", "php83")
	wrapper := filepath.Join(herdBin, "php")
	for _, path := range []string{php, herdReal, wrapper} {
		writeFile(t, path, "")
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(herdReal, herd); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(filepath.Dir(herd), "php.ini"), "ignored")
	env := append(homeEnv(dir), "HOMEBREW_PREFIX="+brew)
	r := &run.Fake{}
	r.Stdout("PHP Warning:  Unable to load dynamic library 'redis.so' (tried: /opt/homebrew/Cellar/php/8.4.3/lib/redis.so)\nPHP 8.3.16 (cli)", herdReal, "-v")
	got, err := PHP(t.Context(), r, env, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	foundBrew, foundHerd := false, false
	for _, in := range got.Installations {
		if in.RealPath == php {
			foundBrew = in.Version == "8.2.27" && in.Source.Manager == "brew" && in.OnPath == 0
		}
		if in.RealPath == herdReal {
			foundHerd = in.Version == "8.3.16" && in.Source.Manager == "Herd"
		}
	}
	if !foundBrew || !foundHerd {
		t.Fatalf("state %+v", got)
	}
	for _, call := range r.Calls() {
		if base := filepath.Base(call.Args[0]); base == "php.ini" || call.Args[0] == wrapper {
			t.Fatalf("ran %s", base)
		}
		// No copy is on PATH, so no configuration is read.
		if slices.Contains(call.Args, "--ini") {
			t.Fatalf("read configuration: %v", call.Args)
		}
	}
}
