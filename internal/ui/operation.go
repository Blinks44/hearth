package ui

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

// operation is a plan the operation dialog shows, and how it goes once
// the user confirmed it.
type operation struct {
	plan    ops.Plan
	state   ops.State
	started bool   // confirmed: state says how it goes
	open    bool   // the dialog shows; hidden while it runs, a toast does
	stop    func() // stops it while it runs
	told    bool   // the toast of its end showed
}

// running reports that the operation was confirmed and is not over.
func (op *operation) running() bool { return op != nil && op.started && !op.state.Phase.Over() }

// Propose shows p in the operation dialog, for the user to read and
// confirm. While another operation runs, that one shows instead: one
// change to the system at a time.
func (a *App) Propose(p ops.Plan) {
	if a.op.running() {
		a.op.open = true
		return
	}
	a.op = &operation{plan: p, open: true}
}

// startOperation runs the operation shown, which the user confirmed, and
// scans again once it is over: whatever it did, the system changed.
func (a *App) startOperation() {
	op := a.op
	op.started, op.told = true, false
	op.state = ops.State{Phase: ops.Running, Steps: make([]ops.StepStatus, len(op.plan.Steps))}
	op.stop = a.RunPlan(op.plan, func(s ops.State) {
		if a.op != op || op.state.Phase.Over() {
			return
		}
		op.state = s
		if s.Phase.Over() && a.OnScan != nil {
			a.Scanning = true
			a.OnScan()
		}
	})
}

// operationView shows the operation: its dialog, or while it runs hidden,
// a toast of its progress, and the toast of its end.
func (a *App) operationView(c *ui.Context) {
	op := a.op
	if op == nil {
		return
	}
	if op.open {
		a.operationDialog(c, op)
	}
	switch {
	case op.open || !op.started:
		op.told = op.told || op.state.Phase.Over()
	case !op.state.Phase.Over():
		a.operationToast(c, op)
	case !op.told:
		op.told = true
		toast := ui.Toast{Title: locales.OpToastDone(op.plan.Title), Type: "success", Action: locales.OpShow(),
			OnAction: func() { op.open = true }}
		// What went wrong waits for the user.
		if op.state.Phase != ops.Succeeded {
			toast.Title, toast.Type, toast.Action, toast.Timeout = locales.OpToastFailed(op.plan.Title), "error", locales.OpDetails(), -1
		}
		if op.state.Phase == ops.Stopped {
			toast.Title = locales.OpToastStopped(op.plan.Title)
		}
		c.AddToast(toast)
	}
}

// operationDialog is "What will change" before the user confirms, then
// the steps as they run, and how it ended.
func (a *App) operationDialog(c *ui.Context, op *operation) {
	t := c.Theme()
	pal := Colors(t)
	ui.DialogBase(c.Key("operation"), &op.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Children(func() {
			ui.Column(c).Gap(4).Padding(20, 20, 14, 20).Children(func() {
				ui.Text(c, op.plan.Title).FontSize(18).FontWeight(600)
				ui.Text(c, operationCaption(op)).FontSize(12).TextColor(t.TextMuted)
				if op.started {
					a.operationProgress(c, op).Margin(10, 0, 0, 0)
				}
			})
			ui.Scroll(c).MaxHeight(440).Children(func() {
				if !op.started {
					a.operationPlan(c, op.plan)
					return
				}
				dialogSection(c, strings.ToUpper(locales.OpSteps()))
				operationSteps(c, op.plan.Steps, op.state)
				a.operationResult(c, op)
			})
			ui.Row(c).Gap(8).Padding(12, 20).Justify(ui.End).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				a.operationButtons(c, op)
			})
		})
	})
}

// operationCaption says, under the title, why before the user confirms,
// then where the operation is and how it ended.
func operationCaption(op *operation) string {
	s := op.state
	n, total := s.Current+1, len(op.plan.Steps)
	switch {
	case !op.started:
		return op.plan.Why
	case s.Phase == ops.Running:
		return locales.OpStepOf(n, total)
	case s.Phase == ops.Verifying:
		return locales.OpVerifying()
	case s.Phase == ops.Succeeded && s.Verified != "":
		return locales.OpVerified(s.Verified)
	case s.Phase == ops.Succeeded:
		return locales.OpSucceeded()
	case s.Phase == ops.Failed:
		why := s.Err
		if why == "" {
			why = locales.OpExitCode(s.ExitCode)
		}
		return locales.OpFailed(n, why)
	case s.Phase == ops.NotVerified:
		return locales.OpNotVerified(s.Err)
	}
	return locales.OpStopped(n)
}

// operationProgress is the bar of the steps done, colored as it ended.
func (a *App) operationProgress(c *ui.Context, op *operation) ui.Element {
	t := c.Theme()
	pal := Colors(t)
	done := 0
	for _, st := range op.state.Steps {
		if st == ops.Done {
			done++
		}
	}
	value := 1.0
	if len(op.state.Steps) > 0 {
		value = float64(done) / float64(len(op.state.Steps))
	}
	// The step it is at, or ended at, counts for half: the bar moves as a
	// step starts.
	if i := op.state.Current; i < len(op.state.Steps) && op.state.Steps[i] != ops.Done {
		value += 0.5 / float64(len(op.state.Steps))
	}
	color := t.Accent
	switch op.state.Phase {
	case ops.Succeeded:
		color = t.Success
	case ops.Failed, ops.NotVerified:
		color = t.Danger
	}
	// ui.Progress fills with the accent only: the bar is two parts that
	// share its width.
	value = min(value, 1)
	return ui.Row(c).Height(4).AlignItems(ui.Stretch).Radius(2).Clip().Background(pal.Muted).Children(func() {
		ui.Box(c).Grow(float32(value)).Background(color)
		ui.Box(c).Grow(float32(1 - value))
	})
}

// operationPlan is what will change: the commands in order and what is
// worth knowing first.
func (a *App) operationPlan(c *ui.Context, p ops.Plan) {
	t := c.Theme()
	pal := Colors(t)
	dialogSection(c, strings.ToUpper(locales.OpCommands()))
	operationSteps(c, p.Steps, ops.State{Steps: make([]ops.StepStatus, len(p.Steps)), Current: -1})
	if len(p.Notes) == 0 {
		return
	}
	dialogSection(c, strings.ToUpper(locales.OpWorthKnowing()))
	for i, n := range p.Notes {
		bottom := float32(8)
		if i == len(p.Notes)-1 {
			bottom = 14
		}
		ui.Row(c).Gap(12).Padding(8, 20, bottom, 20).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, n.Title).FontSize(13)
				if n.Text != "" {
					ui.Text(c, n.Text).FontSize(11.5).TextColor(pal.TextFaint)
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

// operationSteps are the commands, numbered, each with its status once
// the operation started.
func operationSteps(c *ui.Context, steps []ops.Step, s ops.State) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(2).Padding(6).Radius(9).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).
		Margin(0, 20, 14, 20).Font(FontMono).Children(func() {
		for i, step := range steps {
			status := s.Steps[i]
			color := t.Text
			if s.Current >= 0 && (status == ops.Waiting || status == ops.Skipped) {
				color = t.TextMuted
			}
			row := ui.Row(c).Gap(10).Padding(7, 10).Radius(6).AlignItems(ui.Center)
			if i == s.Current || s.Current < 0 {
				row.Background(t.Background)
			}
			row.Children(func() {
				ui.Text(c, strconv.Itoa(i+1)).FontSize(11.5).TextColor(pal.TextFaint)
				command := ops.Command(step.Argv)
				if step.Note != "" {
					command += "  ·  " + step.Note
				}
				clipped(c, command).FontSize(12).TextColor(color).Grow(1).MinWidth(0)
				if s.Current >= 0 {
					label, bg, fg := stepTag(c, status)
					aiTag(c, label, bg, fg)
				}
			})
		}
	})
}

// stepTag is the label of a step's status and its colors.
func stepTag(c *ui.Context, st ops.StepStatus) (label string, bg, fg ui.Color) {
	t := c.Theme()
	pal := Colors(t)
	switch st {
	case ops.Busy:
		return locales.OpTagRunning(), pal.InfoSoft, pal.InfoText
	case ops.Done:
		return locales.OpTagDone(), pal.SuccessSoft, pal.SuccessText
	case ops.StepFailed:
		return locales.OpTagFailed(), pal.DangerSoft, pal.DangerText
	case ops.StepStopped:
		return locales.OpTagStopped(), pal.WarningSoft, pal.WarningText
	case ops.Skipped:
		return locales.OpTagSkipped(), pal.Muted, t.TextMuted
	}
	return locales.OpTagWaiting(), pal.Muted, t.TextMuted
}

// operationResult is below the steps: the output of the step running,
// the last lines of the one that failed, what changed or what stays.
func (a *App) operationResult(c *ui.Context, op *operation) {
	s := op.state
	pal := Colors(c.Theme())
	switch s.Phase {
	case ops.Running, ops.Verifying:
		command := ""
		if s.Current < len(op.plan.Steps) {
			command = ops.Command(op.plan.Steps[s.Current].Argv)
		}
		dialogSection(c, locales.OpLog(command))
		logBlock(c, lastLines(s.Log, 8), c.Theme().TextMuted, "")
	case ops.Failed, ops.NotVerified:
		dialogSection(c, locales.OpLastLines())
		logBlock(c, lastLines(s.Log, 5), pal.DangerText, locales.OpFullLog())
	case ops.Succeeded:
		if len(op.plan.Changes) > 0 {
			dialogSection(c, strings.ToUpper(locales.OpChanged()))
			logBlock(c, op.plan.Changes, c.Theme().Text, "")
		}
	case ops.Stopped:
		done := 0
		for _, st := range s.Steps {
			if st == ops.Done {
				done++
			}
		}
		dialogSection(c, strings.ToUpper(locales.OpLeft()))
		left := locales.OpLeftNone()
		if done > 0 {
			left = locales.OpLeftSteps(done)
		}
		logBlock(c, []string{left}, c.Theme().Text, "")
	}
}

// lastLines returns the last n of lines.
func lastLines(lines []string, n int) []string {
	return lines[max(len(lines)-n, 0):]
}

// logBlock is lines of output in a mono block, with a footnote under them.
func logBlock(c *ui.Context, lines []string, color ui.Color, footnote string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Gap(6).Padding(12, 14).Margin(0, 20, 14, 20).Radius(9).Background(pal.Subtle).
		BorderWidth(1).BorderColor(pal.Line).Font(FontMono).Children(func() {
		if len(lines) == 0 {
			ui.Text(c, "…").FontSize(11.5).TextColor(pal.TextFaint)
		}
		for _, line := range lines {
			clipped(c, line).FontSize(11.5).TextColor(color)
		}
		if footnote != "" {
			ui.Text(c, footnote).FontSize(11.5).TextColor(t.TextMuted)
		}
	})
}

// operationButtons are the dialog's actions for where the operation is.
func (a *App) operationButtons(c *ui.Context, op *operation) {
	openJournal := func() {
		if a.Shell != nil && a.JournalPath != "" {
			a.openFile(c, a.JournalPath)
		}
	}
	hide := func() { op.open = false }
	switch {
	case !op.started:
		ui.Button(c.Key("op-cancel"), locales.Cancel()).OnClick(hide)
		action := op.plan.Action
		if action == "" {
			action = locales.OpRun()
		}
		ui.PrimaryButton(c.Key("op-run"), action).Disabled(a.RunPlan == nil).OnClick(a.startOperation)
	case !op.state.Phase.Over():
		ui.Button(c.Key("op-hide"), locales.OpHide()).OnClick(hide)
		ui.Button(c.Key("op-stop"), locales.OpStop()).TextColor(c.Theme().Danger).
			Disabled(op.state.Phase != ops.Running).OnClick(func() { op.stop() })
	case op.state.Phase == ops.Succeeded:
		if a.JournalPath != "" && a.Shell != nil {
			ui.Button(c.Key("op-journal"), locales.OpOpenJournal()).OnClick(openJournal)
		}
		ui.PrimaryButton(c.Key("op-done"), locales.OpDone()).OnClick(hide)
	case op.state.Phase == ops.Stopped:
		ui.Button(c.Key("op-close"), locales.OpClose()).OnClick(hide)
		ui.PrimaryButton(c.Key("op-again"), locales.OpRestart()).OnClick(a.startOperation)
	default:
		ui.Button(c.Key("op-copy"), locales.OpCopyLog()).OnClick(func() {
			c.WriteClipboard(strings.Join(op.state.Log, "\n"))
			c.Toast(locales.OpLogCopied())
		})
		ui.PrimaryButton(c.Key("op-again"), locales.OpRetry()).OnClick(a.startOperation)
	}
}

// operationToast is the operation running hidden: its step and progress,
// to show it again or stop it.
func (a *App) operationToast(c *ui.Context, op *operation) {
	t := c.Theme()
	pal := Colors(t)
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Right(16).Bottom(16).Width(360).Gap(8).Padding(14, 16).Radius(12).
			Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.14)).Children(func() {
			ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
				ui.Box(c).Size(28, 28).Radius(7).Background(pal.InfoSoft).Center().Children(func() {
					ui.Icon(c, Icon("download")).Size(15, 15).TextColor(pal.Info)
				})
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, op.plan.Title+"…").FontSize(13.5).FontWeight(600)
					ui.Text(c, operationCaption(op)).FontSize(12).TextColor(t.TextMuted)
					a.operationProgress(c, op).Margin(6, 0, 4, 0)
					ui.Row(c).Gap(8).Children(func() {
						ui.Button(c.Key("op-toast-show"), locales.OpShow()).OnClick(func() { op.open = true })
						ui.Button(c.Key("op-toast-stop"), locales.OpStop()).
							Disabled(op.state.Phase != ops.Running).OnClick(func() { op.stop() })
					})
				})
			})
		})
	})
}
