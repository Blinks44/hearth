package ops

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

// One plan stays as it is; several run in turn, with a note each says
// once, and their checks all pass or the first that does not fails.
func TestUpdatePlan(t *testing.T) {
	if _, ok := UpdatePlan(nil); ok {
		t.Error("a plan of none")
	}
	brew := Plan{Title: "Update git", Steps: []Step{{Argv: []string{"brew", "upgrade", "git"}}}, Changes: []string{"git"},
		Verify: func(context.Context) (string, error) { return "git updated", nil }}
	if p, _ := UpdatePlan([]Plan{brew}); p.Title != "Update git" {
		t.Errorf("one plan became %q", p.Title)
	}
	admin := Note{Title: "Administrator"}
	apt := Plan{Title: "Update curl", Steps: []Step{{Argv: []string{"apt-get", "install", "curl"}, Admin: true}}, Changes: []string{"curl"},
		Notes: []Note{admin}, Verify: func(context.Context) (string, error) { return "curl updated", nil }}
	snap := Plan{Title: "Update code", Steps: []Step{{Argv: []string{"snap", "refresh", "code"}, Admin: true}}, Notes: []Note{admin}}
	p, ok := UpdatePlan([]Plan{brew, apt, snap})
	if !ok || p.Title != "Update the chosen · 2" || len(p.Steps) != 3 || len(p.Changes) != 2 || len(p.Notes) != 1 {
		t.Fatalf("plan %+v", p)
	}
	if said, err := p.Verify(context.Background()); err != nil || said != "git updated; curl updated" {
		t.Errorf("check %q, %v", said, err)
	}
	apt.Verify = func(context.Context) (string, error) { return "", errors.New("curl not updated") }
	p, _ = UpdatePlan([]Plan{brew, apt})
	if _, err := p.Verify(context.Background()); err == nil || err.Error() != "curl not updated" {
		t.Errorf("check %v", err)
	}
}

func TestBulkFixPlan(t *testing.T) {
	if _, ok := BulkFixPlan(nil); ok {
		t.Fatal("empty BulkFixPlan returned ok")
	}
	var order []string
	p1 := Plan{
		Title:           "Fix PATH",
		Steps:           []Step{{Argv: []string{"echo", "path"}}},
		Changes:         []string{"PATH cleaned"},
		RestartTerminal: true,
		Notes:           []Note{{Title: "Backup"}},
		Before:          func(context.Context) error { order = append(order, "b1"); return nil },
		Verify:          func(context.Context) (string, error) { return "path ok", nil },
	}
	p2 := Plan{
		Title:   "Merge skills",
		Steps:   []Step{{Argv: []string{"echo", "s1"}}, {Argv: []string{"echo", "s2"}}},
		Changes: []string{"Skills merged"},
		Notes:   []Note{{Title: "Backup"}, {Title: "Links"}},
		Before:  func(context.Context) error { order = append(order, "b2"); return nil },
		Verify:  func(context.Context) (string, error) { return "skills ok", nil },
	}
	got, ok := BulkFixPlan([]Plan{p1, p2})
	if !ok {
		t.Fatal("BulkFixPlan returned !ok")
	}
	if !got.RestartTerminal || len(got.Steps) != 3 || !slices.Equal(got.Parts, []int{0, 1}) || len(got.Notes) != 2 {
		t.Fatalf("unexpected BulkFixPlan: %+v", got)
	}
	if err := got.Before(context.Background()); err != nil || !slices.Equal(order, []string{"b1", "b2"}) {
		t.Fatalf("Before order=%v err=%v", order, err)
	}
	if said, err := got.Verify(context.Background()); err != nil || said != "path ok; skills ok" {
		t.Fatalf("Verify said=%q err=%v", said, err)
	}
}

// A fix replaces the copy through its version manager: the new version
// goes in, becomes the default when the old one was, and the old one goes;
// rustup and mise's own tools update in place.
func TestVersionUpdatePlanCommands(t *testing.T) {
	shell := func(script, name string, args ...string) []string {
		return append([]string{"bash", "-c", script, name}, args...)
	}
	update := func(tool, manager, from, to string) VersionUpdate {
		return VersionUpdate{From: VersionTarget{Tool: tool, Title: tool, Manager: manager, Version: from},
			To: VersionTarget{Tool: tool, Title: tool, Manager: manager, Version: to}}
	}
	def := func(u VersionUpdate) VersionUpdate {
		u.Default = true
		return u
	}
	tests := []struct {
		name string
		u    VersionUpdate
		want [][]string // nil: Hearth cannot
	}{
		{"fnm", def(update("node", "fnm", "22.12.0", "22.23.4")), [][]string{
			{"fnm", "install", "22.23.4"}, {"fnm", "default", "22.23.4"}, {"fnm", "uninstall", "22.12.0"}}},
		{"fnm, not the default", update("node", "fnm", "20.18.0", "20.19.5"), [][]string{
			{"fnm", "install", "20.19.5"}, {"fnm", "uninstall", "20.18.0"}}},
		{"nvm", def(update("node", "nvm", "22.12.0", "22.23.4")), [][]string{
			shell(nvmScript, "nvm", "install", "22.23.4"), shell(nvmScript, "nvm", "alias", "default", "22.23.4"), shell(nvmScript, "nvm", "uninstall", "22.12.0")}},
		{"nvm-windows", def(update("node", "nvm-windows", "22.12.0", "22.23.4")), [][]string{
			{"nvm", "install", "22.23.4"}, {"nvm", "use", "22.23.4"}, {"nvm", "uninstall", "22.12.0"}}},
		{"n", def(update("node", "n", "22.12.0", "22.23.4")), [][]string{
			{"n", "download", "22.23.4"}, {"n", "22.23.4"}, {"n", "rm", "22.12.0"}}},
		{"volta keeps the old one", def(update("node", "volta", "22.12.0", "22.23.4")), [][]string{
			{"volta", "fetch", "node@22.23.4"}, {"volta", "install", "node@22.23.4"}}},
		{"mise", def(update("python", "mise", "3.13.1", "3.13.9")), [][]string{
			{"mise", "install", "python@3.13.9"}, {"mise", "use", "-g", "python@3.13.9"}, {"mise", "uninstall", "python@3.13.1"}}},
		{"asdf", update("ruby", "asdf", "3.3.6", "3.3.10"), [][]string{
			{"asdf", "install", "ruby", "3.3.10"}, {"asdf", "uninstall", "ruby", "3.3.6"}}},
		{"pyenv", def(update("python", "pyenv", "3.12.8", "3.12.12")), [][]string{
			{"pyenv", "install", "3.12.12"}, {"pyenv", "global", "3.12.12"}, {"pyenv", "uninstall", "-f", "3.12.8"}}},
		{"uv", update("python", "uv", "3.12.8", "3.12.12"), [][]string{
			{"uv", "python", "install", "3.12.12"}, {"uv", "python", "uninstall", "3.12.8"}}},
		{"rbenv", def(update("ruby", "rbenv", "3.3.6", "3.3.10")), [][]string{
			{"rbenv", "install", "3.3.10"}, {"rbenv", "global", "3.3.10"}, {"rbenv", "uninstall", "-f", "3.3.6"}}},
		{"rvm", def(update("ruby", "rvm", "ruby-3.3.6", "3.3.10")), [][]string{
			shell(rvmScript, "rvm", "install", "3.3.10"), shell(rvmScript, "rvm", "alias", "create", "default", "3.3.10"), shell(rvmScript, "rvm", "remove", "ruby-3.3.6")}},
		{"rustup", VersionUpdate{From: VersionTarget{Tool: "rust", Manager: "rustup", Version: "1.91.0"}, To: VersionTarget{Version: "1.92.0"}, Toolchain: "stable-aarch64-apple-darwin"},
			[][]string{{"rustup", "update", "stable-aarch64-apple-darwin"}}},
		{"mise's own tool", VersionUpdate{From: VersionTarget{Tool: "npm-prettier", Manager: "mise", Version: "3.5.0"}, To: VersionTarget{Version: "3.6.2"}, Mise: "npm:prettier"},
			[][]string{{"mise", "upgrade", "--prune", "npm:prettier"}}},
		{"a Java", def(update("java", "sdkman", "21.0.5-tem", "21.0.9")), nil},
		{"uv has no default", def(update("python", "uv", "3.12.8", "3.12.12")), nil},
		{"chruby does not install", update("ruby", "chruby", "3.3.6", "3.3.10"), nil},
	}
	for _, tc := range tests {
		p, ok := VersionUpdatePlan(tc.u, nil)
		if ok != (tc.want != nil) {
			t.Errorf("%s: ok = %v", tc.name, ok)
			continue
		}
		var got [][]string
		for _, s := range p.Steps {
			got = append(got, s.Argv)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: steps\n%q\nwant\n%q", tc.name, got, tc.want)
		}
	}
}

// The plan says it replaces the copy, and warns of the global packages
// that go with it.
func TestVersionUpdatePlanText(t *testing.T) {
	u := VersionUpdate{
		From:    VersionTarget{Tool: "node", Title: "Node.js", Manager: "fnm", Version: "22.12.0", Label: "v22.12.0"},
		To:      VersionTarget{Tool: "node", Title: "Node.js", Manager: "fnm", Version: "22.23.4", Label: "v22.23.4"},
		Globals: true,
	}
	p, _ := VersionUpdatePlan(u, nil)
	if p.Title != "Update Node.js v22.12.0 to v22.23.4" || !strings.Contains(p.Why, "removes the old one") ||
		!slices.Equal(p.Changes, []string{"Node.js v22.12.0 → v22.23.4 · fnm"}) {
		t.Errorf("plan %q, %q, %q", p.Title, p.Why, p.Changes)
	}
	if len(p.Notes) != 1 || !strings.Contains(p.Notes[0].Text, "installed globally into Node.js v22.12.0") {
		t.Errorf("notes %+v", p.Notes)
	}
	u.From.Manager, u.To.Manager = "volta", "volta"
	if p, _ := VersionUpdatePlan(u, nil); len(p.Notes) != 0 {
		t.Errorf("Volta keeps the old copy, with its packages: notes %+v", p.Notes)
	}
}

// The check finds the new version, the default when the old one was,
// and the old one gone; in place, the old version gone from the
// toolchain or mise's folder of the tool.
func TestVersionUpdatePlanCheck(t *testing.T) {
	copyOf := func(manager, root, v string, def bool) inventory.Installation {
		return inventory.Installation{Tool: "node", Version: v, Default: def, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: manager, Root: root, Version: v}}
	}
	found := func(ins ...inventory.Installation) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			return inventory.ToolState{Installations: ins}, nil
		}
	}
	u := VersionUpdate{
		From:    VersionTarget{Tool: "node", Title: "Node.js", Manager: "fnm", Version: "22.12.0"},
		To:      VersionTarget{Tool: "node", Title: "Node.js", Manager: "fnm", Version: "22.23.4"},
		Default: true,
	}
	type check struct {
		name  string
		u     VersionUpdate
		found Detect
		ok    bool
	}
	tests := []check{
		{"replaced", u, found(copyOf("fnm", "", "22.23.4", true)), true},
		{"not the default", u, found(copyOf("fnm", "", "22.23.4", false)), false},
		{"old one stays", u, found(copyOf("fnm", "", "22.23.4", true), copyOf("fnm", "", "22.12.0", false)), false},
		{"new one missing", u, found(copyOf("fnm", "", "22.12.0", true)), false},
	}
	rust := VersionUpdate{From: VersionTarget{Tool: "rust", Title: "Rust", Manager: "rustup", Version: "1.91.0", Root: "/r/toolchains/stable"}, Toolchain: "stable"}
	mise := VersionUpdate{From: VersionTarget{Tool: "jq", Title: "jq", Manager: "mise", Version: "1.7.1", Root: "/m/installs/jq/1.7.1"}, Mise: "jq"}
	miseTool := func(ins ...inventory.Installation) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			return inventory.ToolState{MiseTools: []inventory.MiseTool{{Name: "jq", Installations: ins}}}, nil
		}
	}
	tests = append(tests,
		check{"rustup updated", rust, found(copyOf("rustup", "/r/toolchains/stable", "1.92.0", true)), true},
		check{"rustup did not", rust, found(copyOf("rustup", "/r/toolchains/stable", "1.91.0", true)), false},
		check{"mise upgraded", mise, miseTool(copyOf("mise", "/m/installs/jq/1.8.1", "1.8.1", false)), true},
		check{"mise kept the old", mise, miseTool(copyOf("mise", "/m/installs/jq/1.8.1", "1.8.1", false), copyOf("mise", "/m/installs/jq/1.7.1", "1.7.1", false)), false},
	)
	for _, tc := range tests {
		p, _ := VersionUpdatePlan(tc.u, tc.found)
		said, err := p.Verify(context.Background())
		if (err == nil) != tc.ok {
			t.Errorf("%s: check %q, %v", tc.name, said, err)
		}
	}
}
