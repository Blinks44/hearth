package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

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
