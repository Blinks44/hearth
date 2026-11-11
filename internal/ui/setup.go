package ui

import (
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// setupPage is the new computer wizard's path.
const setupPage = "/setup"

// goos is the system the wizard sets up; tests change it.
var goos = runtime.GOOS

// setupState is where the new computer wizard is.
type setupState struct {
	step int // 0 the choice, 1 the plan
	// tab is the stepper's segment, which goes back to the choice; shown
	// the step it showed, which tells the user's click from a new step.
	tab, shown int
	// choice is what the user switched on or off, by the catalog's ID;
	// the others are as setupOffered says.
	choice map[string]bool
	engine string // the Docker engine's package chosen; "" for the first
	// plan and parts are the plan made of the choice for step 2;
	// building reports it is being made.
	plan     ops.Plan
	parts    []ops.SetupPart
	building bool
	diff     string // the file whose diff shows on the plan; "" for none
}

// setupOffered are the tools switched on until the user says otherwise.
var setupOffered = []string{"node", "python", "pnpm", "git", "docker", "claude"}

// setupChosen reports that the tool id is switched on.
func (a *App) setupChosen(id string) bool {
	if on, ok := a.setup.choice[id]; ok {
		return on
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
		if a.version("mise") == "" || a.Snapshot == nil {
			return false
		}
		for _, st := range a.Snapshot.Tools {
			for _, m := range st.Managers {
				if m.Name == "mise" && (m.Activated || m.Shims) {
					return true
				}
			}
		}
		return false
	}
	if e, _ := catalog.Find(id); e.Group == catalog.Agent {
		return slices.Contains(a.foundIDs(), id)
	}
	return a.version(id) != ""
}

// packageManagers are the system's package managers the last scan found.
func (a *App) packageManagers() []inventory.PackageManager {
	if a.Snapshot == nil {
		return nil
	}
	st, _ := a.Snapshot.Tool("packages")
	return st.PackageManagers
}

// setupInput is what the wizard installs, as chosen: the base always.
func (a *App) setupInput() ops.Setup {
	s := ops.Setup{GOOS: goos, Managers: a.packageManagers(), Engine: a.setup.engine, Have: map[string]bool{}, Title: setupTitle}
	for _, e := range a.setupEntries() {
		if a.setupHave(e.ID) {
			s.Have[e.ID] = true
		}
		if e.Group == catalog.Base || a.setupChosen(e.ID) {
			s.Tools = append(s.Tools, e.ID)
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
	}
	if e, _ := catalog.Find(id); e.Group == catalog.Agent {
		return agentTitle(id)
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
	// The stepper goes back to the choice alone.
	if s.tab != s.shown && s.tab < s.step {
		s.step, s.diff = s.tab, ""
	}
	s.tab, s.shown = s.step, s.step
	ui.Column(c).Fill().Children(func() {
		t := c.Theme()
		pal := Colors(t)
		subtitle := locales.SetupStepChoice()
		if s.step == 1 {
			subtitle = locales.SetupStepPlan()
		}
		a.headerWith(c, locales.SetupTitle(), subtitle, headerExtras{after: func() { a.setupButtons(c) }})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(48, 48).Shrink(0).Radius(12).Background(pal.AccentSoft).Center().Children(func() {
						ui.Icon(c, Icon("sparkles")).Size(26, 26).TextColor(Accent)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						title, lead := locales.SetupChoiceTitle(), locales.SetupChoiceLead()
						if s.step == 1 {
							title, lead = locales.SetupPlanHeading(), locales.SetupPlanLead()
						}
						ui.Text(c, title).FontSize(22).FontWeight(600)
						ui.Text(c, lead).FontSize(12).TextColor(t.TextMuted)
					})
				})
				ui.Row(c).Children(func() {
					segments(c, "setup-steps", &s.tab, []segment{
						{"1 " + locales.SetupSegChoice(), ""}, {"2 " + locales.SetupSegPlan(), ""},
						{"3 " + locales.SetupSegInstall(), ""}, {"4 " + locales.SetupSegCheck(), ""},
					})
				})
				if s.step == 1 {
					a.setupPlanView(c)
					return
				}
				a.setupChoiceView(c)
			})
		})
	})
}

// setupButtons are the header's buttons: cancel and go on to the plan on
// the choice; back and install on the plan.
func (a *App) setupButtons(c *ui.Context) {
	s := &a.setup
	if s.step == 1 {
		ui.Button(c.Key("setup-back"), locales.SetupBack()).OnClick(func() { s.step, s.diff = 0, "" })
		ui.PrimaryButton(c.Key("setup-install"), s.plan.Action).Disabled(a.op.running()).OnClick(a.setupInstall)
		return
	}
	ui.Button(c.Key("setup-cancel"), locales.SetupCancel()).OnClick(func() { a.router.Push(overview.path) })
	label := locales.SetupNext()
	if s.building {
		label = locales.SetupBuilding()
	}
	ui.PrimaryButton(c.Key("setup-next"), label).
		Disabled(s.building || a.BuildPlan == nil || len(a.setupInput().Parts()) == 0).OnClick(a.setupNext)
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

// setupInstall runs the plan the user read on step 2, in the operation's
// dialog.
func (a *App) setupInstall() {
	a.Propose(a.setup.plan)
	if a.RunPlan != nil && !a.op.running() {
		a.startOperation()
	}
}

// setupChoiceView is step 1: the languages and tools to switch on beside
// the base, which installs first, and the AI agents.
func (a *App) setupChoiceView(c *ui.Context) {
	in := a.setupInput()
	entries := a.setupEntries()
	group := func(g catalog.Group) []catalog.Entry {
		return slices.DeleteFunc(slices.Clone(entries), func(e catalog.Entry) bool { return e.Group != g })
	}
	columns(c, 12, func() {
		languages := group(catalog.Language)
		have := 0
		for _, e := range languages {
			if a.setupHave(e.ID) {
				have++
			}
		}
		aside := locales.SetupHaveNone()
		if have > 0 {
			aside = locales.SetupHaveCount(have)
		}
		listCard(c, locales.SetupLanguages(), aside, func() {
			for _, e := range languages {
				a.setupRow(c, in, e)
			}
		})
		listCard(c, locales.SetupTools(), "", func() {
			for _, e := range group(catalog.Tool) {
				a.setupRow(c, in, e)
			}
		})
		toolNote(c, locales.SetupChoiceNote(len(in.Parts())))
	}, func() {
		listCard(c, locales.SetupBase(), "", func() {
			for _, e := range group(catalog.Base) {
				a.setupRow(c, in, e)
			}
		})
		listCard(c, locales.SetupAgents(), "", func() {
			for _, e := range group(catalog.Agent) {
				a.setupRow(c, in, e)
			}
		})
	})
}

// setupRow is a tool of the choice: its name, how it installs, and its
// switch; what is there already or the base says so instead.
func (a *App) setupRow(c *ui.Context, in ops.Setup, e catalog.Entry) {
	pal := Colors(c.Theme())
	var methods []catalog.Method
	for _, m := range e.On(goos) {
		if in.Usable(m) {
			methods = append(methods, m)
		}
	}
	if len(methods) == 0 {
		return
	}
	m := methods[0]
	if e.ID == "docker" {
		for _, em := range methods {
			if em.Package == a.setup.engine {
				m = em
			}
		}
	}
	caption := locales.SetupVia(managerTitle(m))
	switch {
	case e.ID == "docker" && m.Terms != "":
		caption = locales.SetupEngineTerms(ops.EngineName(m.Package))
	case e.ID == "docker":
		caption = locales.SetupEngineOpen(ops.EngineName(m.Package))
	case e.ID == "rust" && m.Manager == "installer":
		caption = locales.SetupViaRustup()
	}
	have := a.setupHave(e.ID)
	ui.Row(c.Key("setup:"+e.ID)).Gap(10).Padding(9, 14).MinHeight(48).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, setupTitle(e.ID)).FontSize(13).SingleLine().Tooltip(catalog.About(e.ID))
			ui.Text(c, caption).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
		})
		switch {
		case have:
			ui.Text(c, locales.SetupHave()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		case e.Group == catalog.Base:
			ui.Text(c, locales.SetupRequired()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
		default:
			on := a.setupChosen(e.ID)
			if e.ID == "docker" && on && len(methods) > 1 {
				labels := make([]string, len(methods))
				for i, em := range methods {
					labels[i] = ops.EngineName(em.Package)
				}
				value := ops.EngineName(m.Package)
				ui.Select(c.Key("setup-engine"), &value, labels).Label(locales.SetupDockerEngine()).Width(150).OnChange(func() {
					a.setup.engine = methods[slices.Index(labels, value)].Package
				})
			}
			if ui.Switch(c.Key("on"), &on).Label(locales.InstallNamed(setupTitle(e.ID))).Changed() {
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
					listRow(c, n.Title, false, n.Text, func() {
						if n.URL != "" {
							ui.Button(c.Key("note:"+n.URL), locales.OpOpenLink()+" ↗").OnClick(func() { c.OpenURL(n.URL) })
						}
					})
				}
			})
		}
	})
}

// setupPartRow is a tool in the order of the plan: its number and name,
// what runs, and tags for the password and a file changed.
func (a *App) setupPartRow(c *ui.Context, i int, part ops.SetupPart) {
	pal := Colors(c.Theme())
	steps := a.setup.plan.Steps[part.From:part.To]
	admin := slices.ContainsFunc(steps, func(s ops.Step) bool { return s.Admin })
	file := slices.ContainsFunc(steps, func(s ops.Step) bool { _, _, ok := stepEdit(s); return ok })
	ui.Row(c.Key("part:"+part.Tool)).Gap(10).Padding(9, 14).MinHeight(48).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, strconv.Itoa(i+1)+"  "+setupTitle(part.Tool)).FontSize(13).SingleLine()
			clipped(c, a.partCaption(steps)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
		})
		if admin {
			aiTag(c, locales.SetupTagPassword(), pal.WarningSoft, pal.WarningText)
		}
		if file {
			aiTag(c, locales.SetupTagFile(), pal.Muted, c.Theme().TextMuted)
		}
	})
}

// partCaption says what a tool's steps run: the first command, or what
// a script does, and the files it adds a line to.
func (a *App) partCaption(steps []ops.Step) string {
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
	case what == "" && len(files) == 0:
		return locales.SetupWithHomebrew()
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
