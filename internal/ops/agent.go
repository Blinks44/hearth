package ops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// FindAgent finds an agent again, as a scan would: an agent plan's check
// uses it.
type FindAgent func(ctx context.Context) (inventory.Agent, error)

// AgentNpmUpdatePlan returns the plan that updates in, a copy of the
// agent id npm installed, to the latest release in the npm prefix that
// holds it, which may be another Node's than the terminal's npm; ok is
// false for a copy npm did not install.
func AgentNpmUpdatePlan(id, title string, in inventory.Installation, find FindAgent) (Plan, bool) {
	name := ""
	e, _ := catalog.Find(id)
	for _, method := range e.Methods {
		if method.Manager == "npm" {
			name = method.Package
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
	p.Verify = AgentFound(title, find)
	return p, true
}

// AgentFound is a check that finds the agent title again; nil when find
// is.
func AgentFound(title string, find FindAgent) func(context.Context) (string, error) {
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

// ToolFound is a check that finds the tool title again, with a copy the
// terminal runs; nil when detect is.
func ToolFound(title string, detect Detect) func(context.Context) (string, error) {
	if detect == nil {
		return nil
	}
	return func(ctx context.Context) (string, error) {
		st, err := detect(ctx)
		if err != nil {
			return "", err
		}
		i := inventory.TerminalCopy(st.Installations)
		if i < 0 {
			return "", errors.New(locales.AgentCheckMissing(title))
		}
		return locales.AgentCheckFound(title, st.Installations[i].Version), nil
	}
}
