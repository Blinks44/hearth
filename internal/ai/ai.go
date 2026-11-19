// Package ai finds the AI coding agents on the computer: their commands
// and desktop apps, versions, and the user's settings, instructions, MCP
// and skills files of each; and the skills in the user's and projects'
// skills directories. It only reads.
package ai

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// agent is what Hearth knows of an agent: its commands, where it keeps
// the user's settings, and which files there are its own.
type agent struct {
	id string
	// commands are the names it puts on PATH, the main one first.
	commands []string
	// dir is its settings directory: the first of these set, "~/" for
	// the home and "$NAME/" for an environment variable's directory.
	dir []string
	// files are relative to dir, or to the home with "~/".
	files []file
	// skills are the skills directories it reads of the user's, as files
	// are written, and projectSkills those of a project, relative to it.
	skills, projectSkills []string
}

type file struct {
	path string
	kind inventory.FileKind
}

const (
	settings     = inventory.FileSettings
	instructions = inventory.FileInstructions
	mcp          = inventory.FileMCP
	skills       = inventory.FileSkills
)

// sharedSkills is the skills directory several agents read.
const sharedSkills = "~/.agents/skills"

// agents are the ones Hearth finds, in the order shown, by detection.md §7.
var agents = []agent{
	{"claude", []string{"claude"}, []string{"$CLAUDE_CONFIG_DIR", "~/.claude"}, []file{
		{"settings.json", settings}, {"CLAUDE.md", instructions}, {"~/.claude.json", mcp},
	}, []string{"skills"}, []string{".claude/skills"}},
	{"codex", []string{"codex"}, []string{"$CODEX_HOME", "~/.codex"}, []file{
		{"config.toml", settings}, {"AGENTS.md", instructions}, {"AGENTS.override.md", instructions},
	}, []string{sharedSkills}, []string{".agents/skills"}},
	{"copilot", []string{"copilot"}, []string{"$COPILOT_HOME", "~/.copilot"}, []file{
		{"settings.json", settings}, {"copilot-instructions.md", instructions}, {"mcp-config.json", mcp},
	}, []string{"skills", sharedSkills}, []string{".github/skills", ".claude/skills", ".agents/skills"}},
	{"antigravity", []string{"agy"}, []string{"~/.gemini/antigravity-cli"}, []file{
		{"settings.json", settings}, {"~/.gemini/config/hooks.json", settings}, {"~/.gemini/config/mcp_config.json", mcp},
		{"~/.gemini/AGENTS.md", instructions}, {"~/.gemini/GEMINI.md", instructions},
		{"~/.gemini/config/AGENTS.md", instructions}, {"~/.gemini/config/GEMINI.md", instructions},
	}, []string{"~/.gemini/config/skills", "skills", "~/.gemini/skills"},
		[]string{".agents/skills", ".agent/skills", "_agents/skills", "_agent/skills"}},
	{"gemini", []string{"gemini"}, []string{"~/.gemini"}, []file{
		{"settings.json", settings}, {"GEMINI.md", instructions},
	}, []string{"skills", sharedSkills}, []string{".gemini/skills", ".agents/skills"}},
	// Cursor also reads Claude Code's and Codex's skills directories.
	{"cursor", []string{"cursor-agent"}, []string{"~/.cursor"}, []file{
		{"cli-config.json", settings}, {"mcp.json", mcp},
	}, []string{"skills", sharedSkills, "~/.claude/skills", "~/.codex/skills"},
		[]string{".cursor/skills", ".agents/skills", ".claude/skills", ".codex/skills"}},
	{"opencode", []string{"opencode"}, []string{"$OPENCODE_CONFIG_DIR", "$XDG_CONFIG_HOME/opencode", "~/.config/opencode"}, []file{
		{"opencode.json", settings}, {"AGENTS.md", instructions},
	}, []string{"skills", "~/.claude/skills", sharedSkills}, []string{".opencode/skills", ".claude/skills", ".agents/skills"}},
	{"devin", []string{"devin"}, []string{"$APPDATA/devin", "$XDG_CONFIG_HOME/devin", "~/.config/devin"}, []file{
		{"mcp_config.json", mcp}, {"AGENTS.md", instructions},
	}, []string{"skills"}, []string{".devin/skills", ".agents/skills", ".windsurf/skills"}},
	// Grok Build also reads Claude Code's and Cursor's skills directories.
	{"grok", []string{"grok"}, []string{"$GROK_HOME", "~/.grok"}, []file{
		{"config.toml", settings},
	}, []string{"skills", sharedSkills, "~/.claude/skills", "~/.cursor/skills"},
		[]string{".grok/skills", ".agents/skills", ".claude/skills", ".cursor/skills"}},
	// Kimi Code, the successor of the archived Kimi CLI and its ~/.kimi.
	{"kimi", []string{"kimi"}, []string{"$KIMI_CODE_HOME", "~/.kimi-code"}, []file{
		{"config.toml", settings}, {"AGENTS.md", instructions}, {"mcp.json", mcp},
	}, []string{"skills", sharedSkills}, []string{".kimi-code/skills", ".agents/skills"}},
	// Aider has neither MCP nor skills: only its settings in the home.
	{"aider", []string{"aider"}, nil, []file{
		{"~/.aider.conf.yml", settings},
	}, nil, nil},
}

// Detect finds every agent Hearth knows, on the terminal's PATH (term)
// and the GUI's (gui), with its desktop app and the user's files. The
// list has every agent, found or not, in Hearth's order.
func Detect(ctx context.Context, r run.Runner, term, gui sysenv.Env) []inventory.Agent {
	home := homeDir(term)
	out := make([]inventory.Agent, len(agents))
	var wg sync.WaitGroup
	for i, a := range agents {
		// Each may run a command for its version; they run side by side.
		wg.Go(func() { out[i] = detect(ctx, r, term, gui, home, a) })
	}
	wg.Wait()
	return out
}

func detect(ctx context.Context, r run.Runner, term, gui sysenv.Env, home string, a agent) inventory.Agent {
	found := inventory.Agent{ID: a.id, Installations: copies(term, gui, a.commands)}
	versionEnv := term
	if a.id == "antigravity" {
		versionEnv = append(append(sysenv.Env(nil), term...), "AGY_CLI_DISABLE_AUTO_UPDATE=true")
	}
	for i := range found.Installations {
		in := &found.Installations[i]
		in.Tool = a.id
		// A shim leads to its manager, as mise: the shim itself runs the
		// agent.
		if in.Source.Shim {
			in.Version = runVersion(ctx, r, versionEnv, in.Path)
		} else if in.Version = pathVersion(in.RealPath); in.Version == "" {
			in.Version = runVersion(ctx, r, versionEnv, in.RealPath)
		}
	}
	if a.id == "cursor" {
		found.App, found.AppVersion = cursorApp(term, home)
	}
	found.Dir = dir(a.dir, term, home)
	if a.id == "codex" {
		instructions := CodexInstructions(term)
		found.Instructions = &instructions
	}
	add := func(path string, kind inventory.FileKind) {
		p := resolve(path, found.Dir, home)
		if _, err := os.Stat(p); p != "" && err == nil {
			found.Files = append(found.Files, inventory.AgentFile{Path: p, Kind: kind})
		}
	}
	for _, f := range a.files {
		add(f.path, f.kind)
	}
	for _, s := range a.skills {
		add(s, skills)
	}
	// Claude Code sets the "env" of its settings in its own environment,
	// API keys among them.
	if a.id == "claude" && found.Dir != "" {
		found.Settings = filepath.Join(found.Dir, "settings.json")
		// A file that does not read shows no secrets: the agent page says
		// what is in it.
		found.PlainEnv, _ = SettingsSecrets(found.Settings)
	}
	// As with the secrets, a file that does not read shows nothing: the
	// agent page lists the files.
	found.Rules, found.Hooks, _ = access(a.id, found.Dir, home, accessFiles[a.id].user, "")
	return found
}

// homeDir is the home of env: HOME, or USERPROFILE on Windows.
func homeDir(env sysenv.Env) string {
	if home := env.Get("HOME"); home != "" {
		return home
	}
	return env.Get("USERPROFILE")
}

// resolve is the path of a file as the agents table writes it: in the
// home for "~/", else in the agent's directory dir; "" when that is not
// known.
func resolve(path, dir, home string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home == "" {
			return ""
		}
		return filepath.Join(home, filepath.FromSlash(rest))
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, filepath.FromSlash(path))
}

// copies are the agent's commands on both PATHs, a copy two paths lead
// to once, in the terminal's order, then the GUI's.
func copies(term, gui sysenv.Env, commands []string) []inventory.Installation {
	var out []inventory.Installation
	byReal := map[string]int{}
	add := func(path string) *inventory.Installation {
		real, err := sysenv.RealPath(path)
		if err != nil {
			real = path
		}
		if i, ok := byReal[real]; ok {
			return &out[i]
		}
		byReal[real] = len(out)
		out = append(out, inventory.Installation{Path: path, RealPath: real, Source: sources.Classify(path, real, term)})
		return &out[len(out)-1]
	}
	for _, name := range commands {
		for i, p := range sysenv.LookPathAll(name, term) {
			if in := add(p); in.OnPath == 0 {
				in.OnPath = inventory.PathRank(i + 1)
			}
		}
		for i, p := range sysenv.LookPathAll(name, gui) {
			if in := add(p); in.InGUI == 0 {
				in.InGUI = inventory.PathRank(i + 1)
			}
		}
	}
	return out
}

// dir is the first of dirs that is set: an environment variable's, or
// one in the home; "" when none is.
func dir(dirs []string, env sysenv.Env, home string) string {
	for _, d := range dirs {
		switch {
		case strings.HasPrefix(d, "$"):
			name, rest, _ := strings.Cut(d[1:], "/")
			if v := env.Get(name); v != "" {
				return filepath.Join(v, filepath.FromSlash(rest))
			}
		case strings.HasPrefix(d, "~/") && home != "":
			return filepath.Join(home, filepath.FromSlash(d[2:]))
		}
	}
	return ""
}

// version finds a version in a command's output or a path's name, as
// "1.0.83" in "1.0.83 (Claude Code)" or "codex-cli 0.29.0".
var version = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// versionName is a file or directory named by a version, as the native
// installs of Claude Code and Cursor's agent keep them.
var versionName = regexp.MustCompile(`^v?\d+\.\d+\.\d+`)

// pathVersion is the version real's name or its directory's tells, as
// ~/.local/share/claude/versions/1.0.83; "" when neither does.
func pathVersion(real string) string {
	for _, name := range []string{filepath.Base(real), filepath.Base(filepath.Dir(real))} {
		if versionName.MatchString(name) {
			return version.FindString(name)
		}
	}
	return ""
}

// runVersion asks the command at path for its version; "" when it does
// not tell.
func runVersion(ctx context.Context, r run.Runner, env sysenv.Env, path string) string {
	// Agents are big programs: Kimi Code's first start on Windows on Arm
	// takes some 10 s. They are asked side by side.
	res, err := r.Run(ctx, run.Cmd{Args: []string{path, "--version"}, Env: env, Timeout: 20 * time.Second})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return version.FindString(string(res.Stdout))
}
