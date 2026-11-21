package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// installSetup installs the catalog's tool id alone, with what it needs
// first that is not there, as the wizard would.
func (a *App) installSetup(id string) ops.Setup {
	s := a.setupHere()
	s.Tools = []string{id}
	return s
}

// canInstallTool reports that Hearth can install the catalog's tool id
// here, which the last scan did not find.
func (a *App) canInstallTool(id string) bool {
	if a.Snapshot == nil || a.BuildPlan == nil || a.setupHave(id) {
		return false
	}
	return slices.ContainsFunc(a.installSetup(id).Parts(), func(p ops.SetupPart) bool { return p.Tool == id })
}

// installTool offers the plan that installs the catalog's tool id, with
// what it needs first.
func (a *App) installTool(id string) {
	a.installToolPlan(id, a.proposeOrSay)
}

func (a *App) installToolPlan(id string, done func(ops.Plan, error)) {
	s := a.installSetup(id)
	title := setupTitle(id)
	verify := ops.ToolFound(title, a.detect(id))
	if e, _ := catalog.Find(id); e.Group == catalog.Agent {
		verify = ops.AgentFound(title, a.findAgent(id))
	}
	if kind, ok := catalog.DatabaseKinds[id]; ok {
		verify = ops.DatabaseFound(title, kind, a.detect("databases"))
	}
	if slices.Contains(kubeCommands, id) || isCLICommand(id) {
		verify = ops.CommandFound(title, id, a.detect("cli"))
	}
	if id == "uv" {
		verify = ops.ManagerFound(title, "uv", a.detect("python"))
	}
	if id == "gh" {
		verify = ops.GitHubFound(title, a.detect("git"))
	}
	if lang, command, ok := languageCommand(id); ok {
		verify = ops.VarCommandFound(title, command, a.detect(lang))
	}
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		p, parts, err := ops.SetupPlan(s, env)
		if err != nil {
			return p, err
		}
		p.Title, p.Why, p.Action, p.Verify = locales.PkgInstallTitle(title), installWhy(parts), locales.Install(), verify
		return p, nil
	}, done)
}

// installWhy says how a tool installs, the last of parts: through what,
// after what it needs.
func installWhy(parts []ops.SetupPart) string {
	last := parts[len(parts)-1]
	title, via := setupTitle(last.Tool), managerTitle(last.Method)
	if len(parts) == 1 {
		return locales.InstallWhy(title, via)
	}
	var needs []string
	for _, p := range parts[:len(parts)-1] {
		// Homebrew found where the scan did not look has no steps.
		if p.To > p.From || p.With != "" {
			needs = append(needs, setupTitle(p.Tool))
		}
	}
	if len(needs) == 0 {
		return locales.InstallWhy(title, via)
	}
	return locales.InstallNeedsWhy(title, strings.Join(needs, ", "), via)
}

// installToolButton is the button that installs the catalog's tool id,
// shown when Hearth can.
func (a *App) installToolButton(c *ui.Context, id string) {
	if a.canInstallTool(id) {
		ui.Button(c.Key("install:"+id), locales.AgentInstall()).Shrink(0).OnClick(func() { a.installTool(id) })
	}
}

// installPrimary is the prominent button of an empty state that installs
// the catalog's tool id, shown when Hearth can.
func (a *App) installPrimary(c *ui.Context, id string) {
	if id != "" && a.canInstallTool(id) {
		ui.Row(c).Padding(6, 0, 0, 0).Children(func() {
			ui.PrimaryButton(c.Key("install:"+id), locales.InstallNamed(setupTitle(id))).OnClick(func() { a.installTool(id) })
		})
	}
}
