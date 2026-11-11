package ops

import (
	"cmp"
	"errors"
	"slices"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// Setup is what the new computer wizard installs.
type Setup struct {
	// Tools are the catalog's tools chosen; what they need comes along.
	Tools []string
	// Have are the tools there already, which do not install again.
	Have map[string]bool
	// Engine is the package of the Docker engine chosen, as the catalog
	// names it; "" for the first that installs here.
	Engine string
	// GOOS is the system, as runtime.GOOS names it; Managers are its
	// package managers found.
	GOOS     string
	Managers []inventory.PackageManager
	// Title names a tool, as the wizard shows it.
	Title func(id string) string
}

// SetupPart is a tool of a setup and how it installs: the plan's steps
// from From up to To.
type SetupPart struct {
	Tool     string
	Method   catalog.Method
	From, To int
}

// miseVersions are the versions mise installs of a tool, where it is not
// the latest: Node's LTS, as Node advises for most, and Java's current
// LTS of Temurin, whose prefix mise installs the newest release of.
var miseVersions = map[string]string{"node": "lts", "java": "temurin-25"}

// Parts returns the tools s installs and how, in the order they install:
// each after what it needs. A tool there already, or with no way to
// install here, is left out.
func (s Setup) Parts() []SetupPart {
	var parts []SetupPart
	seen := map[string]bool{}
	var add func(id string)
	add = func(id string) {
		if seen[id] || s.Have[id] {
			return
		}
		seen[id] = true
		e, _ := catalog.Find(id)
		m, ok := s.method(e)
		if !ok {
			return
		}
		for _, need := range e.NeedsWith(m) {
			add(need)
		}
		parts = append(parts, SetupPart{Tool: id, Method: m})
	}
	for _, e := range catalog.Tools {
		if slices.Contains(s.Tools, e.ID) {
			add(e.ID)
		}
	}
	return parts
}

// method is the first of e's methods that works here; Docker's is the
// engine chosen.
func (s Setup) method(e catalog.Entry) (catalog.Method, bool) {
	for _, m := range e.On(s.GOOS) {
		if e.ID == "docker" && s.Engine != "" && m.Package != s.Engine {
			continue
		}
		if s.Usable(m) {
			return m, true
		}
	}
	return catalog.Method{}, false
}

// Usable reports that m works here: through a tool of the catalog,
// which installs first, the maker's installer, or a package manager
// found.
func (s Setup) Usable(m catalog.Method) bool {
	switch m.Manager {
	case "installer", "xcode-select", "brew", "mise", "npm", "corepack":
		return true
	}
	return slices.ContainsFunc(s.Managers, func(pm inventory.PackageManager) bool { return pm.ID == m.Manager })
}

// SetupPlan returns the plan that installs what s does, in the order of
// its parts, and the parts. env is the terminal's: the plan finds mise
// and Homebrew there, or where they will be once installed.
func SetupPlan(s Setup, env sysenv.Env) (Plan, []SetupPart, error) {
	parts := s.Parts()
	if len(parts) == 0 {
		return Plan{}, nil, errors.New(locales.SetupNothing())
	}
	p := Plan{Title: locales.SetupPlanTitle(), Why: locales.SetupPlanWhy()}
	planned := func(id string) bool {
		return slices.ContainsFunc(parts, func(part SetupPart) bool { return part.Tool == id })
	}
	// npm and Corepack run through the Node mise installs, which no
	// terminal has on its PATH yet.
	var viaMise []string
	if planned("node") {
		viaMise = []string{"mise", "exec", "--"}
	}
	brew, _, _ := homebrew(env)
	mise := ""
	for i := range parts {
		part := &parts[i]
		m, title := part.Method, s.Title(part.Tool)
		var steps []Step
		var notes []Note
		ok := true
		switch {
		case part.Tool == "mise":
			setup, err := SetUpMise(env)
			if err != nil {
				return Plan{}, nil, err
			}
			// Homebrew's steps are its own part.
			steps, notes, mise = setup.Steps[setup.Base:], setup.Notes, setup.Mise
			if setup.Base > 0 {
				notes = slices.DeleteFunc(slices.Clone(notes), func(n Note) bool { return n.Tag == "Homebrew" })
			}
		case part.Tool == "homebrew":
			_, steps, notes = homebrew(env)
		case m.Manager == "installer":
			steps, ok = installerSteps(part.Tool, env)
		case m.Manager == "xcode-select":
			// Homebrew's package brings the Command Line Tools.
			if !planned("homebrew") {
				cp, _ := compilerPlan(nil)
				steps, notes = cp.Steps, cp.Notes
			}
		case m.Manager == "mise":
			steps = []Step{{Argv: []string{"mise", "use", "--global", m.Package + "@" + cmp.Or(miseVersions[part.Tool], "latest")}}}
		case m.Manager == "npm":
			steps = []Step{{Argv: append(slices.Clone(viaMise), "npm", "install", "--global", m.Package)}}
		case m.Manager == "corepack":
			// Node 25 and later no longer bring Corepack: npm installs it.
			steps = []Step{
				{Argv: append(slices.Clone(viaMise), "npm", "install", "--global", "corepack")},
				{Argv: append(slices.Clone(viaMise), "corepack", "enable", m.Package)},
			}
			if viaMise != nil {
				steps = append(steps, Step{Argv: []string{"mise", "reshim"}})
			}
		default:
			pm := inventory.PackageManager{ID: m.Manager, Name: m.Manager, Path: brew}
			if i := slices.IndexFunc(s.Managers, func(f inventory.PackageManager) bool { return f.ID == m.Manager }); i >= 0 {
				pm = s.Managers[i]
			}
			var pp Plan
			if part.Tool == "docker" {
				pp, ok = EngineInstallPlan(EngineName(m.Package), pm, false, nil)
			} else {
				pp, ok = PackagePlan(PackageInstall, pm, []inventory.SystemPackage{{Name: m.Package, Kind: m.Kind}}, nil)
			}
			steps, notes = pp.Steps, pp.Notes
		}
		if !ok {
			return Plan{}, nil, errors.New(locales.SetupNoWay(title))
		}
		part.From = len(p.Steps)
		p.Steps = append(p.Steps, steps...)
		part.To = len(p.Steps)
		for _, n := range notes {
			if !slices.Contains(p.Notes, n) {
				p.Notes = append(p.Notes, n)
			}
		}
		p.Changes = append(p.Changes, "+ "+title)
	}
	// The mise set up here is not on the terminal's PATH yet.
	if mise != "" {
		for i, step := range p.Steps {
			if len(step.Argv) > 0 && step.Argv[0] == "mise" {
				p.Steps[i].Argv = append([]string{mise}, step.Argv[1:]...)
			}
		}
	}
	chainEdits(p.Steps)
	p.Action = locales.SetupInstall(len(parts))
	return p, parts, nil
}

// EngineName is the Docker engine the catalog's package installs, as
// EngineInstallPlan names it.
func EngineName(pkg string) string {
	switch pkg {
	case "colima":
		return "Colima"
	case "orbstack":
		return "OrbStack"
	case "Docker.DockerDesktop":
		return "Docker Desktop"
	}
	return "Docker Engine"
}

// chainEdits makes each edit of a file an earlier step edits too start
// from the text the earlier one leaves, adding the lines it adds to
// Hearth's block: each was made from the file as it is now.
func chainEdits(steps []Step) {
	after := map[string]string{}
	for i, step := range steps {
		e := step.Edit
		if e == nil {
			continue
		}
		if prev, ok := after[e.Path]; ok {
			had := blockBody(e.Before)
			var added []string
			for _, l := range blockBody(e.After) {
				if !slices.Contains(had, l) {
					added = append(added, l)
				}
			}
			chained := *e
			chained.Before, chained.After, chained.New = prev, AddToBlock(prev, added...), false
			steps[i].Edit = &chained
		}
		after[e.Path] = steps[i].Edit.After
	}
}

// blockBody are the lines of Hearth's block in text.
func blockBody(text string) []string {
	lines := splitLines(text)
	start, end := blockLines(lines)
	if start < 0 {
		return nil
	}
	return lines[start+1 : end]
}
