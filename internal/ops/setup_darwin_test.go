package ops

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// On a new Mac, Homebrew's package comes first, mise through it, then
// what runs mise or npm through the mise it set up: none is on the
// terminal's PATH yet.
func TestSetupPlanMac(t *testing.T) {
	root := t.TempDir()
	defer func(p []string) { brewPrefixes = p }(brewPrefixes)
	brewPrefixes = []string{filepath.Join(root, "arm"), filepath.Join(root, "intel")}
	env := sysenv.Env{"HOME=" + root, "SHELL=/bin/zsh", "PATH=" + filepath.Join(root, "bin")}
	s := Setup{GOOS: "darwin", Tools: []string{"homebrew", "mise", "node", "pnpm", "git", "codex", "grok", "claude"}, Title: strings.ToUpper}

	p, parts, err := SetupPlan(s, env)
	if err != nil {
		t.Fatal(err)
	}
	part := func(id string) []Step {
		for _, pt := range parts {
			if pt.Tool == id {
				return p.Steps[pt.From:pt.To]
			}
		}
		t.Fatalf("no part %s in %+v", id, parts)
		return nil
	}
	for i := 1; i < len(parts); i++ {
		if parts[i].From != parts[i-1].To {
			t.Errorf("parts %+v do not follow each other", parts)
		}
	}
	if hb := part("homebrew"); len(hb) != 3 || !hb[2].Admin {
		t.Errorf("Homebrew: %+v", hb)
	}
	brew := part("mise")[0].Argv[0]
	if !strings.HasSuffix(brew, "/bin/brew") || part("mise")[0].Argv[1] != "install" {
		t.Errorf("mise: %+v", part("mise"))
	}
	mise := filepath.Join(filepath.Dir(brew), "mise")
	if got := part("node")[0].Argv; !slices.Equal(got, []string{mise, "use", "--global", "node@lts"}) {
		t.Errorf("Node: %v", got)
	}
	if got := part("codex")[0].Argv; !slices.Equal(got, []string{brew, "install", "--cask", "codex"}) {
		t.Errorf("Codex: %v", got)
	}
	if got := part("grok")[0].Argv; !slices.Equal(got, []string{mise, "exec", "--", "npm", "install", "--global", "@xai-official/grok"}) {
		t.Errorf("Grok through the Node mise installs: %v", got)
	}
	// npm of mise's Node runs mise reshim, which needs mise on PATH.
	if env := part("grok")[0].Env; len(env) != 1 || !strings.HasPrefix(env[0], "PATH="+filepath.Dir(mise)+":") {
		t.Errorf("Grok's PATH: %v", env)
	}
	if git := part("git"); len(git) != 0 {
		t.Errorf("Homebrew's part installs the Command Line Tools: %+v", git)
	}
	if hb := part("homebrew")[2]; !strings.Contains(hb.Argv[2], "softwareupdate -i") || !strings.Contains(hb.Argv[2], "installer -pkg") {
		t.Errorf("Homebrew's install: %+v", hb)
	}
	// mise's hook and Claude Code's PATH share .zshrc: one block, edited
	// in turn.
	var zshrc []*FileEdit
	for _, step := range p.Steps {
		if step.Edit != nil && filepath.Base(step.Edit.Path) == ".zshrc" {
			zshrc = append(zshrc, step.Edit)
		}
	}
	if len(zshrc) != 2 || zshrc[1].Before != zshrc[0].After ||
		!strings.Contains(zshrc[1].After, "mise activate zsh") || !strings.Contains(zshrc[1].After, ".local/bin") {
		t.Errorf(".zshrc edits: %+v", zshrc)
	}
	homebrewNotes := 0
	for _, n := range p.Notes {
		if n.Tag == "Homebrew" {
			homebrewNotes++
		}
	}
	if homebrewNotes != 1 {
		t.Errorf("notes %+v", p.Notes)
	}
	// With Homebrew there, Git brings the Command Line Tools on its own,
	// without Apple's window.
	s.Have = map[string]bool{"homebrew": true}
	s.Tools = []string{"git"}
	p, _, err = SetupPlan(s, env)
	if err != nil || len(p.Steps) != 1 || !p.Steps[0].Admin || !strings.Contains(p.Steps[0].Argv[2], "softwareupdate") {
		t.Errorf("Git alone: %+v, %v", p.Steps, err)
	}
}
