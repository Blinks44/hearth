package ui

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

const uvProjectGuideURL = "https://docs.astral.sh/uv/guides/projects/"

// preparing belongs to one request. Workers publish only through BuildPlan's
// UI callback; replacing the request cancels it and rejects its stale result.
type preparing struct {
	dir    string
	open   bool
	cancel context.CancelFunc
}

type projectOutcomeKind int

const (
	outcomePlan projectOutcomeKind = iota
	outcomeRunning
	outcomeChecking
	outcomeReady
	outcomeNeedsAction
	outcomeAlreadyInstalled
	outcomeConflict
	outcomeUnknown
	outcomeReadErr
	outcomeNoRequirements
	outcomeFailed
	outcomeStopped
	outcomeUnverified
)

type statusTone int

const (
	statusOK statusTone = iota
	statusWarn
	statusError
	statusSkipped
	statusFile
	statusBusy
)

type statusRowAction struct {
	key     string
	label   string
	onClick func()
}

type statusRow struct {
	key     string
	tone    statusTone
	title   string
	detail  string
	mono    string
	actions []statusRowAction
}

// CancelPreparation ends a pending read when the window closes.
func (a *App) CancelPreparation() {
	if a.preparing != nil {
		a.preparing.cancel()
		a.preparing = nil
	}
}

func (a *App) prepareProject(dir string) {
	if a.op.running() {
		a.op.open = true
		return
	}
	if a.BuildPlan == nil || a.Snapshot == nil {
		return
	}
	a.CancelPreparation()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	request := &preparing{dir: dir, open: true, cancel: cancel}
	a.preparing = request
	// Capture immutable data now. No view state is read from the worker.
	snapshot := *a.Snapshot
	input := ops.ProjectInput{Snapshot: &snapshot, Releases: a.projectReleases(), Lines: a.nodeLines(), Setup: a.setupHere(), Title: toolTitle, Detect: a.DetectTool}
	var requestResult ops.ProjectPreparation
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		defer cancel()
		input.Project = projects.Read(dir)
		if len(input.Project.Errs) > 0 {
			return ops.Plan{}, input.Project.Errs[0]
		}
		if input.Detect != nil {
			var tools []string
			for _, req := range input.Project.Requirements {
				if !slices.Contains(tools, req.Tool) {
					tools = append(tools, req.Tool)
				}
			}
			// Never mutate the snapshot owned by the UI.
			snapshot.Tools = slices.Clone(snapshot.Tools)
			for _, tool := range tools {
				if ctx.Err() != nil {
					return ops.Plan{}, ctx.Err()
				}
				st, err := input.Detect(ctx, tool)
				st.Tool = tool
				if err != nil {
					st.Err = err.Error()
				}
				i := slices.IndexFunc(snapshot.Tools, func(s inventory.ToolState) bool { return s.Tool == tool })
				if i < 0 {
					snapshot.Tools = append(snapshot.Tools, st)
				} else {
					snapshot.Tools[i] = st
				}
				input.Setup.Have[tool] = len(st.Installations) > 0 && st.Err == ""
			}
		}
		result, err := ops.PrepareProject(input, env)
		if ctx.Err() != nil {
			return ops.Plan{}, ctx.Err()
		}
		// The result is delivered by the completion callback, before the dialog
		// becomes visible; no application state crosses the worker boundary.
		requestResult = result
		return result.Plan, err
	}, func(p ops.Plan, err error) {
		if a.preparing != request {
			return
		}
		a.preparing = nil
		if !request.open {
			return
		}
		if slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == dir }) < 0 {
			a.planErr = locales.PrepareMissingProject()
			return
		}
		if err != nil {
			// Keep the folder and retry actions available even when parsing fails.
			p = ops.Plan{Title: locales.PrepareTitle(filepath.Base(dir)), Why: locales.PrepareReadErrors()}
			a.Propose(p)
			a.op.project = &ops.ProjectPreparation{
				Project: projects.Project{Dir: dir},
				ReadErr: err.Error(),
				Items:   []ops.ProjectItem{{Title: locales.PrepareReadErrors(), Text: err.Error(), Kind: ops.ProjectUnknown}},
			}
			return
		}
		a.Propose(p)
		a.op.project = &requestResult
	})
}

func (a *App) projectReleases() map[string][]string {
	out := map[string][]string{}
	if a.NodeVersions != nil {
		for _, r := range a.NodeVersions.Releases {
			out["node"] = append(out["node"], r.Version)
		}
	}
	if a.GoVersions != nil {
		out["go"] = slices.Clone(a.GoVersions.Releases)
	}
	for _, c := range []struct {
		tool   string
		cycles *versions.Cycles
	}{{"python", a.PythonVersions}, {"php", a.PHPVersions}, {"ruby", a.RubyVersions}, {"java", a.JavaVersions}, {"rust", a.RustVersions}} {
		if c.cycles != nil {
			for _, l := range c.cycles.Lines {
				out[c.tool] = append(out[c.tool], l.Latest)
			}
		}
	}
	if a.BunLatest != nil {
		out["bun"] = []string{a.BunLatest.Version}
	}
	if a.JSVersions != nil {
		out["npm"] = []string{a.JSVersions.Npm.Version}
		out["pnpm"] = []string{a.JSVersions.Pnpm.Version}
		out["yarn"] = []string{a.JSVersions.Yarn.Version}
	}
	if a.DotnetVersions != nil {
		for _, l := range a.DotnetVersions.Lines {
			if l.Version != "" {
				out["dotnet"] = append(out["dotnet"], l.Version)
			}
		}
	}
	return out
}

func (a *App) projectPostAssessment(op *operation) *ops.ProjectPreparation {
	if op.project == nil || !op.started || !op.state.Phase.Over() || op.rescanning(a) || !op.hasFreshSnapshot(a) {
		return nil
	}
	proj := op.project.Project
	if i := slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == proj.Dir }); i >= 0 {
		proj = a.Projects[i]
	}
	assessed := ops.AssessProject(ops.ProjectInput{
		Project:  proj,
		Snapshot: a.Snapshot,
		Releases: a.projectReleases(),
		Lines:    a.nodeLines(),
		Title:    toolTitle,
	})
	return &assessed
}

func (a *App) projectOutcome(op *operation) (projectOutcomeKind, *ops.ProjectPreparation) {
	if op.project == nil {
		return outcomePlan, nil
	}
	if !op.started {
		if len(op.plan.Steps) > 0 {
			return outcomePlan, op.project
		}
		if op.project.ReadErr != "" {
			return outcomeReadErr, op.project
		}
		if len(op.project.Items) == 0 {
			return outcomeNoRequirements, op.project
		}
		return classifyItemsOutcome(op.project.Items, false), op.project
	}
	if !op.state.Phase.Over() {
		return outcomeRunning, op.project
	}
	post := a.projectPostAssessment(op)
	switch op.state.Phase {
	case ops.NotVerified:
		return outcomeUnverified, post
	case ops.Failed:
		return outcomeFailed, post
	case ops.Stopped:
		return outcomeStopped, post
	case ops.Succeeded:
		if op.rescanning(a) {
			return outcomeChecking, nil
		}
		if post == nil {
			// Successful commands alone never prove project readiness.
			return outcomeUnverified, nil
		}
		if len(post.Items) == 0 {
			return outcomeNoRequirements, post
		}
		return classifyItemsOutcome(post.Items, true), post
	}
	return outcomeUnverified, post
}

func classifyItemsOutcome(items []ops.ProjectItem, afterRun bool) projectOutcomeKind {
	allReady := len(items) > 0
	hasConflict := false
	hasUnknown := false
	hasNeedsSelection := false
	for _, item := range items {
		switch item.Kind {
		case ops.ProjectReady:
		case ops.ProjectConflict:
			allReady, hasConflict = false, true
		case ops.ProjectUnknown, ops.ProjectUnchecked:
			allReady, hasUnknown = false, true
		case ops.ProjectNeedsSelection:
			allReady, hasNeedsSelection = false, true
		default:
			allReady = false
		}
	}
	if allReady {
		return outcomeReady
	}
	if afterRun {
		return outcomeNeedsAction
	}
	switch {
	case hasConflict:
		return outcomeConflict
	case hasUnknown:
		return outcomeUnknown
	case hasNeedsSelection:
		return outcomeAlreadyInstalled
	default:
		return outcomeNeedsAction
	}
}

func (a *App) projectTitle(op *operation) string {
	outcome, _ := a.projectOutcome(op)
	switch outcome {
	case outcomeReady:
		return locales.PrepareResultReadyTitle()
	case outcomeNeedsAction:
		if op.started {
			return locales.PrepareResultActionsTitle()
		}
		return op.plan.Title
	case outcomeAlreadyInstalled:
		return locales.PrepareAlreadyInstalledTitle()
	case outcomeConflict:
		return locales.PrepareConflictTitle()
	case outcomeUnknown:
		return locales.PrepareUnknownTitle()
	case outcomeChecking:
		return locales.PrepareResultCheckingTitle()
	case outcomeUnverified:
		return locales.PrepareResultUnverifiedTitle()
	case outcomeStopped:
		return locales.PrepareResultStoppedTitle()
	case outcomeFailed:
		if slices.Contains(op.state.Steps, ops.Done) {
			return locales.PrepareResultPartialTitle()
		}
		return locales.PrepareResultFailedTitle()
	default:
		return op.plan.Title
	}
}

func (a *App) projectCaption(op *operation) string {
	name := filepath.Base(op.project.Project.Dir)
	outcome, _ := a.projectOutcome(op)
	switch outcome {
	case outcomePlan:
		return op.plan.Why
	case outcomeReady:
		return locales.PrepareResultReadySubtitle(name)
	case outcomeNeedsAction:
		if op.started {
			return locales.PrepareResultActionsSubtitle(name)
		}
		return op.plan.Why
	case outcomeAlreadyInstalled:
		return locales.PrepareAlreadyInstalledSubtitle(name)
	case outcomeConflict:
		return locales.PrepareConflictSubtitle(name)
	case outcomeUnknown:
		return locales.PrepareUnknownSubtitle(name)
	case outcomeReadErr:
		return locales.PrepareReadErrors()
	case outcomeNoRequirements:
		return locales.PrepareNoRequirements()
	case outcomeChecking:
		return locales.PrepareResultCheckingSubtitle(name)
	case outcomeUnverified:
		if op.state.Err != "" {
			return name + " · " + op.state.Err
		}
		return locales.PrepareResultUnverifiedSubtitle(name)
	case outcomeFailed:
		n := op.state.Current + 1
		if op.state.FileChanged {
			return locales.OpFileChanged()
		}
		return name + " · " + locales.OpFailed(n, failReason(op.state))
	case outcomeStopped:
		n := op.state.Current + 1
		if !slices.Contains(op.state.Steps, ops.StepStopped) {
			return name + " · " + locales.OpCheckStopped()
		}
		return name + " · " + locales.OpStopped(n)
	default:
		return op.plan.Why
	}
}

func (a *App) preparationView(c *ui.Context) {
	request := a.preparing
	if request == nil {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	dialogBase(c.Key("preparing"), &request.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(440).Radius(12).Clip().Background(t.Background).
			BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Children(func() {
			ui.Column(c).Padding(20, 20, 16, 20).Gap(10).Children(func() {
				ui.Text(c, locales.PrepareTitle(filepath.Base(request.dir))).FontSize(16).FontWeight(600).Wrap()
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					spinIcon(c, "refresh-cw", 14, t.Accent, true)
					ui.Text(c, locales.PrepareChecking()).FontSize(12.5).TextColor(t.TextMuted).Grow(1).MinWidth(0).Wrap()
				})
			})
			ui.Row(c).Padding(12, 20).Justify(ui.End).Background(pal.Subtle).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Button(c.Key("prepare-cancel"), locales.Cancel()).Height(controlHeight).
					OnClick(func() { request.open = false; a.CancelPreparation() })
			})
		})
	})
	if !request.open {
		a.CancelPreparation()
	}
}

func (a *App) projectPreparationItems(c *ui.Context, p *ops.ProjectPreparation, before bool) {
	dialogSection(c, locales.PrepareItems())
	if before {
		ui.Text(c, locales.PrepareBefore()).FontSize(11.5).TextColor(c.Theme().TextMuted).Padding(4, 20, 8, 20)
	}
	if len(p.Items) == 0 {
		ui.Text(c, locales.PrepareNoRequirements()).FontSize(12).Padding(10, 20)
		return
	}
	rows := a.itemStatusRows(c, p, before)
	statusListCard(c, "", rows)
}

func (a *App) projectNoPlanBody(c *ui.Context, op *operation) {
	outcome, p := a.projectOutcome(op)
	if p == nil {
		return
	}
	switch outcome {
	case outcomeReadErr:
		dialogSection(c, strings.ToUpper(locales.PrepareResultSection()))
		statusListCard(c, "", []statusRow{{
			key:    "read-err",
			tone:   statusError,
			title:  locales.PrepareReadErrors(),
			detail: p.ReadErr,
		}})
		return
	case outcomeNoRequirements:
		dialogSection(c, locales.PrepareItems())
		statusListCard(c, "", []statusRow{{
			key:   "no-req",
			tone:  statusWarn,
			title: locales.PrepareNoRequirements(),
		}})
		return
	case outcomeReady:
		a.projectReadyBody(c, p, false)
		return
	}
	// Source files with direct Open buttons, then item statuses, then Worth Knowing.
	if srcRows := a.projectSourceRows(c, p); len(srcRows) > 0 {
		dialogSection(c, strings.ToUpper(locales.PrepareSourcesSection()))
		statusListCard(c, "", srcRows)
	}
	dialogSection(c, locales.PrepareItems())
	statusListCard(c, "", a.itemStatusRows(c, p, false))
	var notes []ops.Note
	switch outcome {
	case outcomeConflict:
		notes = append(notes, ops.Note{Title: locales.PrepareConflictNoteTitle(), Text: locales.PrepareConflictNoteText()})
	case outcomeAlreadyInstalled:
		notes = append(notes, a.selectionSummaryNotes(p.Items)...)
		notes = append(notes, ops.Note{Title: locales.PrepareGlobalUnchangedTitle(), Text: locales.PrepareGlobalUnchangedText()})
	default:
		notes = append(notes, a.selectionSummaryNotes(p.Items)...)
	}
	notes = append(notes, ops.Note{Title: locales.PrepareDepsNoteTitle(), Text: locales.PrepareDepsNoteText()})
	worthKnowingList(c, notes)
}

func (a *App) projectResultView(c *ui.Context, op *operation) {
	outcome, post := a.projectOutcome(op)
	s := op.state
	pal := Colors(c.Theme())
	switch outcome {
	case outcomeChecking:
		dialogSection(c, strings.ToUpper(locales.PrepareResultSection()))
		rows := a.stepStatusRows(op.plan.Steps, s)
		rows = append(rows, statusRow{
			key:   "rescan-busy",
			tone:  statusBusy,
			title: locales.PrepareResultCheckingTitle(),
		})
		statusListCard(c, locales.PrepareCommandsNotReadiness(), rows)
	case outcomeReady:
		a.projectReadyBody(c, post, true)
	case outcomeNeedsAction:
		dialogSection(c, strings.ToUpper(locales.PrepareResultSection()))
		statusListCard(c, locales.PrepareConfirmedCaption(), a.itemStatusRows(c, post, false))
		notes := a.selectionSummaryNotes(post.Items)
		notes = append(notes,
			ops.Note{Title: locales.PrepareTerminalNoteTitle(), Text: locales.PrepareTerminalNoteText()},
			ops.Note{Title: locales.PrepareDepsNoteTitle(), Text: locales.PrepareDepsNoteText()},
		)
		worthKnowingList(c, notes)
	case outcomeNoRequirements:
		dialogSection(c, strings.ToUpper(locales.PrepareResultSection()))
		statusListCard(c, locales.PrepareConfirmedCaption(), []statusRow{{
			key:   "no-req-post",
			tone:  statusWarn,
			title: locales.PrepareNoRequirements(),
		}})
	case outcomeUnverified:
		dialogSection(c, strings.ToUpper(locales.PrepareResultSection()))
		rows := a.stepStatusRows(op.plan.Steps, s)
		if s.Err != "" {
			rows = append(rows, statusRow{
				key:   "verify-err",
				tone:  statusError,
				title: s.Err,
			})
		}
		statusListCard(c, locales.PrepareCommandsNotReadiness(), rows)
		if len(s.Log) > 0 {
			dialogSection(c, locales.OpLastLines())
			logBlock(c, lastLines(s.Log, 5), pal.DangerText, locales.OpFullLog(), true)
		}
		notes := []ops.Note{
			{Title: locales.PrepareUnverifiedNoteTitle(), Text: locales.PrepareUnverifiedNoteText()},
		}
		if len(s.Restored)+len(s.Undone) > 0 {
			notes = append(notes, ops.Note{Title: locales.OpLeft(), Text: strings.Join(a.restored(s), "\n")})
		}
		notes = append(notes, ops.Note{Title: locales.PrepareDepsNoteTitle(), Text: locales.PrepareDepsNoteText()})
		worthKnowingList(c, notes)
	case outcomeFailed:
		dialogSection(c, strings.ToUpper(locales.PrepareResultSection()))
		rows := a.stepStatusRows(op.plan.Steps, s)
		if post != nil {
			rows = append(rows, a.itemStatusRows(c, post, false)...)
		} else {
			rows = append(rows, a.itemToolShortcutRows(c, op.project)...)
		}
		statusListCard(c, "", rows)
		if len(s.Log) > 0 {
			dialogSection(c, locales.OpLastLines())
			logBlock(c, lastLines(s.Log, 5), pal.DangerText, locales.OpFullLog(), true)
		}
		var notes []ops.Note
		if slices.Contains(s.Steps, ops.Done) {
			notes = append(notes, ops.Note{Title: locales.PrepareKeepInstalledNoteTitle(), Text: locales.PrepareKeepInstalledNoteText()})
		}
		if len(s.Restored)+len(s.Undone) > 0 {
			notes = append(notes, ops.Note{Title: locales.OpLeft(), Text: strings.Join(a.restored(s), "\n")})
		}
		notes = append(notes, ops.Note{Title: locales.PrepareContinueNoteTitle(), Text: locales.PrepareContinueNoteText()})
		worthKnowingList(c, notes)
	case outcomeStopped:
		dialogSection(c, strings.ToUpper(locales.PrepareResultSection()))
		rows := a.stepStatusRows(op.plan.Steps, s)
		if post != nil {
			rows = append(rows, a.itemStatusRows(c, post, false)...)
		}
		statusListCard(c, "", rows)
		var notes []ops.Note
		if slices.Contains(s.Steps, ops.Done) {
			notes = append(notes, ops.Note{Title: locales.PrepareKeepInstalledNoteTitle(), Text: locales.PrepareKeepInstalledNoteText()})
		}
		if len(s.Restored)+len(s.Undone) > 0 {
			notes = append(notes, ops.Note{Title: locales.OpLeft(), Text: strings.Join(a.restored(s), "\n")})
		}
		notes = append(notes, ops.Note{Title: locales.PrepareContinueNoteTitle(), Text: locales.PrepareContinueNoteText()})
		worthKnowingList(c, notes)
	}
}

func (a *App) projectReadyBody(c *ui.Context, p *ops.ProjectPreparation, afterRun bool) {
	dialogSection(c, strings.ToUpper(locales.PrepareConfirmedSection()))
	caption := ""
	if afterRun {
		caption = locales.PrepareConfirmedCaption()
	}
	statusListCard(c, caption, a.itemStatusRows(c, p, false))
	notes := []ops.Note{
		{Title: locales.PrepareTerminalNoteTitle(), Text: locales.PrepareTerminalNoteText()},
		{Title: locales.PrepareDepsNoteTitle(), Text: locales.PrepareDepsNoteText()},
	}
	worthKnowingList(c, notes)
}

func (a *App) projectSourceRows(c *ui.Context, p *ops.ProjectPreparation) []statusRow {
	var rows []statusRow
	seen := map[string]bool{}
	for _, item := range p.Items {
		for i, src := range item.Sources {
			key := src.File + "|" + src.Key + "|" + src.Want
			if seen[key] {
				continue
			}
			seen[key] = true
			title := src.File
			if src.Key != "" {
				title += " · " + src.Key
			}
			if src.Want != "" {
				title += " → " + src.Want
			}
			var actions []statusRowAction
			if a.Shell != nil {
				file := src.File
				actions = append(actions, statusRowAction{
					key:     "source-open:" + item.Tool + ":" + file + ":" + strconv.Itoa(i),
					label:   locales.PrepareOpenSource(),
					onClick: func() { a.openFile(c, filepath.Join(p.Project.Dir, file)) },
				})
			}
			rows = append(rows, statusRow{
				key:     "source:" + item.Tool + ":" + fileKey(src, i),
				tone:    statusFile,
				title:   title,
				detail:  src.Err,
				actions: actions,
			})
		}
	}
	return rows
}

func fileKey(src ops.ProjectSource, i int) string {
	return src.File + ":" + src.Key + ":" + strconv.Itoa(i)
}

func (a *App) itemStatusRows(c *ui.Context, p *ops.ProjectPreparation, before bool) []statusRow {
	rows := make([]statusRow, 0, len(p.Items))
	for i, item := range p.Items {
		tone := itemStatusTone(item, before)
		title := itemStatusTitle(item)
		var detailParts []string
		if src := ops.ProjectSources(p.Project, item.Tool); src != "" {
			detailParts = append(detailParts, src)
		}
		if item.Kind == ops.ProjectNeedsSelection {
			if item.Selected != "" {
				detailParts = append(detailParts, locales.PrepareFolderCurrentlySelects(item.Title, item.Selected))
			} else {
				detailParts = append(detailParts, locales.PrepareFolderNoneSelected(item.Title))
			}
		}
		if item.Text != "" && item.Text != title {
			detailParts = append(detailParts, item.Text)
		}
		if item.Tool == "python" && item.Kind != ops.ProjectReady {
			detailParts = append(detailParts, locales.PreparePythonNote())
		}
		mono := ""
		if len(item.Command) > 0 {
			mono = ops.Command(item.Command)
		}
		if item.Config != "" {
			if mono != "" {
				mono += "\n"
			}
			mono += item.Config
		}
		var actions []statusRowAction
		if len(item.Command) > 0 {
			command := ops.Command(item.Command)
			actions = append(actions, statusRowAction{
				key:     "prepare-copy:" + item.Tool + ":" + strconv.Itoa(i),
				label:   locales.PrepareCopy(),
				onClick: func() { c.WriteClipboard(command); c.Toast(locales.PrepareCopied()) },
			})
		}
		if item.Config != "" {
			cfg := item.Config
			actions = append(actions, statusRowAction{
				key:     "prepare-setting:" + item.Tool + ":" + strconv.Itoa(i),
				label:   locales.PrepareCopySetting(),
				onClick: func() { c.WriteClipboard(cfg); c.Toast(locales.PrepareSettingCopied()) },
			})
		}
		if item.Tool == "python" {
			actions = append(actions, statusRowAction{
				key:     "prepare-python-guide:" + strconv.Itoa(i),
				label:   locales.PrepareGuide(),
				onClick: func() { c.OpenURL(uvProjectGuideURL) },
			})
		}
		if item.Tool != "" && item.Kind != ops.ProjectReady {
			tool := item.Tool
			actions = append(actions, statusRowAction{
				key:     "prepare-tool:" + tool + ":" + strconv.Itoa(i),
				label:   locales.PrepareOpenTool(),
				onClick: func() { a.op.open = false; a.router.Push(projectToolPath(tool)) },
			})
		}
		if a.Shell != nil && before {
			for _, file := range item.Files {
				actions = append(actions, statusRowAction{
					key:     "prepare-file:" + item.Tool + ":" + file,
					label:   file,
					onClick: func() { a.openFile(c, filepath.Join(p.Project.Dir, file)) },
				})
			}
		}
		rows = append(rows, statusRow{
			key:     "prepare-item:" + item.Tool + ":" + strconv.Itoa(i),
			tone:    tone,
			title:   title,
			detail:  strings.Join(detailParts, "\n"),
			mono:    mono,
			actions: actions,
		})
	}
	return rows
}

func (a *App) itemToolShortcutRows(c *ui.Context, p *ops.ProjectPreparation) []statusRow {
	var rows []statusRow
	for i, item := range p.Items {
		if item.Tool == "" || item.Kind == ops.ProjectReady {
			continue
		}
		tool := item.Tool
		rows = append(rows, statusRow{
			key:   "fail-tool:" + tool + ":" + strconv.Itoa(i),
			tone:  statusWarn,
			title: item.Title + withSpace(item.Version),
			actions: []statusRowAction{{
				key:     "fail-open-tool:" + tool + ":" + strconv.Itoa(i),
				label:   locales.PrepareOpenTool(),
				onClick: func() { a.op.open = false; a.router.Push(projectToolPath(tool)) },
			}},
		})
	}
	return rows
}

func itemStatusTone(item ops.ProjectItem, before bool) statusTone {
	if before {
		return statusWarn
	}
	switch item.Kind {
	case ops.ProjectReady:
		return statusOK
	case ops.ProjectConflict, ops.ProjectUnknown, ops.ProjectUnchecked:
		return statusError
	default:
		return statusWarn
	}
}

func itemStatusTitle(item ops.ProjectItem) string {
	switch item.Kind {
	case ops.ProjectReady:
		if item.Tool == "docker" {
			if item.SelectedEngine != "" {
				return locales.PrepareDockerEngineAnswers(item.SelectedEngine)
			}
			return locales.PrepareDockerReady()
		}
		if item.Selected != "" {
			return locales.PrepareFolderSelects(item.Title, item.Selected)
		}
		return locales.PrepareReady(item.Title + withSpace(item.Version))
	case ops.ProjectNeedsSelection:
		if item.Manager != "" && item.Version != "" {
			return locales.PrepareInstalledVia(item.Manager, item.Title, item.Version)
		}
		return item.Title + withSpace(item.Version)
	case ops.ProjectPythonAvailable:
		if item.Version != "" {
			return locales.PrepareInstalledVia("uv", item.Title, item.Version)
		}
		return item.Title
	default:
		return item.Title + withSpace(item.Version)
	}
}

func (a *App) stepStatusRows(steps []ops.Step, s ops.State) []statusRow {
	rows := make([]statusRow, 0, len(steps))
	for i, step := range steps {
		label := stepLabel(step, a.Home)
		st := ops.Waiting
		if i < len(s.Steps) {
			st = s.Steps[i]
		}
		row := statusRow{key: "step-res:" + strconv.Itoa(i)}
		switch st {
		case ops.Done:
			row.tone = statusOK
			row.title = locales.PrepareStepDone(label)
		case ops.StepFailed:
			row.tone = statusError
			row.title = locales.PrepareStepFailed(label)
			if reason := failReason(s); reason != "" {
				row.detail = reason
			}
		case ops.StepStopped:
			row.tone = statusWarn
			row.title = locales.PrepareStepStopped(label)
		case ops.Skipped, ops.Waiting:
			row.tone = statusSkipped
			row.title = locales.PrepareStepSkipped(label)
		case ops.Busy:
			row.tone = statusBusy
			row.title = label
		}
		rows = append(rows, row)
	}
	return rows
}

func stepLabel(step ops.Step, home string) string {
	command := ops.Command(step.Shown())
	if e, _, edit := stepEdit(step); edit {
		command = locales.OpEditStep(tilde(e.Path, home))
	}
	if step.Skill != nil {
		command = locales.SkillInstallStep(tilde(step.Skill.Path, home))
	}
	if step.Remove != "" {
		command = locales.OpRemoveStep(tilde(step.Remove, home))
	}
	if step.Note != "" {
		command += " · " + step.Note
	}
	return command
}

func (a *App) selectionSummaryNotes(items []ops.ProjectItem) []ops.Note {
	var notes []ops.Note
	for _, item := range items {
		if item.Kind != ops.ProjectNeedsSelection {
			continue
		}
		title := locales.PrepareFolderNoneSelected(item.Title)
		if item.Selected != "" {
			title = locales.PrepareFolderCurrentlySelects(item.Title, item.Selected)
		}
		text := locales.PrepareCommandInFolder()
		if len(item.Command) > 0 {
			text = ops.Command(item.Command) + " · " + text
		}
		notes = append(notes, ops.Note{Title: title, Text: text})
	}
	return notes
}

func statusListCard(c *ui.Context, caption string, rows []statusRow) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(2).Padding(6).Margin(0, 20, 14, 20).Radius(9).Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Children(func() {
		if caption != "" {
			ui.Text(c, caption).FontSize(11).TextColor(pal.TextFaint).Padding(4, 10, 4, 10)
		}
		for _, r := range rows {
			inlineActions := len(r.actions) > 0 && (r.tone == statusFile || (r.detail == "" && r.mono == ""))
			align := ui.Start
			iconTop := float32(2)
			if inlineActions && r.detail == "" && r.mono == "" {
				align = ui.Center
				iconTop = 0
			}
			ui.Row(c.Key(r.key)).Gap(10).Padding(8, 10).Radius(6).AlignItems(align).Background(t.Background).Children(func() {
				iconName, iconColor := statusIcon(t, pal, r.tone)
				ui.Icon(c, Icon(iconName)).Size(15, 15).Shrink(0).TextColor(iconColor).Margin(iconTop, 0, 0, 0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					titleColor := t.Text
					if r.tone == statusSkipped {
						titleColor = t.TextMuted
					}
					ui.Text(c, r.title).FontSize(12.5).FontWeight(500).TextColor(titleColor).Wrap()
					if r.detail != "" {
						ui.Text(c, r.detail).FontSize(11.5).TextColor(t.TextMuted).Wrap()
					}
					if r.mono != "" {
						ui.Text(c, r.mono).Font(FontMono).FontSize(11.5).TextColor(pal.MonoText).Wrap().Selectable()
					}
					if len(r.actions) > 0 && !inlineActions {
						ui.Row(c).Gap(6).Wrap().Margin(3, 0, 0, 0).Children(func() {
							for _, act := range r.actions {
								ui.Button(c.Key(act.key), act.label).OnClick(act.onClick)
							}
						})
					}
				})
				if inlineActions {
					ui.Row(c).Gap(6).Shrink(0).Children(func() {
						for _, act := range r.actions {
							ui.Button(c.Key(act.key), act.label).OnClick(act.onClick)
						}
					})
				}
			})
		}
	})
}

func statusIcon(t *ui.Theme, pal Palette, tone statusTone) (string, ui.Color) {
	switch tone {
	case statusOK:
		return "circle-check", t.Success
	case statusError:
		return "circle-alert", t.Danger
	case statusWarn:
		return "triangle-alert", t.Warning
	case statusSkipped:
		return "minus", pal.TextFaint
	case statusFile:
		return "file-text", t.TextMuted
	case statusBusy:
		return "refresh-cw", t.Accent
	}
	return "info", t.TextMuted
}

func worthKnowingList(c *ui.Context, notes []ops.Note) {
	if len(notes) == 0 {
		return
	}
	pal := Colors(c.Theme())
	dialogSection(c, strings.ToUpper(locales.OpWorthKnowing()))
	for i, n := range notes {
		bottom := float32(8)
		if i == len(notes)-1 {
			bottom = 14
		}
		ui.Row(c).Gap(12).Padding(8, 20, bottom, 20).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, n.Title).FontSize(13)
				if n.Text != "" {
					ui.Text(c, n.Text).FontSize(11.5).TextColor(pal.TextFaint).Wrap()
				}
			})
			switch {
			case n.URL != "":
				ui.Button(c.Key("note:"+n.URL), locales.OpOpenLink()+" ↗").OnClick(func() { c.OpenURL(n.URL) })
			case n.Tag != "":
				ui.Text(c, n.Tag).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			}
		})
	}
}

func projectToolPath(tool string) string {
	switch tool {
	case "pnpm", "yarn", "npm":
		return "/tools/jspm"
	case "composer":
		return "/tools/php"
	case "uv", "pipx":
		return "/tools/python"
	case "bundler":
		return "/tools/ruby"
	case "maven", "gradle":
		return "/tools/java"
	case "git":
		return gitPage
	case "cpp":
		return cppPage
	case "docker":
		return "/tools/docker"
	case "kubectl", "helm", "kind":
		return kubePage
	}
	for _, l := range languages {
		if l.id == tool {
			return "/tools/" + tool
		}
	}
	return cliPage
}

func singleManualCommand(items []ops.ProjectItem) string {
	cmd := ""
	for _, item := range items {
		if item.Kind == ops.ProjectReady || len(item.Command) == 0 {
			continue
		}
		if cmd != "" {
			return ""
		}
		cmd = ops.Command(item.Command)
	}
	return cmd
}

func (a *App) projectPreparationButtons(c *ui.Context, op *operation) {
	dir := op.project.Project.Dir
	outcome, active := a.projectOutcome(op)
	ui.Button(c.Key("prepare-close"), locales.OpClose()).Height(controlHeight).OnClick(func() { op.open = false })
	if outcome == outcomeFailed && (len(op.state.Log) > 0 || op.state.Err != "") {
		ui.Button(c.Key("prepare-copy-log"), locales.OpCopyLog()).Height(controlHeight).OnClick(func() {
			log := strings.Join(op.state.Log, "\n")
			if log == "" {
				log = tilde(op.state.Err, a.Home)
			}
			c.WriteClipboard(log)
			c.Toast(locales.OpLogCopied())
		})
	}
	if a.canOpenFile() {
		ui.Button(c.Key("prepare-folder"), locales.PrepareOpenFolder()).Height(controlHeight).OnClick(func() { a.openFile(c, dir) })
	}
	if outcome == outcomeReady {
		if active != nil && active.Project.Readme != "" && a.canOpenFile() {
			readme := active.Project.Readme
			ui.Button(c.Key("prepare-readme"), locales.PrepareOpenReadme(readme)).Height(controlHeight).OnClick(func() { a.openFile(c, filepath.Join(dir, readme)) })
		}
		ui.PrimaryButton(c.Key("prepare-to-project"), locales.PrepareToProject()).Height(controlHeight).OnClick(func() {
			op.open = false
			a.router.Push(projectPath(dir))
		})
		return
	}
	if (outcome == outcomeNeedsAction || outcome == outcomeAlreadyInstalled) && active != nil {
		if cmd := singleManualCommand(active.Items); cmd != "" {
			ui.Button(c.Key("prepare-copy-footer"), locales.PrepareCopyExact(cmd)).Height(controlHeight).OnClick(func() {
				c.WriteClipboard(cmd)
				c.Toast(locales.PrepareCopied())
			})
		}
	}
	label := locales.PrepareAgain()
	if outcome == outcomeFailed || outcome == outcomeStopped {
		label = locales.PrepareContinue()
	}
	ui.PrimaryButton(c.Key("prepare-again"), label).Height(controlHeight).
		Disabled(a.BuildPlan == nil || outcome == outcomeChecking).
		OnClick(func() { op.open = false; a.prepareProject(dir) })
}
