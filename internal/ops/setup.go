package ops

import (
	"cmp"
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

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
	// With is the tool whose steps install this one too, as the system's
	// packages in one command; "" when it has steps of its own.
	With string
}

// unixUser is a user name as useradd makes them.
var unixUser = regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`)

// linuxManagers are the system package managers of Linux, whose
// packages the plan installs in one command: one password.
var linuxManagers = []string{"apt", "dnf", "zypper", "pacman", "apk"}

// miseVersions are the versions mise installs of a tool, where it is not
// the latest: Node's LTS, as Node advises for most, and Java's current
// LTS of Temurin, whose prefix mise installs the newest release of.
var miseVersions = map[string]string{"node": "lts", "java": "temurin-25"}

// Parts returns the tools s installs and how, in the order they install:
// each after what it needs. A tool there already, or with no way to
// install here, itself or what it needs, is left out.
func (s Setup) Parts() []SetupPart {
	var parts []SetupPart
	// ready are the tools there or installing, by whether they are.
	ready := map[string]bool{}
	var add func(id string) bool
	add = func(id string) bool {
		if s.Have[id] {
			return true
		}
		if r, seen := ready[id]; seen {
			return r
		}
		ready[id] = false
		e, _ := catalog.Find(id)
		m, ok := s.method(e)
		if !ok || slices.ContainsFunc(e.Conflicts, func(c string) bool { return s.Have[c] || ready[c] }) {
			return false
		}
		for _, need := range e.NeedsWith(m) {
			if !add(need) {
				return false
			}
		}
		ready[id] = true
		parts = append(parts, SetupPart{Tool: id, Method: m})
		return true
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
	// npm and Corepack run through mise, which finds the Node it keeps
	// though the terminal Hearth read has it on no PATH yet, as before
	// mise's line in the shell's startup; without one of its own, the
	// PATH's. Without mise, the terminal's Node runs them as it is.
	var viaMise []string
	if planned("mise") || s.Have["mise"] {
		viaMise = []string{"mise", "exec", "--"}
	}
	brew, _, _ := homebrew(env)
	mise := ""
	// The system's packages go in one command with the first of them:
	// one password.
	together, docker := -1, false
	var packages []inventory.SystemPackage
	for i, part := range parts {
		if !slices.Contains(linuxManagers, part.Method.Manager) ||
			together >= 0 && part.Method.Manager != parts[together].Method.Manager {
			continue
		}
		docker = docker || part.Tool == "docker"
		if together < 0 {
			together = i
		}
		packages = append(packages, inventory.SystemPackage{Name: part.Method.Package, Kind: part.Method.Kind})
	}
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
			// Homebrew's part installs the Command Line Tools first.
			if !planned("homebrew") {
				steps, notes = cltSteps()
			} else {
				part.With = "homebrew"
			}
		case m.Manager == "mise":
			steps = []Step{{Argv: []string{"mise", "use", "--global", m.Package + "@" + cmp.Or(miseVersions[part.Tool], "latest")}}}
		case m.Manager == "npm":
			steps = []Step{{Argv: append(slices.Clone(viaMise), "npm", "install", "--global", m.Package)}}
		case m.Manager == "corepack":
			// Node 25 and later no longer bring Corepack: npm installs it,
			// with shims for Yarn and pnpm both. Only the one chosen stays:
			// pnpm's would stand beside the pnpm mise installs.
			steps = []Step{
				{Argv: append(slices.Clone(viaMise), "npm", "install", "--global", "corepack")},
				{Argv: append(slices.Clone(viaMise), "corepack", "disable")},
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
			switch {
			case i == together:
				pp, ok = PackagePlan(PackageInstall, pm, packages, nil)
				// Docker Engine's socket is the docker group's: the user joins
				// it in the same command, one password, as Docker's own
				// steps after installing say. The command reads plain in
				// polkit's window: a user name as the system makes them
				// needs no quoting.
				if user := env.Get("USER"); ok && docker && unixUser.MatchString(user) && len(pp.Steps) == 1 && pp.Steps[0].Admin {
					install := pp.Steps[0]
					line := strings.Join(install.Argv, " ")
					if len(install.Argv) == 3 && install.Argv[0] == "/bin/sh" {
						line = install.Argv[2] // apt's, after its update
					}
					script := line + " && /usr/sbin/usermod -aG docker " + user
					pp.Steps = []Step{{Argv: []string{"/bin/sh", "-c", script}, Runs: install.Shown(), Admin: true}}
					pp.Notes = append(pp.Notes, Note{Title: locales.SetupDockerGroupTitle(), Text: locales.SetupDockerGroupText(user)})
				}
			case part.Tool == "docker" && !slices.Contains(linuxManagers, m.Manager):
				pp, ok = EngineInstallPlan(m.Engine, pm, false, nil)
			case together >= 0 && m.Manager == parts[together].Method.Manager:
				part.With = parts[together].Tool
			default:
				pp, ok = PackagePlan(PackageInstall, pm, []inventory.SystemPackage{{Name: m.Package, Kind: m.Kind}}, nil)
			}
			steps, notes = pp.Steps, pp.Notes
			if m.Manager == "winget" {
				steps, notes = windowsDatabaseSetup(part.Tool, steps, notes)
			}
			// Homebrew installs a server without starting it: brew
			// services starts it now and at each login, as the system's
			// managers do with their packages' services.
			if e, _ := catalog.Find(part.Tool); ok && e.Group == catalog.Database && m.Manager == "brew" {
				steps = append(steps, Step{Argv: []string{pm.Path, "services", "start", m.Package[strings.LastIndex(m.Package, "/")+1:]}, Timeout: 2 * time.Minute})
			}
			// Rust builds on Windows with Microsoft's linker, which Visual
			// Studio Build Tools bring: rustup warns without them.
			if part.Tool == "rust" && s.GOOS == "windows" && !s.Have["cpp"] {
				if cp, found := compilerPlan(s.Managers); found {
					steps = append(slices.Clone(cp.Steps), steps...)
					// WinGet's own note says Windows asks through UAC.
					notes = append([]Note{{Title: locales.SetupRustMSVC(), Text: locales.SetupRustMSVCText()}}, notes...)
				}
			}
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
	// The mise set up here is not on the terminal's PATH yet: its steps
	// run it by path, with its folder on PATH for what it runs, as npm
	// of the Node it keeps, which runs mise reshim.
	if mise != "" {
		path := "PATH=" + filepath.Dir(mise) + string(filepath.ListSeparator) + env.Get("PATH")
		for i, step := range p.Steps {
			if len(step.Argv) > 0 && step.Argv[0] == "mise" {
				p.Steps[i].Argv = append([]string{mise}, step.Argv[1:]...)
				p.Steps[i].Env = append(slices.Clone(step.Env), path)
			}
		}
	}
	chainEdits(p.Steps)
	for _, part := range parts {
		p.Parts = append(p.Parts, part.From)
	}
	p.Action = locales.SetupInstall(len(parts))
	return p, parts, nil
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
