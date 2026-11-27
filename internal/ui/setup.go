package ui

import (
	"errors"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// setupPage is the Install page's path: the catalog to install from,
// which is the new computer wizard on a computer with nothing set up.
const setupPage = "/install"

// goos is the system the wizard sets up; tests change it.
var goos = runtime.GOOS

// installCat groups catalog tools into balanced visual categories on the
// Install page while keeping catalog.Group unchanged for ops and sets.
type installCat int

const (
	catAll installCat = iota
	catLanguages
	catPackages
	catInfra
	catDatabases
	catAgents
	catTerminal
)

// setupState is where the new computer wizard is.
type setupState struct {
	step int // 0 the choice, 1 the plan, 2 the installation, 3 the check
	// tab is the stepper's segment, which goes back to the choice; shown
	// the step it showed, which tells the user's click from a new step.
	tab, shown int
	back       bool // the last step change went back, which the step comes in from
	// choice is what the user switched on or off, by the catalog's ID;
	// the others are as setupOffered says.
	choice   map[string]bool
	engine   string            // the Docker engine's package chosen; "" for the first
	packages map[string]string // chosen versioned package override by catalog ID
	versions map[string]string // chosen mise version spec override by catalog ID
	// filter is the choice's segment: all, not installed, installed;
	// cat narrows to one category; query narrows the tools shown;
	// more expands "N more" per category; showHave expands installed summaries.
	filter    int
	cat       installCat
	query     string
	setNotice string
	more      map[installCat]bool
	showHave  map[installCat]bool
	// wizard keeps the page the wizard once it opened as one, as the
	// Overview's offer to set the computer up does.
	wizard bool
	// plan and parts are the plan made of the choice for step 2;
	// building reports it is being made.
	plan     ops.Plan
	parts    []ops.SetupPart
	building bool
	diff     string // the file whose diff shows on the plan; "" for none
	// op is the operation that installs the plan, from step 3 on.
	op *operation
}

// setupOffered are the tools the wizard switches on until the user says
// otherwise.
var setupOffered = []string{"node", "python", "pnpm", "git", "docker", "claude"}

// setupFeatured is how many available tools of a category show before "N more".
const setupFeatured = 8

// catalogCLITools are catalog runtimes and command-line utilities detected by
// detect.Cli without being added to the CLI & clouds page table.
var catalogCLITools = []string{"deno", "zig", "elixir", "jq", "ripgrep", "fzf", "fd", "bat", "just", "direnv", "k9s"}

func isCatalogCLITool(id string) bool {
	return slices.Contains(catalogCLITools, id)
}

// setupWizard reports that the Install page is the new computer wizard:
// on a computer with nothing set up, it offers a set to start with and
// installs the base with it.
func (a *App) setupWizard() bool { return a.setup.wizard || a.bareComputer() }

// setupChosen reports that the tool id is switched on.
func (a *App) setupChosen(id string) bool {
	if on, ok := a.setup.choice[id]; ok {
		return on
	}
	if !a.setupWizard() {
		return false
	}
	// An engine with terms to read, as Docker Desktop's on Windows, waits
	// for the user to choose it.
	if e, _ := catalog.Find(id); id == "docker" {
		if ms := e.On(goos); len(ms) > 0 && ms[0].Terms != "" {
			return false
		}
	}
	return slices.Contains(setupOffered, id)
}

// setupHave reports that the tool id is on the computer already, as the
// last scan found it: mise hooked into the shell, as the wizard sets it
// up.
func (a *App) setupHave(id string) bool {
	switch id {
	case "homebrew":
		return slices.ContainsFunc(a.packageManagers(), func(m inventory.PackageManager) bool { return m.ID == "brew" })
	case "mise":
		m, ok := a.miseManager()
		return ok && (m.Activated || m.Shims)
	case "kubectl", "helm", "kind", "aws", "gcloud", "az", "terraform", "tofu":
		_, found := cliCopy(a.cliState(), id)
		return found
	case "uv":
		st, _ := a.Snapshot.Tool("python")
		return slices.ContainsFunc(st.Managers, func(m inventory.Manager) bool { return m.Name == "uv" && m.Version != "" })
	case "gh":
		st, _ := a.Snapshot.Tool("git")
		return st.Git != nil && st.Git.GitHub != nil
	}
	if isCatalogCLITool(id) {
		_, found := cliCopy(a.cliState(), id)
		return found
	}
	// Commands a language's scan finds beside it.
	if lang, command, ok := languageCommand(id); ok {
		st, _ := a.Snapshot.Tool(lang)
		return st.Vars[command] != ""
	}
	if kind, ok := catalog.DatabaseKinds[id]; ok {
		_, found := a.databaseServer(kind)
		return found
	}
	e, _ := catalog.Find(id)
	if e.Group == catalog.Agent {
		return slices.Contains(a.foundIDs(), id)
	}
	if a.version(id) == "" {
		return false
	}
	// A language the system brings, as macOS's Ruby 2.6, is there for
	// the system's own scripts: the wizard installs one to work with,
	// unless the system's manager is how it installs here.
	if ms := e.On(goos); e.Group == catalog.Language && len(ms) > 0 && !systemManager(ms[0].Manager) {
		st, _ := a.Snapshot.Tool(id)
		return slices.ContainsFunc(st.Installations, func(in inventory.Installation) bool {
			return !in.Stub && in.Source.Kind != inventory.KindSystem
		})
	}
	return true
}

// languageCommand is the language whose scan finds the tool id, and the
// command it finds; false for a tool with a scan of its own.
func languageCommand(id string) (lang, command string, ok bool) {
	switch id {
	case "pipx":
		return "python", "pipx", true
	case "bundler":
		return "ruby", "bundle", true
	case "maven":
		return "java", "mvn", true
	case "gradle":
		return "java", "gradle", true
	}
	return "", "", false
}

// setupSystemCopy is the system's own copy of the language id when that
// is all there is: setupHave does not count it, and Hearth installs one to
// work with beside it.
func (a *App) setupSystemCopy(id string) (inventory.Installation, bool) {
	if a.setupHave(id) {
		return inventory.Installation{}, false
	}
	st, _ := a.Snapshot.Tool(id)
	i := slices.IndexFunc(st.Installations, func(in inventory.Installation) bool { return !in.Stub && in.Source.Kind == inventory.KindSystem })
	if i < 0 {
		return inventory.Installation{}, false
	}
	return st.Installations[i], true
}

// systemManager reports that manager is the system's package manager,
// whose packages are the system's own copies.
func systemManager(manager string) bool {
	switch manager {
	case "apt", "dnf", "zypper", "pacman", "apk":
		return true
	}
	return false
}

// miseManager is mise as the last scan found it beside the tools it
// keeps: its version, and how the shell runs it; false when no tool
// names a mise that runs.
func (a *App) miseManager() (inventory.Manager, bool) {
	if a.Snapshot == nil {
		return inventory.Manager{}, false
	}
	var found inventory.Manager
	for _, st := range a.Snapshot.Tools {
		for _, m := range st.Managers {
			if m.Name == "mise" && m.Version != "" && (found.Version == "" || m.Activated || m.Shims) {
				found = m
			}
		}
	}
	return found, found.Version != ""
}

// packageManagers are the system's package managers the last scan found.
func (a *App) packageManagers() []inventory.PackageManager {
	if a.Snapshot == nil {
		return nil
	}
	st, _ := a.Snapshot.Tool("packages")
	return st.PackageManagers
}

// setupInput is what the page installs, as chosen: the wizard's the base
// always.
func (a *App) setupInput() ops.Setup {
	s := a.setupHere()
	s.Packages = maps.Clone(a.setup.packages)
	s.Versions = maps.Clone(a.setup.versions)
	wizard := a.setupWizard()
	now := time.Now()
	for _, e := range a.setupEntries() {
		if wizard && e.Group == catalog.Base || a.setupChosen(e.ID) {
			s.Tools = append(s.Tools, e.ID)
			if s.Versions[e.ID] != "" {
				continue
			}
			for _, m := range e.On(goos) {
				if !s.Usable(m) {
					continue
				}
				if m.Manager == "mise" {
					if opts := a.setupVersionOptions(e.ID, m, now); len(opts) > 0 {
						sel := a.setupSelectedOption(e.ID, opts)
						if !sel.isPkg && sel.value != "" {
							if s.Versions == nil {
								s.Versions = map[string]string{}
							}
							s.Versions[e.ID] = sel.value
						}
					}
				}
				break
			}
		}
	}
	return s
}

// setupHere is a setup of this computer, as the last scan found it,
// that installs nothing yet.
func (a *App) setupHere() ops.Setup {
	s := ops.Setup{
		GOOS:     goos,
		Managers: a.packageManagers(),
		Engine:   a.setup.engine,
		Have:     map[string]bool{},
		Title:    setupTitle,
	}
	// Rust on Windows needs a C++ compiler, which is not in the catalog.
	if a.version("cpp") != "" {
		s.Have["cpp"] = true
	}
	for _, e := range a.setupEntries() {
		if a.setupHave(e.ID) {
			s.Have[e.ID] = true
		}
	}
	return s
}

// setupEntries are the catalog's tools that install on this system.
func (a *App) setupEntries() []catalog.Entry {
	var out []catalog.Entry
	for _, e := range catalog.Tools {
		if slices.Contains(e.Systems(), goos) {
			out = append(out, e)
		}
	}
	return out
}

func (a *App) setupView(c *ui.Context) {
	s := &a.setup
	// The installation went through: the check follows.
	if s.step == 2 && s.op.state.Phase == ops.Succeeded {
		s.step = 3
	}
	// The stepper goes back to the choice alone, before the installation.
	if s.tab != s.shown && s.tab < s.step && s.step <= 1 {
		s.step, s.diff = s.tab, ""
	}
	if s.step != s.shown {
		s.back = s.step < s.shown
	}
	s.tab, s.shown = s.step, s.step
	// Out of the wizard the choice is the catalog with a filter; the
	// plan, the installation and the check are the wizard's.
	catalogView := s.step == 0 && !a.setupWizard()
	ui.Column(c).Fill().Children(func() {
		t := c.Theme()
		pal := Colors(t)
		subtitle := []string{locales.SetupStepChoice(), locales.SetupStepPlan(), locales.SetupStepInstall(), locales.SetupStepCheck()}[s.step]
		if catalogView {
			subtitle = locales.InstallSubtitle()
		}
		a.headerWith(c, locales.InstallTitle(), subtitle, headerExtras{after: func() { a.setupButtons(c) }})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).ChildExit(ui.Motion{X: -stepShift(s.back)}).Children(func() {
				if catalogView {
					have, total := a.setupCounts()
					toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
					if narrow(c) {
						toolbar.Wrap()
					}
					toolbar.Children(func() {
						segments(c, "install-filter", &s.filter, []segment{
							{locales.InstallFilterAll(), strconv.Itoa(total)},
							{locales.InstallFilterMissing(), strconv.Itoa(total - have)},
							{locales.InstallFilterHave(), strconv.Itoa(have)},
						})
						ui.Box(c).Grow(1).Height(1)
						ui.SearchField(c.Key("install-filter-search"), &s.query).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Height(controlHeight).Width(200)
					})
					a.setupChoiceView(c)
					return
				}
				ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Shrink(0).Radius(12).Background(pal.AccentSoft).Center().Children(func() {
						ui.Icon(c, Icon("sparkles")).Size(26, 26).TextColor(Accent)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						title, lead := a.setupHeading()
						ui.Text(c, title).FontSize(22).FontWeight(600)
						ui.Text(c, lead).FontSize(12).TextColor(t.TextMuted)
					})
				})
				stepper := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					stepper.Wrap()
				}
				stepper.Children(func() {
					segments(c, "setup-steps", &s.tab, []segment{
						{"1 " + locales.SetupSegChoice(), ""}, {"2 " + locales.SetupSegPlan(), ""},
						{"3 " + locales.SetupSegInstall(), ""}, {"4 " + locales.SetupSegCheck(), ""},
					})
					if s.step == 0 {
						ui.Box(c).Grow(1).Height(1)
						ui.SearchField(c.Key("install-filter-search"), &s.query).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Height(controlHeight).Width(200)
					}
				})
				if s.step >= 2 {
					a.operationProgress(c.Key("setup-progress"), s.op)
				}
				ui.Column(c.Key(s.step)).Gap(18).Transition(stepMotion(s.back)).Children(func() {
					switch s.step {
					case 0:
						a.setupChoiceView(c)
					case 1:
						a.setupPlanView(c)
					case 2:
						a.setupInstallView(c)
					case 3:
						a.setupCheckView(c)
					}
				})
			})
		})
	})
}

// setupHeading is the title and the lead of the step shown.
func (a *App) setupHeading() (title, lead string) {
	s := &a.setup
	switch s.step {
	case 1:
		return locales.SetupPlanHeading(), locales.SetupPlanLead()
	case 2:
		i := a.setupCurrent()
		st := s.op.state
		switch st.Phase {
		case ops.Failed, ops.NotVerified:
			return locales.SetupFailed(setupTitle(s.parts[i].Tool)), locales.SetupFailedLead(failReason(st))
		case ops.Stopped:
			return locales.SetupStopped(), locales.SetupStoppedLead()
		}
		return locales.SetupInstalling(), locales.SetupInstallLead(i+1, len(s.parts), setupTitle(s.parts[i].Tool))
	case 3:
		if a.Scanning {
			return locales.SetupChecking(), locales.SetupCheckingLead()
		}
		missing, left := 0, false
		for _, part := range s.parts {
			switch _, r := a.setupResult(part.Tool); r {
			case checkMissing:
				missing++
			case checkStopped, checkDenied:
				left = true
			}
		}
		switch {
		case missing > 0:
			return locales.SetupMissingTitle(), locales.SetupMissingLead(missing)
		case left:
			return locales.SetupLeftTitle(), locales.SetupLeftLead()
		}
		return locales.SetupDoneTitle(), locales.SetupDoneLead()
	}
	return locales.SetupChoiceTitle(), locales.SetupChoiceLead()
}

// setupButtons are the header's buttons: cancel and go on to the plan on
// the choice; back and install on the plan.
func (a *App) setupButtons(c *ui.Context) {
	s := &a.setup
	switch {
	case s.step == 3:
		if a.canOpenFile() && a.JournalPath != "" {
			ui.Button(c.Key("setup-journal"), locales.OpOpenJournal()).Height(controlHeight).OnClick(func() { a.openFile(c, a.JournalPath) })
		}
		// The wizard ends on the Overview; the page shows the catalog
		// again, with what installed.
		ui.PrimaryButton(c.Key("setup-done"), locales.OpDone()).Height(controlHeight).OnClick(func() {
			wizard := a.setupWizard()
			a.setup = setupState{}
			if wizard {
				a.router.Push(overview.path)
			}
		})
	case s.step == 2 && !s.op.state.Phase.Over():
		// The operation's toast shows it away from the wizard.
		ui.Button(c.Key("setup-hide"), locales.OpHide()).Height(controlHeight).OnClick(func() { a.router.Push(overview.path) })
		ui.Button(c.Key("setup-stop"), locales.OpStop()).Height(controlHeight).TextColor(c.Theme().Danger).OnClick(func() { s.op.stop() })
	case s.step == 2:
		ui.Button(c.Key("setup-details"), locales.OpDetails()).Height(controlHeight).OnClick(func() { s.op.open = true })
		label := locales.SetupAgain()
		if s.building {
			label = locales.SetupBuilding()
		}
		// The plan made again leaves out what the scan after the
		// installation found.
		ui.PrimaryButton(c.Key("setup-again"), label).Height(controlHeight).Disabled(s.building || a.Scanning || a.BuildPlan == nil).OnClick(a.setupNext)
	case s.step == 1:
		ui.Button(c.Key("setup-back"), locales.SetupBack()).Height(controlHeight).OnClick(func() { s.step, s.diff = 0, "" })
		ui.PrimaryButton(c.Key("setup-install"), s.plan.Action).Height(controlHeight).Disabled(a.op.running()).OnClick(a.setupInstall)
	default:
		label := locales.SetupNext()
		if a.setupWizard() {
			ui.Button(c.Key("setup-cancel"), locales.SetupCancel()).Height(controlHeight).OnClick(func() { a.router.Push(overview.path) })
			if a.OpenSet != nil {
				ui.Button(c.Key("set-open"), locales.SetOpen()).Height(controlHeight).OnClick(a.openSet)
			}
		} else {
			a.setMenu(c)
			if chosen := a.setupChosenTools(); len(chosen) > 0 {
				ui.Button(c.Key("setup-clear"), locales.InstallClear()).Height(controlHeight).OnClick(func() {
					for _, id := range chosen {
						s.choice[id] = false
						delete(s.packages, id)
						delete(s.versions, id)
					}
					s.setNotice = ""
				})
				label = locales.SetupNextCount(len(chosen))
			}
		}
		if s.building {
			label = locales.SetupBuilding()
		}
		ui.PrimaryButton(c.Key("setup-next"), label).Height(controlHeight).
			Disabled(s.building || a.BuildPlan == nil || len(a.setupInput().Parts()) == 0).OnClick(a.setupNext)
	}
}

// setupNext makes the plan of the choice and shows it.
func (a *App) setupNext() {
	s := &a.setup
	in := a.setupInput()
	var parts []ops.SetupPart
	s.building = true
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		p, ps, err := ops.SetupPlan(in, env)
		parts = ps
		return p, err
	}, func(p ops.Plan, err error) {
		s.building = false
		if err != nil {
			a.planErr = err.Error()
			return
		}
		s.plan, s.parts, s.step, s.diff = p, parts, 1, ""
	})
}

// setupInstall runs the plan the user read on step 2, which step 3
// shows as it goes.
func (a *App) setupInstall() {
	if a.RunPlan == nil || a.op.running() {
		return
	}
	a.Propose(a.setup.plan)
	a.op.open = false
	a.startOperation()
	a.setup.op, a.setup.step, a.setup.diff = a.op, 2, ""
}

// setMenu is the catalog's menu that saves the tools here as a set, and
// opens one.
func (a *App) setMenu(c *ui.Context) {
	if a.SaveSet == nil && a.OpenSet == nil {
		return
	}
	ui.Button(c.Key("set-menu"), "").Size(controlHeight, controlHeight).Shrink(0).Padding(0).Label(locales.MoreActions()).Menu(func(m *ui.Menu) {
		if a.SaveSet != nil && m.Item(locales.SetSave()).Disabled(len(a.setupSetTools()) == 0).Chosen() {
			a.saveSet()
		}
		if a.OpenSet != nil && m.Item(locales.SetOpen()).Chosen() {
			a.openSet()
		}
	}).Children(func() {
		ui.Icon(c, Icon("ellipsis")).Size(14, 14).TextColor(c.Theme().TextMuted)
	})
}

// setupSetTools are the catalog's tools on this computer, which its set
// saves.
func (a *App) setupSetTools() []string {
	var out []string
	for _, e := range a.setupEntries() {
		if a.setupHave(e.ID) {
			out = append(out, e.ID)
		}
	}
	return out
}

// saveSet saves the tools here as a set, to a file the user chooses.
func (a *App) saveSet() {
	data, err := catalog.MarshalSet(a.setupSetTools())
	if err != nil {
		a.planErr = locales.SetSaveFailed(err.Error())
		return
	}
	a.SaveSet(data, func(path string, err error) {
		switch {
		case err != nil:
			a.planErr = locales.SetSaveFailed(err.Error())
		case path != "":
			a.notice = locales.SetSaved(tilde(path, a.Home))
		}
	})
}

// openSet reads a set from a file the user chooses and previews its
// selection on step 1.
func (a *App) openSet() {
	a.OpenSet(func(path string, data []byte, err error) {
		if path == "" {
			return
		}
		if err != nil {
			a.planErr = locales.OpenFailed(tilde(path, a.Home), err.Error())
			return
		}
		tools, err := catalog.ParseSet(data)
		switch {
		case errors.Is(err, catalog.ErrNewerSet):
			a.planErr = locales.SetNewer()
		case err != nil:
			a.planErr = locales.SetNotSet(filepath.Base(path))
		default:
			a.applySet(tools)
		}
	})
}

// applySet switches on the tools of a set, and every other off, and
// stays on step 1 so the user can review and adjust the selection before
// continuing to the plan; tools that do not install here are left out,
// and said so but for another system's base, as Homebrew on Windows.
func (a *App) applySet(tools []string) {
	s := &a.setup
	s.choice = map[string]bool{}
	s.packages = nil
	s.versions = nil
	for _, e := range a.setupEntries() {
		s.choice[e.ID] = slices.Contains(tools, e.ID)
	}
	s.step, s.diff, s.filter, s.cat, s.query = 0, "", 0, catAll, ""
	parts := a.setupInput().Parts()
	planned := func(id string) bool {
		return slices.ContainsFunc(parts, func(p ops.SetupPart) bool { return p.Tool == id })
	}
	var left []string
	selected := 0
	for _, id := range tools {
		e, _ := catalog.Find(id)
		switch {
		case a.setupHave(id):
		case planned(id):
			selected++
		case e.Group == catalog.Base:
		default:
			left = append(left, setupTitle(id))
		}
	}
	if len(left) > 0 {
		a.notice = locales.SetNotHere(strings.Join(left, ", "))
	}
	if selected == 0 {
		s.setNotice = ""
		if len(left) == 0 {
			a.notice = locales.SetAllHere()
		}
		return
	}
	s.setNotice = locales.SetLoadedPreview(selected)
}
