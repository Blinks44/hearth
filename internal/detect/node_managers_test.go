package detect

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestNodeManagers(t *testing.T) {
	src := func(kind inventory.Kind, manager string) inventory.Installation {
		return inventory.Installation{Source: inventory.Source{Kind: kind, Manager: manager}}
	}
	ins := []inventory.Installation{
		src(inventory.KindVersionManager, "fnm"), src(inventory.KindPackageManager, "brew"),
		src(inventory.KindVersionManager, "fnm"), src(inventory.KindVersionManager, "nvm"),
		src(inventory.KindVersionManager, "volta"), src(inventory.KindVersionManager, "n"),
	}
	tests := []struct {
		name string
		rc   string // the startup file's text
		fnm  bool
		nvm  bool
	}{
		{"no hooks", "export PATH=$HOME/bin:$PATH\n", false, false},
		{"fnm hook", `eval "$(fnm env --use-on-cd --shell zsh)"` + "\n", true, false},
		{"hook commented out", `# eval "$(fnm env --use-on-cd)"` + "\n", false, false},
		{"nvm's snippet", "autoload -U add-zsh-hook\nload-nvmrc() {\n  nvm use\n}\nadd-zsh-hook chpwd load-nvmrc\n", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			writeFile(t, startupFile(home), tt.rc)
			want := []inventory.Manager{
				{Name: "fnm", SwitchOnCD: tt.fnm},
				{Name: "nvm", SwitchOnCD: tt.nvm},
				{Name: "volta", SwitchOnCD: true, Files: []string{"package.json"}},
				{Name: "n"},
			}
			if tt.fnm {
				want[0].Files = []string{".node-version", ".nvmrc"}
			}
			if tt.nvm {
				want[1].Files = []string{".nvmrc"}
			}
			if got := nodeManagers(ins, sysenv.Env{}, home); !reflect.DeepEqual(got, want) {
				t.Errorf("got  %+v\nwant %+v", got, want)
			}
		})
	}
	if got := nodeManagers([]inventory.Installation{src(inventory.KindPackageManager, "brew")}, sysenv.Env{}, ""); got != nil {
		t.Errorf("brew only: %+v", got)
	}
}

// corepack enable puts scripts that run Corepack beside node.
func TestCorepackEnabled(t *testing.T) {
	dir := t.TempDir()
	node := filepath.Join(dir, "bin", "node")
	writeFile(t, node, "")
	if corepackEnabled(filepath.Dir(node)) {
		t.Error("enabled without its shims")
	}
	writeFile(t, filepath.Join(dir, "bin", corepackShims[0]), "#!/bin/sh\nnode \"$basedir/node_modules/corepack/dist/pnpm.js\" \"$@\"\n")
	if !corepackEnabled(filepath.Dir(node)) {
		t.Error("not enabled with its shims")
	}
}

func TestCommands(t *testing.T) {
	tests := []struct {
		name string
		bin  any
		want []string
	}{
		{"typescript", map[string]any{"tsserver": "./bin/tsserver", "tsc": "./bin/tsc"}, []string{"tsc", "tsserver"}},
		{"@anthropic-ai/claude-code", "cli.js", []string{"claude-code"}},
		{"left-pad", nil, nil},
	}
	for _, tt := range tests {
		if got := commands(tt.name, tt.bin); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("commands(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}
