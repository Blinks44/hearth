package ai

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// fixtureHome is a home with files of every agent, and none of their
// commands.
func fixtureHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("testdata", "home"))
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func byID(agents []inventory.Agent) map[string]inventory.Agent {
	m := map[string]inventory.Agent{}
	for _, a := range agents {
		m[a.ID] = a
	}
	return m
}

// Each agent gets its settings directory and those of its files the home
// has, the skills directories it shares with others included; an agent not installed is
// not found, though its files are listed.
func TestDetectFiles(t *testing.T) {
	home := fixtureHome(t)
	p := func(rel ...string) string { return filepath.Join(append([]string{home}, rel...)...) }
	term := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + t.TempDir()}
	got := Detect(t.Context(), &run.Fake{}, term, nil)
	if len(got) != len(agents) {
		t.Fatalf("%d agents, want %d", len(got), len(agents))
	}
	tests := []struct {
		id    string
		dir   string
		files []inventory.AgentFile
	}{
		{"claude", p(".claude"), []inventory.AgentFile{
			{Path: p(".claude", "settings.json"), Kind: settings}, {Path: p(".claude", "CLAUDE.md"), Kind: instructions},
			{Path: p(".claude.json"), Kind: mcp}, {Path: p(".claude", "skills"), Kind: skills},
		}},
		{"codex", p(".codex"), []inventory.AgentFile{
			{Path: p(".codex", "config.toml"), Kind: settings}, {Path: p(".codex", "AGENTS.md"), Kind: instructions},
			{Path: p(".agents", "skills"), Kind: skills},
		}},
		{"copilot", p(".copilot"), []inventory.AgentFile{
			{Path: p(".copilot", "mcp-config.json"), Kind: mcp}, {Path: p(".agents", "skills"), Kind: skills},
		}},
		{"gemini", p(".gemini"), []inventory.AgentFile{
			{Path: p(".gemini", "settings.json"), Kind: settings}, {Path: p(".gemini", "GEMINI.md"), Kind: instructions},
			{Path: p(".agents", "skills"), Kind: skills},
		}},
		{"cursor", p(".cursor"), []inventory.AgentFile{
			{Path: p(".cursor", "mcp.json"), Kind: mcp}, {Path: p(".agents", "skills"), Kind: skills},
			{Path: p(".claude", "skills"), Kind: skills},
		}},
		{"opencode", p(".config", "opencode"), []inventory.AgentFile{
			{Path: p(".config", "opencode", "opencode.json"), Kind: settings},
			{Path: p(".claude", "skills"), Kind: skills}, {Path: p(".agents", "skills"), Kind: skills},
		}},
		{"devin", p(".config", "devin"), []inventory.AgentFile{
			{Path: p(".config", "devin", "mcp_config.json"), Kind: mcp},
		}},
		{"grok", p(".grok"), []inventory.AgentFile{
			{Path: p(".grok", "config.toml"), Kind: settings}, {Path: p(".agents", "skills"), Kind: skills},
			{Path: p(".claude", "skills"), Kind: skills},
		}},
		{"kimi", p(".kimi-code"), []inventory.AgentFile{
			{Path: p(".kimi-code", "mcp.json"), Kind: mcp}, {Path: p(".agents", "skills"), Kind: skills},
		}},
		// Aider keeps no directory of its own.
		{"aider", "", []inventory.AgentFile{{Path: p(".aider.conf.yml"), Kind: settings}}},
	}
	found := byID(got)
	for _, tt := range tests {
		a := found[tt.id]
		if a.Dir != tt.dir || !reflect.DeepEqual(a.Files, tt.files) {
			t.Errorf("%s: dir %q, files %+v\nwant dir %q, files %+v", tt.id, a.Dir, a.Files, tt.dir, tt.files)
		}
		if tt.id != "cursor" && a.Found() {
			t.Errorf("%s found without its command: %+v", tt.id, a)
		}
	}
}

// A settings directory set in the environment wins over the default.
func TestDetectDirFromEnv(t *testing.T) {
	home := fixtureHome(t)
	alt := filepath.Join(home, "alt-codex")
	term := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "CODEX_HOME=" + alt}
	codex := byID(Detect(t.Context(), &run.Fake{}, term, nil))["codex"]
	want := []inventory.AgentFile{
		{Path: filepath.Join(alt, "config.toml"), Kind: settings},
		{Path: filepath.Join(home, ".agents", "skills"), Kind: skills},
	}
	if codex.Dir != alt || !reflect.DeepEqual(codex.Files, want) {
		t.Errorf("dir %q, files %+v", codex.Dir, codex.Files)
	}
}

// Without a home, nothing in it is looked for.
func TestDetectNoHome(t *testing.T) {
	for _, a := range Detect(t.Context(), &run.Fake{}, nil, nil) {
		if a.Dir != "" || len(a.Files) > 0 || a.Found() {
			t.Errorf("%s: %+v", a.ID, a)
		}
	}
}

func TestVersions(t *testing.T) {
	tests := []struct{ in, path, out string }{
		{path: "/Users/dev/.local/share/claude/versions/1.0.83", out: "1.0.83"},
		{path: "/Users/dev/.local/share/cursor-agent/versions/2025.09.04-fc40cd1/cursor-agent", out: "2025.09.04"},
		{path: "/opt/homebrew/bin/codex", out: ""},
		{in: "1.0.83 (Claude Code)\n", out: "1.0.83"},
		{in: "codex-cli 0.29.0\n", out: "0.29.0"},
		{in: "0.4.1\n", out: "0.4.1"},
	}
	for _, tt := range tests {
		got := ""
		if tt.path != "" {
			got = pathVersion(filepath.FromSlash(tt.path))
		} else {
			got = version.FindString(tt.in)
		}
		if got != tt.out {
			t.Errorf("%q%q: %q, want %q", tt.path, tt.in, got, tt.out)
		}
	}
}

// An agent's first start may be slow: its version gets more than the
// default timeout.
func TestRunVersionTimeout(t *testing.T) {
	r := &run.Fake{}
	r.Stdout("grok 1.0.77 (50ca579fa996)", "/bin/grok", "--version")
	if v := runVersion(t.Context(), r, nil, "/bin/grok"); v != "1.0.77" || r.Calls()[0].Timeout < 10*time.Second {
		t.Fatal(v, r.Calls())
	}
}
