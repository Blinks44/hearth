//go:build !windows

package detect

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestRubyDiscovery(t *testing.T) {
	dir := root(t)
	rbenv := filepath.Join(dir, ".rbenv")
	shim := fakeExe(t, filepath.Join(rbenv, "shims", "ruby"))
	first := fakeExe(t, filepath.Join(rbenv, "versions", "3.3.6", "bin", "ruby"))
	fakeExe(t, filepath.Join(rbenv, "versions", "3.2.6", "bin", "ruby"))
	writeFile(t, filepath.Join(rbenv, "version"), "3.3.6\n")
	for _, name := range []string{"rubocop-1.70.0.gemspec", "bundler-2.6.2.gemspec"} {
		writeFile(t, filepath.Join(rbenv, "versions", "3.3.6", "lib", "ruby", "gems", "3.3.0", "specifications", name), "raise 'must not execute'")
	}
	rvm := filepath.Join(dir, "rvm")
	fakeExe(t, filepath.Join(rvm, "rubies", "ruby-3.1.6", "bin", "ruby"))
	fakeExe(t, filepath.Join(dir, ".rubies", "ruby-3.0.7", "bin", "ruby"))
	env := sysenv.Env{"HOME=" + dir, "PATH=" + filepath.Dir(shim), "RBENV_ROOT=" + rbenv, "rvm_path=" + rvm}
	r := &run.Fake{}
	st, err := Ruby(t.Context(), r, env, sysenv.Env{"PATH=" + filepath.Dir(first)})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installations) != 5 {
		t.Fatalf("installations: %+v", st.Installations)
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 {
		t.Fatalf("no active: %+v", st)
	}
	in := st.Installations[active]
	if in.Version != "3.3.6" || in.InGUI != 1 || len(in.Packages) != 2 {
		t.Fatalf("active: %+v", in)
	}
	if len(r.Calls()) != 0 {
		t.Fatalf("executed command: %+v", r.Calls())
	}
	if len(st.Managers) != 3 || st.Managers[0].Name != "rbenv" || !st.Managers[0].Shims {
		t.Fatalf("managers: %+v", st.Managers)
	}
}

func TestRubyProbe(t *testing.T) {
	dir := root(t)
	path := fakeExe(t, filepath.Join(dir, "bin", "ruby"))
	r := &run.Fake{}
	r.Stdout("ruby 3.3.6 (2024-11-05 revision abc) [arm64-darwin]\n", path, "--version")
	env := sysenv.Env{"HOME=" + dir, "PATH=" + filepath.Dir(path), "RUBYOPT=-rmalicious", "RUBYLIB=/unsafe"}
	st, err := Ruby(t.Context(), r, env, nil)
	if err != nil || len(st.Installations) != 1 || st.Installations[0].Version != "3.3.6" {
		t.Fatalf("%+v %v", st, err)
	}
	for _, call := range r.Calls() {
		for _, kv := range call.Env {
			if strings.HasPrefix(kv, "RUBYOPT=") || strings.HasPrefix(kv, "RUBYLIB=") {
				t.Fatalf("unsafe env: %s", kv)
			}
		}
	}
}

func TestRubyOverride(t *testing.T) {
	dir := root(t)
	rbenv := filepath.Join(dir, ".rbenv")
	shim := fakeExe(t, filepath.Join(rbenv, "shims", "ruby"))
	for _, v := range []string{"3.2.6", "3.3.6"} {
		fakeExe(t, filepath.Join(rbenv, "versions", v, "bin", "ruby"))
	}
	writeFile(t, filepath.Join(rbenv, "version"), "3.3.6\n")
	env := sysenv.Env{"HOME=" + dir, "PATH=" + filepath.Dir(shim), "RBENV_VERSION=3.2.6"}
	st, err := Ruby(t.Context(), &run.Fake{}, env, nil)
	active := inventory.TerminalCopy(st.Installations)
	if err != nil || active < 0 || st.Installations[active].Version != "3.2.6" || st.Managers[0].SwitchOnCD {
		t.Fatalf("%+v %v", st, err)
	}
}
