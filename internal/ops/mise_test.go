package ops

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// A plan that runs mise runs the mise the setup installs, after it.
func TestWithMise(t *testing.T) {
	p := Plan{Why: "Through mise.", Steps: []Step{{Argv: []string{"mise", "install", "node@22.12.0"}}}}
	s := MiseSetup{Steps: []Step{{Argv: []string{"brew", "install", "mise"}}}, Notes: []Note{{Title: "Homebrew first"}}, Mise: "/opt/homebrew/bin/mise"}
	got := WithMise(p, s)
	if len(got.Steps) != 2 || got.Steps[1].Argv[0] != "/opt/homebrew/bin/mise" || got.Steps[0].Argv[0] != "brew" || len(got.Notes) != 1 || got.Why == p.Why {
		t.Errorf("WithMise() = %+v", got)
	}
	if p.Steps[0].Argv[0] != "mise" {
		t.Error("WithMise changed the plan it was given")
	}
	ready := WithMise(p, MiseSetup{Mise: "/usr/bin/mise"})
	if ready.Why != p.Why || ready.Steps[0].Argv[0] != "/usr/bin/mise" {
		t.Errorf("mise set up: %+v", ready)
	}
}

func TestCheckMise(t *testing.T) {
	data := t.TempDir()
	for _, tt := range []struct {
		env sysenv.Env
		ok  bool
	}{
		{sysenv.Env{"MISE_SHELL=zsh"}, true},
		{sysenv.Env{"MISE_DATA_DIR=" + data, "PATH=" + filepath.Join(data, "shims") + string(filepath.ListSeparator) + "/usr/bin"}, true},
		{sysenv.Env{"MISE_DATA_DIR=" + data, "PATH=/usr/bin"}, false},
	} {
		if _, err := checkMise(tt.env); (err == nil) != tt.ok {
			t.Errorf("checkMise(%q) = %v", tt.env, err)
		}
	}
}
