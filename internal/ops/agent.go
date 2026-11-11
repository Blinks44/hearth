package ops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// FindAgent finds an agent again, as a scan would: an agent plan's check
// uses it.
type FindAgent func(ctx context.Context) (inventory.Agent, error)

// agentPackage is a package an agent comes in: its manager, as
// inventory.PackageManager's ID or "npm", its name and Homebrew's kind.
type agentPackage struct{ manager, name, kind string }

// agentPackages are the packages of each agent its makers publish, in
// the order Hearth prefers: the system's manager first, as for any tool
// that is not a language, then npm. Agents not here are not installed by
// Hearth.
var agentPackages = map[string][]agentPackage{
	"claude":   {{"brew", "claude-code", "cask"}, {"winget", "Anthropic.ClaudeCode", ""}, {"npm", "@anthropic-ai/claude-code", ""}},
	"codex":    {{"brew", "codex", "cask"}, {"npm", "@openai/codex", ""}},
	"copilot":  {{"brew", "copilot-cli", "cask"}, {"winget", "GitHub.Copilot", ""}, {"npm", "@github/copilot", ""}},
	"gemini":   {{"brew", "gemini-cli", "formula"}, {"npm", "@google/gemini-cli", ""}},
	"cursor":   {{"brew", "cursor", "cask"}, {"winget", "Anysphere.Cursor", ""}},
	"opencode": {{"brew", "opencode", "formula"}, {"npm", "opencode-ai", ""}},
	"grok":     {{"winget", "xAI.GrokBuild", ""}, {"npm", "@xai-official/grok", ""}},
	"kimi":     {{"winget", "MoonshotAI.KimiCodeCLI", ""}},
}

// AgentInstallPlan returns the plan that installs the agent id, named
// title, through the first of its packages a manager here has: managers
// are the system's package managers found, npm reports npm on the
// terminal's PATH. ok is false when none has it.
func AgentInstallPlan(id, title string, managers []inventory.PackageManager, npm bool, find FindAgent) (p Plan, ok bool) {
	for _, pkg := range agentPackages[id] {
		if pkg.manager == "npm" {
			if !npm {
				continue
			}
			p = Plan{
				Title:   locales.PkgInstallTitle(title),
				Why:     locales.AgentNpmInstallWhy(),
				Steps:   []Step{{Argv: []string{"npm", "install", "--global", pkg.name}}},
				Changes: []string{"+ " + title + " · npm"},
			}
			ok = true
		}
		for _, m := range managers {
			if ok || m.ID != pkg.manager {
				continue
			}
			p, ok = PackagePlan(PackageInstall, m, []inventory.SystemPackage{{Name: pkg.name, Kind: pkg.kind}}, nil)
			p.Title = locales.PkgInstallTitle(title)
		}
		if ok {
			p.Verify = agentFound(title, find)
			return p, true
		}
	}
	return Plan{}, false
}

// AgentNpmUpdatePlan returns the plan that updates in, a copy of the
// agent id npm installed, to the latest release in the npm prefix that
// holds it, which may be another Node's than the terminal's npm; ok is
// false for a copy npm did not install.
func AgentNpmUpdatePlan(id, title string, in inventory.Installation, find FindAgent) (Plan, bool) {
	name := ""
	for _, pkg := range agentPackages[id] {
		if pkg.manager == "npm" {
			name = pkg.name
		}
	}
	// npm links the command to its package in node_modules, or on Windows
	// puts a shim in its prefix, which the scan tells. A system package,
	// as Homebrew's formula, may hold the npm package inside: its manager
	// updates it.
	if name == "" || in.Source.Shim || PackageManaged(in.Source.Manager) {
		return Plan{}, false
	}
	// The prefix is where npm keeps it: lib/node_modules under it on macOS
	// and Linux; on Windows the shim's folder, node_modules beside it.
	prefix := ""
	if before, _, ok := strings.Cut(filepath.ToSlash(in.RealPath), "/lib/node_modules/"+name+"/"); ok {
		prefix = filepath.FromSlash(before)
	} else if in.Source.Manager == "npm" {
		prefix = filepath.Dir(in.Path)
	}
	if prefix == "" {
		return Plan{}, false
	}
	p := Plan{
		Title:   locales.PkgUpgradeTitle(title),
		Why:     locales.AgentNpmUpdateWhy(),
		Steps:   []Step{{Argv: []string{"npm", "install", "--global", "--prefix", prefix, name + "@latest"}, Timeout: 10 * time.Minute}},
		Changes: []string{title + " " + in.Version + " → latest · npm"},
	}
	p.Verify = agentFound(title, find)
	return p, true
}

// agentFound is a check that finds the agent title again; nil when find
// is.
func agentFound(title string, find FindAgent) func(context.Context) (string, error) {
	if find == nil {
		return nil
	}
	return func(ctx context.Context) (string, error) {
		ag, err := find(ctx)
		if err != nil {
			return "", err
		}
		if !ag.Found() {
			return "", errors.New(locales.AgentCheckMissing(title))
		}
		version := ag.AppVersion
		if len(ag.Installations) > 0 {
			version = ag.Installations[0].Version
			if i := inventory.TerminalCopy(ag.Installations); i >= 0 {
				version = ag.Installations[i].Version
			}
		}
		return locales.AgentCheckFound(title, version), nil
	}
}
