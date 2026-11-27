package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

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
