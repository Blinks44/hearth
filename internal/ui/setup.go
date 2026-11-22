package ui

import (
	"cmp"
	"errors"
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

// setupState is where the new computer wizard is.
type setupState struct {
	step int // 0 the choice, 1 the plan, 2 the installation, 3 the check
	// tab is the stepper's segment, which goes back to the choice; shown
	// the step it showed, which tells the user's click from a new step.
	tab, shown int
	back       bool // the last step change went back, which the step comes in from
	// choice is what the user switched on or off, by the catalog's ID;
	// the others are as setupOffered says.
	choice map[string]bool
	engine string // the Docker engine's package chosen; "" for the first
	// filter is the choice's segment: all, not installed, installed;
	// query narrows the tools shown; more are the groups whose every tool shows.
	filter    int
	query     string
	setNotice string
	more      map[catalog.Group]bool
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

// setupFeatured is how many tools of a group show before "N more".
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
	wizard := a.setupWizard()
	for _, e := range a.setupEntries() {
		if wizard && e.Group == catalog.Base || a.setupChosen(e.ID) {
			s.Tools = append(s.Tools, e.ID)
		}
	}
	return s
}

// setupHere is a setup of this computer, as the last scan found it,
// that installs nothing yet.
func (a *App) setupHere() ops.Setup {
	s := ops.Setup{GOOS: goos, Managers: a.packageManagers(), Engine: a.setup.engine, Have: map[string]bool{}, Title: setupTitle}
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

// setupTitle names a tool of the catalog in the wizard.
func setupTitle(id string) string {
	switch id {
	case "homebrew":
		return "Homebrew"
	case "mise":
		return "mise"
	case "node":
		return "Node.js LTS"
	case "docker":
		return locales.SetupDockerEngine()
	case "aws":
		return "AWS CLI"
	case "gcloud":
		return "Google Cloud CLI"
	case "az":
		return "Azure CLI"
	case "terraform":
		return "Terraform"
	case "tofu":
		return "OpenTofu"
	case "gh":
		return "GitHub CLI"
	case "deno":
		return "Deno"
	case "zig":
		return "Zig"
	case "elixir":
		return "Elixir"
	case "jq":
		return "jq"
	case "ripgrep":
		return "ripgrep"
	case "fzf":
		return "fzf"
	case "fd":
		return "fd"
	case "bat":
		return "bat"
	case "just":
		return "just"
	case "direnv":
		return "direnv"
	case "k9s":
		return "k9s"
	}
	if e, _ := catalog.Find(id); e.Group == catalog.Agent {
		return agentTitle(id)
	}
	if kind, ok := catalog.DatabaseKinds[id]; ok {
		return kind
	}
	return toolName(id)
}

// managerTitle names how a method installs, as "mise" or "Homebrew".
func managerTitle(m catalog.Method) string {
	switch m.Manager {
	case "installer":
		return locales.SetupOfficialInstaller()
	case "xcode-select":
		return "Xcode Command Line Tools"
	case "brew":
		return "Homebrew"
	case "winget":
		return "WinGet"
	case "corepack":
		return "Corepack"
	case "choco":
		return "Chocolatey"
	case "scoop":
		return "Scoop"
	}
	if m.Manager == "apt" || m.Manager == "dnf" || m.Manager == "apk" {
		return strings.ToUpper(m.Manager)
	}
	return m.Manager
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
		if a.Shell != nil && a.JournalPath != "" {
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
	for _, e := range a.setupEntries() {
		s.choice[e.ID] = slices.Contains(tools, e.ID)
	}
	s.step, s.diff, s.filter, s.query = 0, "", 0, ""
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

// setupChoiceView is step 1: the languages and tools to switch on beside
// the base, which installs first, and the AI agents. Out of the wizard
// the filter picks the tools there or missing.
func (a *App) setupChoiceView(c *ui.Context) {
	in := a.setupInput()
	wizard := a.setupWizard()
	query := strings.ToLower(strings.TrimSpace(a.setup.query))
	if a.setup.setNotice != "" {
		toolNote(c, a.setup.setNotice)
	}
	type groupData struct {
		title string
		g     catalog.Group
		all   []catalog.Entry
		rows  []catalog.Entry
		have  int
	}
	buildGroup := func(title string, g catalog.Group) groupData {
		all := a.setupOffers(in, g)
		var rows []catalog.Entry
		have := 0
		for _, e := range all {
			h := a.setupHave(e.ID)
			if h {
				have++
			}
			if !wizard && a.setup.filter != 0 && (a.setup.filter == 2) != h {
				continue
			}
			if query != "" && !setupMatches(e, query) {
				continue
			}
			rows = append(rows, e)
		}
		return groupData{title: title, g: g, all: all, rows: rows, have: have}
	}
	leftGroups := []groupData{
		buildGroup(locales.SetupLanguages(), catalog.Language),
		buildGroup(locales.SetupTools(), catalog.Tool),
	}
	rightGroups := []groupData{
		buildGroup(locales.SetupBase(), catalog.Base),
		buildGroup(locales.SetupAgents(), catalog.Agent),
		buildGroup(locales.SetupDatabases(), catalog.Database),
	}
	shown := 0
	for _, gd := range append(slices.Clone(leftGroups), rightGroups...) {
		if len(gd.rows) > 0 {
			shown++
		}
	}
	if shown == 0 {
		if query != "" {
			cardNote(c, locales.NothingMatches())
		} else {
			cardNote(c, locales.InstallNothingHere())
		}
		return
	}
	renderCard := func(gd groupData) {
		if len(gd.rows) == 0 {
			return
		}
		// The base's title is long for its narrow column.
		aside := locales.SetupHaveOf(gd.have, len(gd.all))
		if gd.g == catalog.Base {
			aside = ""
		}
		// Do not collapse when searching, expanded, Base, filtered tab in
		// catalog view, or when only 1-2 items would be hidden.
		collapse := query == "" && !a.setup.more[gd.g] && gd.g != catalog.Base && (wizard || a.setup.filter == 0) && len(gd.rows) > setupFeatured+2
		listCard(c, gd.title, aside, func() {
			var more []string
			for i, e := range gd.rows {
				// The first of a group, what is there, what is chosen,
				// and any search match show; the rest wait behind "N more".
				_, system := a.setupSystemCopy(e.ID)
				if !collapse || i < setupFeatured || a.setupHave(e.ID) || system || a.setupChosen(e.ID) {
					a.setupRow(c, in, e)
				} else {
					more = append(more, setupTitle(e.ID))
				}
			}
			if len(more) > 0 {
				listRow(c, locales.SetupMore(len(more)), false, strings.Join(more, ", "), func() {
					ui.ButtonBase(c.Key("setup-more:" + strconv.Itoa(int(gd.g)))).Label(locales.SetupShowMore()).OnClick(func() {
						if a.setup.more == nil {
							a.setup.more = map[catalog.Group]bool{}
						}
						a.setup.more[gd.g] = true
					}).Children(func() {
						ui.Text(c, locales.SetupShowMore()).Font(FontMono).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
					})
				})
			}
		})
	}
	note := func() {
		if wizard {
			toolNote(c, locales.SetupChoiceNote(len(in.Parts())))
			return
		}
		chosen := a.setupChosenTools()
		if len(chosen) == 0 {
			toolNote(c, locales.InstallNoteNone())
			return
		}
		names := make([]string, len(chosen))
		for i, id := range chosen {
			names[i] = setupTitle(id)
		}
		toolNote(c, locales.InstallNote(len(chosen), strings.Join(names, ", ")))
	}
	columns(c, 12, func() {
		for _, gd := range leftGroups {
			renderCard(gd)
		}
		note()
	}, func() {
		for _, gd := range rightGroups {
			renderCard(gd)
		}
	})
}

func (a *App) setupTargetVersion(id string, m catalog.Method, now time.Time) string {
	switch id {
	case "node":
		if a.NodeVersions != nil {
			if rel, ok := a.NodeVersions.Recommended(); ok && rel.Version != "" {
				return rel.Version + " LTS"
			}
		}
	case "python":
		if v := a.PythonVersions.Newest(now); v != "" {
			return v
		}
	case "go":
		if a.GoVersions != nil && len(a.GoVersions.Releases) > 0 {
			return a.GoVersions.Releases[0]
		}
	case "rust":
		if v := a.RustVersions.Newest(now); v != "" {
			return v
		}
	case "bun":
		if a.BunLatest != nil && a.BunLatest.Version != "" {
			return a.BunLatest.Version
		}
	case "ruby":
		if v := a.RubyVersions.Newest(now); v != "" {
			return v
		}
	case "dotnet":
		if a.DotnetVersions != nil {
			for _, l := range a.DotnetVersions.Lines {
				if l.Version != "" && !l.Ended(now) {
					return l.Version
				}
			}
		}
	case "pnpm":
		if a.JSVersions != nil && a.JSVersions.Pnpm.Version != "" {
			return a.JSVersions.Pnpm.Version
		}
	case "yarn":
		if a.JSVersions != nil && a.JSVersions.Yarn.Version != "" {
			return a.JSVersions.Yarn.Version
		}
	}
	v := ops.SetupTargetVersion(id, m)
	if v == "latest" {
		return locales.SetupVersionLatest()
	}
	return v
}

func setupMatches(e catalog.Entry, q string) bool {
	return strings.Contains(strings.ToLower(setupTitle(e.ID)), q) ||
		strings.Contains(strings.ToLower(e.ID), q) ||
		strings.Contains(strings.ToLower(catalog.About(e.ID)), q)
}

// setupConflict is the tool there or chosen that e does not install
// beside; "" for none.
func (a *App) setupConflict(e catalog.Entry) string {
	for _, id := range e.Conflicts {
		if a.setupHave(id) || a.setupChosen(id) {
			return id
		}
	}
	return ""
}

// setupOffers are the tools of group g the page offers here: those that
// install here, and those there already.
func (a *App) setupOffers(in ops.Setup, g catalog.Group) []catalog.Entry {
	var out []catalog.Entry
	for _, e := range a.setupEntries() {
		if e.Group == g && (a.setupHave(e.ID) || slices.ContainsFunc(e.On(goos), in.Usable)) {
			out = append(out, e)
		}
	}
	return out
}

// setupCounts are how many tools the page offers here, and how many of
// them are there.
func (a *App) setupCounts() (have, total int) {
	in := a.setupInput()
	for _, g := range []catalog.Group{catalog.Base, catalog.Language, catalog.Tool, catalog.Agent, catalog.Database} {
		for _, e := range a.setupOffers(in, g) {
			total++
			if a.setupHave(e.ID) {
				have++
			}
		}
	}
	return have, total
}

// setupChosenTools are the tools switched on that are not there, in the
// catalog's order.
func (a *App) setupChosenTools() []string {
	var out []string
	for _, e := range a.setupEntries() {
		if a.setupChosen(e.ID) && !a.setupHave(e.ID) {
			out = append(out, e.ID)
		}
	}
	return out
}

// setupRow is a tool of the choice: its name, how it installs, and its
// switch; one there already says where it is from and its version, the
// wizard's base that it is required.
func (a *App) setupRow(c *ui.Context, in ops.Setup, e catalog.Entry) {
	t := c.Theme()
	pal := Colors(t)
	var methods []catalog.Method
	for _, m := range e.On(goos) {
		if in.Usable(m) {
			methods = append(methods, m)
		}
	}
	have := a.setupHave(e.ID)
	if len(methods) == 0 && !have {
		return
	}
	about, caption := catalog.About(e.ID), ""
	var m catalog.Method
	if len(methods) > 0 {
		m = methods[0]
		if e.ID == "docker" {
			for _, em := range methods {
				if em.Package == a.setup.engine {
					m = em
				}
			}
		}
		caption = locales.SetupVia(managerTitle(m))
		switch {
		case m.Engine == "Colima":
			about += " " + locales.ColimaWizard()
			caption += " · " + locales.ColimaManaged()
		case e.ID == "docker" && m.Terms != "":
			caption = locales.SetupEngineTerms(m.Engine)
		case e.ID == "docker":
			caption = locales.SetupEngineOpen(m.Engine)
		case e.ID == "rust" && m.Manager == "installer":
			caption = locales.SetupViaRustup()
		}
	}
	version := ""
	target := ""
	if !have && len(methods) > 0 {
		target = a.setupTargetVersion(e.ID, m, c.Now())
	}
	if in, ok := a.setupSystemCopy(e.ID); ok {
		about = locales.SetupSystemCopy(in.Version)
	}
	if have {
		// Where it is from says more than what it is for.
		about, caption = "", ""
		if found, r := a.setupResult(e.ID); r != checkMissing {
			version = found.Version
			caption = strings.TrimPrefix(sourceName(found.Source)+" · "+tilde(found.Path, a.Home), " · ")
			if found.Source == (inventory.Source{}) {
				caption = tilde(found.Path, a.Home)
			}
		}
	}
	ui.Row(c.Key("setup:"+e.ID)).Gap(10).Padding(9, 14).MinHeight(48).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, setupTitle(e.ID)).FontSize(13).SingleLine()
				if target != "" {
					aiTag(c, target, pal.Muted, t.TextMuted)
				}
			})
			// What it is for, to someone new to it, then how it installs.
			if about != "" {
				ui.Text(c, about).FontSize(11.5).TextColor(t.TextMuted)
			}
			if caption != "" {
				clipped(c, caption).FontSize(11.5).TextColor(pal.TextFaint)
			}
		})
		switch {
		case have:
			label := locales.InstalledPlain()
			if version != "" {
				label = locales.InstalledVersion(version)
			}
			chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, label)
		case e.Group == catalog.Base && a.setupWizard():
			ui.Text(c, locales.SetupRequired()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		case a.setupConflict(e) != "":
			ui.Text(c, locales.SetupConflicts(setupTitle(a.setupConflict(e)))).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		default:
			on := a.setupChosen(e.ID)
			if e.ID == "docker" && on && len(methods) > 1 {
				labels := make([]string, len(methods))
				for i, em := range methods {
					labels[i] = em.Engine
				}
				value := m.Engine
				ui.Select(c.Key("setup-engine"), &value, labels).Label(locales.SetupDockerEngine()).Width(160).OnChange(func() {
					a.setup.engine = methods[slices.Index(labels, value)].Package
				})
			}
			if toggle(c, "on", &on).Label(locales.InstallNamed(setupTitle(e.ID))).Changed() {
				if a.setup.choice == nil {
					a.setup.choice = map[string]bool{}
				}
				a.setup.choice[e.ID] = on
			}
		}
	})
}

// setupPlanView is step 2: the order the tools install in, with their
// commands, the files that change and what is worth knowing.
func (a *App) setupPlanView(c *ui.Context) {
	s := &a.setup
	edits := setupEdits(s.plan, s.parts)
	columns(c, 12, func() {
		listCard(c, locales.SetupOrder(), locales.SetupSteps(len(s.parts)), func() {
			for i, part := range s.parts {
				a.setupPartRow(c, i, part)
			}
		})
		// The diff asked for shows under the order, where its lines fit.
		for _, e := range edits {
			if e.edit.Path == s.diff {
				pal := Colors(c.Theme())
				ui.Column(c).Radius(12).Clip().Background(c.Theme().Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
					a.editDiff(c, e.edit, e.setting)
				})
			}
		}
		toolNote(c, locales.SetupStopNote())
	}, func() {
		if len(edits) > 0 {
			listCard(c, locales.SetupFiles(), "", func() {
				for _, e := range edits {
					a.setupFileRow(c, e)
				}
			})
		}
		if len(s.plan.Notes) > 0 {
			listCard(c, locales.OpWorthKnowing(), "", func() {
				for _, n := range s.plan.Notes {
					noteRow(c, n.Title, n.Text, func() {
						if n.URL != "" {
							ui.Button(c.Key("note:"+n.URL), locales.OpOpenLink()+" ↗").OnClick(func() { c.OpenURL(n.URL) })
						}
					})
				}
			})
		}
	})
}

// noteRow is a row of "Worth knowing": a title and the text under it,
// whole, which says more than a caption.
func noteRow(c *ui.Context, title, text string, end func()) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(8).Padding(9, 12, 9, 14).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(13)
			if text != "" {
				ui.Text(c, text).FontSize(11.5).TextColor(pal.TextFaint)
			}
		})
		end()
	})
}

// setupPartRow is a tool in the order of the plan: its number and name,
// what runs, and tags for the password and a file changed; while it
// installs, how its steps went.
func (a *App) setupPartRow(c *ui.Context, i int, part ops.SetupPart) {
	t := c.Theme()
	pal := Colors(t)
	steps := a.setup.plan.Steps[part.From:part.To]
	admin := slices.ContainsFunc(steps, func(s ops.Step) bool { return s.Admin })
	file := slices.ContainsFunc(steps, func(s ops.Step) bool { _, setting, ok := stepEdit(s); return ok && !setting })
	setting := slices.ContainsFunc(steps, func(s ops.Step) bool { _, setting, _ := stepEdit(s); return setting })
	installing := a.setup.step == 2
	status := ops.Waiting
	if installing {
		status = partStatus(a.setup.op.state, part)
	}
	color := t.Text
	if installing && (status == ops.Waiting || status == ops.Skipped) {
		color = t.TextMuted
	}
	row := ui.Row(c.Key("part:"+part.Tool)).Gap(10).Padding(9, 14).MinHeight(48).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	if installing && status == ops.Busy {
		row.Background(pal.AccentSoft)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, strconv.Itoa(i+1)+"  "+setupTitle(part.Tool)).FontSize(13).TextColor(color).SingleLine()
			clipped(c, a.partCaption(part, steps)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
		})
		if installing {
			label, bg, fg := stepTag(c, status)
			aiTag(c, label, bg, fg).Transition(colorFade)
			return
		}
		if admin {
			aiTag(c, locales.SetupTagPassword(), pal.WarningSoft, pal.WarningText)
		}
		if file {
			aiTag(c, locales.SetupTagFile(), pal.Muted, t.TextMuted)
		}
		if setting {
			aiTag(c, locales.SetupTagPath(), pal.Muted, t.TextMuted)
		}
	})
}

// partStatus is how a tool's steps went: as the step that failed,
// stopped or runs, done once all are. A tool with no steps of its own,
// as Git with Homebrew's, is done with the steps before it.
func partStatus(st ops.State, part ops.SetupPart) ops.StepStatus {
	if part.From == part.To {
		switch {
		case !slices.ContainsFunc(st.Steps[:part.From], func(s ops.StepStatus) bool { return s != ops.Done }):
			return ops.Done
		case st.Phase.Over():
			return ops.Skipped
		}
		return ops.Waiting
	}
	steps := st.Steps[part.From:part.To]
	for _, s := range []ops.StepStatus{ops.StepFailed, ops.StepStopped, ops.Busy} {
		if slices.Contains(steps, s) {
			return s
		}
	}
	done := 0
	for _, s := range steps {
		if s == ops.Done {
			done++
		}
	}
	switch {
	case done == len(steps):
		return ops.Done
	case done > 0 && st.Phase == ops.Running:
		// Between two of its steps.
		return ops.Busy
	case slices.Contains(steps, ops.Skipped):
		return ops.Skipped
	}
	return ops.Waiting
}

// setupCurrent is the index of the tool whose step runs, or ran last.
func (a *App) setupCurrent() int {
	i := 0
	for j, part := range a.setup.parts {
		if part.From <= a.setup.op.state.Current && part.To > part.From {
			i = j
		}
	}
	return i
}

// setupInstallView is step 3: the order with how each tool went, the log
// of the step running, and what stays when it stops.
func (a *App) setupInstallView(c *ui.Context) {
	s := &a.setup
	st := s.op.state
	t := c.Theme()
	pal := Colors(t)
	columns(c, 12, func() {
		listCard(c, locales.SetupOrder(), locales.SetupSteps(len(s.parts)), func() {
			for i, part := range s.parts {
				a.setupPartRow(c, i, part)
			}
		})
		toolNote(c, a.setupLeftNote())
	}, func() {
		listCard(c, locales.SetupLog(a.setupCurrent()+1), "", func() {
			color, wrap := t.TextMuted, false
			if st.Phase == ops.Failed || st.Phase == ops.NotVerified {
				color, wrap = pal.DangerText, true
			}
			lines := lastLines(st.Log, 8)
			ui.Column(c).Gap(4).Padding(10, 14, 12, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Font(FontMono).Children(func() {
				// A step that prints nothing yet, as one the administrator
				// runs, says what it does.
				if len(lines) == 0 {
					what := "…"
					if st.Current < len(s.plan.Steps) && !st.Phase.Over() {
						step := s.plan.Steps[st.Current]
						what = cmp.Or(step.Note, ops.Command(step.Shown())) + "…"
					}
					ui.Text(c, what).FontSize(11.5).TextColor(pal.TextFaint)
				}
				for _, line := range lines {
					logLine(c, line, wrap).FontSize(11.5).TextColor(color)
				}
			})
		})
		if st.Phase.Over() {
			return
		}
		listCard(c, locales.OpWorthKnowing(), "", func() {
			var ahead, asked []string
			for i, part := range s.parts {
				if !slices.ContainsFunc(s.plan.Steps[part.From:part.To], func(s ops.Step) bool { return s.Admin }) {
					continue
				}
				if partStatus(st, part) == ops.Done {
					asked = append(asked, strconv.Itoa(i+1))
				} else {
					ahead = append(ahead, strconv.Itoa(i+1)+": "+setupTitle(part.Tool))
				}
			}
			switch {
			case len(ahead) > 0:
				noteRow(c, locales.SetupPasswordAhead(), locales.SetupPasswordAt(len(ahead), strings.Join(ahead, ", ")), func() {})
			case len(asked) > 0:
				noteRow(c, locales.SetupPasswordDone(), locales.SetupPasswordAsked(len(asked), strings.Join(asked, ", ")), func() {})
			}
			noteRow(c, locales.SetupHideTitle(), locales.SetupHideText(), func() {})
		})
	})
}

// setupLeftNote says, under the order, what stays: the tools installed
// already.
func (a *App) setupLeftNote() string {
	s := &a.setup
	var done []string
	for _, part := range s.parts {
		if partStatus(s.op.state, part) == ops.Done {
			done = append(done, setupTitle(part.Tool))
		}
	}
	switch {
	case len(done) == 0 && s.op.state.Phase.Over():
		return locales.SetupNothingChanged()
	case len(done) == 0:
		return locales.SetupNothingYet()
	case s.op.state.Phase.Over():
		return locales.SetupLeftOver(strings.Join(done, ", "))
	}
	return locales.SetupLeftRunning(strings.Join(done, ", "))
}

// checkResult is how a tool installed answers in a new terminal.
type checkResult int

const (
	checkWorks   checkResult = iota
	checkStopped             // Docker's CLI is there, its engine does not answer
	checkDenied              // Docker's engine refuses the user until they sign in again
	checkMissing             // not on the terminal's PATH
)

// setupResult is the copy of tool id a new terminal runs, as the scan
// after the installation found it, reading the terminal's environment
// again, and how it answers.
func (a *App) setupResult(id string) (inventory.Installation, checkResult) {
	if a.Snapshot == nil {
		return inventory.Installation{}, checkMissing
	}
	if id == "mise" {
		m, ok := a.miseManager()
		switch {
		case ok && m.Activated:
			return inventory.Installation{Version: m.Version, Path: locales.SetupMiseActivated()}, checkWorks
		case ok && m.Shims:
			return inventory.Installation{Version: m.Version, Path: locales.SetupMiseShims()}, checkWorks
		}
		return inventory.Installation{}, checkMissing
	}
	if id == "homebrew" {
		for _, m := range a.packageManagers() {
			if m.ID == "brew" {
				// brew --version says "Homebrew 4.6.3".
				return inventory.Installation{Version: strings.TrimPrefix(m.Version, "Homebrew "), Path: m.Path}, checkWorks
			}
		}
		return inventory.Installation{}, checkMissing
	}
	if id == "uv" {
		st, _ := a.Snapshot.Tool("python")
		for _, m := range st.Managers {
			if m.Name == "uv" && m.Version != "" {
				return inventory.Installation{Version: m.Version}, checkWorks
			}
		}
		return inventory.Installation{}, checkMissing
	}
	if id == "gh" {
		st, _ := a.Snapshot.Tool("git")
		if st.Git != nil && st.Git.GitHub != nil {
			return inventory.Installation{Version: st.Git.GitHub.Version, Path: st.Git.GitHub.Path}, checkWorks
		}
		return inventory.Installation{}, checkMissing
	}
	if lang, command, ok := languageCommand(id); ok {
		st, _ := a.Snapshot.Tool(lang)
		if path := st.Vars[command]; path != "" {
			return inventory.Installation{Path: path}, checkWorks
		}
		return inventory.Installation{}, checkMissing
	}
	if kind, ok := catalog.DatabaseKinds[id]; ok {
		db, found := a.databaseServer(kind)
		if !found {
			return inventory.Installation{}, checkMissing
		}
		in := inventory.Installation{Version: db.Version, Path: cmp.Or(db.Path, db.Service)}
		if db.Source == "brew" {
			in.Source = inventory.Source{Manager: "brew"}
		}
		return in, checkWorks
	}
	var ins []inventory.Installation
	var docker *inventory.DockerState
	switch {
	case slices.Contains(kubeCommands, id) || isCLICommand(id) || isCatalogCLITool(id):
		for _, in := range a.cliState().Tools {
			if in.Tool == id {
				ins = append(ins, in)
			}
		}
	default:
		if e, _ := catalog.Find(id); e.Group == catalog.Agent {
			if i := slices.IndexFunc(a.Snapshot.Agents, func(ag inventory.Agent) bool { return ag.ID == id }); i >= 0 {
				ag := a.Snapshot.Agents[i]
				ins = ag.Installations
				// An agent that is an app alone, as Cursor, works as one.
				if len(ins) == 0 && ag.App != "" {
					return inventory.Installation{Version: ag.AppVersion, Path: ag.App}, checkWorks
				}
			}
		} else {
			st, _ := a.Snapshot.Tool(id)
			ins, docker = st.Installations, st.Docker
		}
	}
	i := inventory.TerminalCopy(ins)
	// A shim first on PATH whose version is picked as it runs, as Yarn's
	// through Corepack before its first download, still answers.
	if i < 0 {
		i = slices.IndexFunc(ins, func(in inventory.Installation) bool { return in.OnPath == 1 && in.Source.Shim && !in.Stub })
	}
	if i < 0 {
		return inventory.Installation{}, checkMissing
	}
	if docker != nil {
		for _, ctx := range docker.Contexts {
			if ctx.Name != docker.Selected || ctx.Status != "unavailable" {
				continue
			}
			if ctx.Denied {
				return ins[i], checkDenied
			}
			return ins[i], checkStopped
		}
	}
	return ins[i], checkWorks
}

// setupCheckView is step 4: each tool in a new terminal, what is left to
// do and what changed.
func (a *App) setupCheckView(c *ui.Context) {
	s := &a.setup
	works := 0
	for _, part := range s.parts {
		if _, r := a.setupResult(part.Tool); r == checkWorks {
			works++
		}
	}
	aside := ""
	if !a.Scanning {
		aside = locales.SetupCheckAside(works, len(s.parts))
	}
	columns(c, 12, func() {
		listCard(c, locales.SetupCheckCard(), aside, func() {
			for i, part := range s.parts {
				a.setupCheckRow(c, i, part.Tool)
			}
		})
		if goos != "windows" {
			toolNote(c, locales.SetupAppsNote())
		}
	}, func() {
		if !a.Scanning {
			a.setupNextCard(c)
		}
		var paths []string
		for _, e := range setupEdits(s.plan, s.parts) {
			paths = append(paths, tilde(e.edit.Path, a.Home))
		}
		if len(paths) > 0 {
			listCard(c, locales.SetupChanged(), "", func() {
				caption := locales.SetupChangedCaption()
				if goos == "windows" {
					caption = locales.SetupChangedCaptionWindows()
				}
				listRow(c, strings.Join(paths, ", "), false, caption, func() {})
			})
		}
	})
}

// setupCheckRow is a tool as a new terminal runs it: its version and
// path, and whether it works.
func (a *App) setupCheckRow(c *ui.Context, i int, id string) {
	t := c.Theme()
	pal := Colors(t)
	in, r := a.setupResult(id)
	caption := locales.SetupNotOnPath()
	if r != checkMissing {
		switch path := tilde(in.Path, a.Home); {
		case in.Version != "" && path != "":
			caption = in.Version + " · " + path
		case path != "":
			caption = path
		default:
			caption = cmp.Or(in.Version, "…")
		}
	}
	label, bg, fg := locales.SetupTagWorks(), pal.SuccessSoft, pal.SuccessText
	switch {
	case a.Scanning:
		label, bg, fg, caption = locales.SetupTagChecking(), pal.InfoSoft, pal.InfoText, ""
	case r == checkStopped:
		label, bg, fg = locales.SetupTagEngineStopped(), pal.WarningSoft, pal.WarningText
	case r == checkDenied:
		label, bg, fg = locales.SetupTagSignIn(), pal.WarningSoft, pal.WarningText
	case r == checkMissing:
		label, bg, fg = locales.SetupTagMissing(), pal.DangerSoft, pal.DangerText
	}
	row := ui.Row(c.Key("check:"+id)).Gap(10).Padding(9, 14).MinHeight(48).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
	if !a.Scanning && (r == checkStopped || r == checkDenied) {
		row.Background(pal.WarningSoft)
	}
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, strconv.Itoa(i+1)+"  "+setupTitle(id)).FontSize(13).SingleLine()
			if caption != "" {
				clipped(c, caption).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
			}
		})
		aiTag(c, label, bg, fg)
	})
}

// setupNextCard is what is left to do: start the engine installed, open
// a new terminal, add projects.
func (a *App) setupNextCard(c *ui.Context) {
	link := func(key, label string, onClick func()) {
		ui.ButtonBase(c.Key(key)).Label(label).OnClick(onClick).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(Accent).SingleLine()
		})
	}
	listCard(c, locales.SetupWhatNext(), "", func() {
		docker := slices.ContainsFunc(a.setup.parts, func(p ops.SetupPart) bool { return p.Tool == "docker" })
		if _, r := a.setupResult("docker"); docker && r == checkDenied {
			listRow(c, locales.SetupSignInAgain(), false, locales.SetupSignInAgainText(), func() {})
		}
		if plan, ok := a.engineStartPlan(); ok && docker {
			listRow(c, plan.Title, false, locales.SetupEngineNeeded(), func() {
				link("setup-engine-start", locales.SetupStart(), func() { a.Propose(plan) })
			})
		}
		listRow(c, locales.SetupNewTerminal(), false, locales.SetupNewTerminalText(), func() {})
		if len(a.Projects) == 0 && a.ChooseFolder != nil {
			listRow(c, locales.SetupAddProjects(), false, locales.SetupAddProjectsText(), func() {
				link("setup-projects", locales.SetupAdd(), func() { a.router.Push(projectsPage.path) })
			})
		}
	})
}

// partCaption says what a tool's steps run: the first command, or what
// a script does, and the files it adds a line to.
func (a *App) partCaption(part ops.SetupPart, steps []ops.Step) string {
	switch {
	case part.With == "homebrew":
		return locales.SetupWithHomebrew()
	case part.With != "":
		return locales.SetupWithPart(setupTitle(part.With))
	}
	var what string
	var files []string
	for _, s := range steps {
		if e, _, ok := stepEdit(s); ok {
			files = append(files, tilde(e.Path, a.Home))
			continue
		}
		if what != "" {
			continue
		}
		argv := s.Shown()
		if s.Note != "" && (argv[0] == "/bin/sh" || s.Admin) {
			what = s.Note
			continue
		}
		what = strings.Join(append([]string{strings.TrimSuffix(filepath.Base(argv[0]), ".exe")}, argv[1:]...), " ")
	}
	switch {
	case len(files) == 0:
		return what
	case what == "":
		return locales.SetupLineIn(strings.Join(files, ", "))
	}
	return what + " · " + locales.SetupLineIn(strings.Join(files, ", "))
}

// setupEdit is a file the plan changes: from what it is to what all the
// steps that change it leave, and the tools whose steps those are.
type setupEdit struct {
	edit    ops.FileEdit
	setting bool
	tools   []string
}

// setupEdits are the files the plan changes, in the order it first
// changes them.
func setupEdits(p ops.Plan, parts []ops.SetupPart) []setupEdit {
	var out []setupEdit
	for _, part := range parts {
		for _, step := range p.Steps[part.From:part.To] {
			e, setting, ok := stepEdit(step)
			if !ok {
				continue
			}
			i := slices.IndexFunc(out, func(se setupEdit) bool { return se.edit.Path == e.Path })
			if i < 0 {
				out = append(out, setupEdit{edit: e, setting: setting})
				i = len(out) - 1
			}
			out[i].edit.After = e.After
			if !slices.Contains(out[i].tools, part.Tool) {
				out[i].tools = append(out[i].tools, part.Tool)
			}
		}
	}
	return out
}

// setupFileRow is a file the plan changes, with a button that shows its
// diff under the order.
func (a *App) setupFileRow(c *ui.Context, e setupEdit) {
	names := make([]string, len(e.tools))
	for i, id := range e.tools {
		names[i] = setupTitle(id)
	}
	shown := a.setup.diff == e.edit.Path
	listRow(c, tilde(e.edit.Path, a.Home), false, locales.SetupFileCaption(strings.Join(names, ", ")), func() {
		label := locales.SetupShowDiff()
		if shown {
			label = locales.SetupHideDiff()
		}
		ui.ButtonBase(c.Key("diff:" + e.edit.Path)).Label(label).OnClick(func() {
			if shown {
				a.setup.diff = ""
			} else {
				a.setup.diff = e.edit.Path
			}
		}).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(Accent).SingleLine()
		})
	})
}
