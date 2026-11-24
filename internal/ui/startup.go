package ui

import (
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// The times the startup tab judges a shell start and its lines by: a
// start up to fastStart goes unnoticed, one from slowStart is traced line
// by line, and a line from slowLine takes most of what a start may.
const (
	fastStart = 300 * time.Millisecond
	slowStart = 500 * time.Millisecond
	slowLine  = 300 * time.Millisecond
	// shownFile is the least time a traced startup file takes to be
	// listed: a system's has a score of them.
	shownFile = 10 * time.Millisecond
)

// fileColumn is the width of the startup tab's column of files, which
// holds bash_profile.
const fileColumn = 104

// seconds is d as the page shows it, as 1.42 s.
func seconds(d time.Duration) string {
	return locales.DurationSeconds(d.Seconds())
}

// startupCard is the header's card of the shell's start.
func startupCard(c *ui.Context, s *inventory.ShellStartup) {
	if s.Took >= slowStart {
		stateCard(c, c.Theme().Warning, locales.StartupLabel(), seconds(s.Took), locales.StartupSlow(s.Shell))
		return
	}
	stateCard(c, c.Theme().Success, locales.StartupLabel(), seconds(s.Took), s.Shell)
}

// startupLines is the PATH page's tab of the shell's start: the lines of the
// startup files that take the time, slowest first, and the rest.
func (a *App) startupLines(c *ui.Context, s *inventory.ShellStartup) {
	pal := Colors(c.Theme())
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(12).Padding(8, 16).Background(pal.Subtle).Children(func() {
			ui.Text(c, locales.StartupTime()).Width(100).SingleLine().FontSize(11.5)
			ui.Text(c, locales.StartupFile()).Width(fileColumn).FontSize(11.5)
			ui.Text(c, locales.StartupLine()).Grow(1).FontSize(11.5)
		})
		var listed time.Duration
		for i, l := range s.Lines {
			listed += l.Took
			a.startupRow(c, s, i, l)
		}
		switch {
		case len(s.Lines) > 0 && s.Lines[0].Took > 0 && s.Took-listed >= time.Millisecond:
			rest := s.Took - listed
			ui.Row(c).Gap(12).Padding(12, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Width(100).Gap(2).Children(func() {
					ui.Text(c, seconds(rest)).Font(FontMono).FontSize(12.5).TextColor(pal.TextFaint)
					ui.Text(c, locales.StartupShare(share(rest, s.Took))).FontSize(11).TextColor(pal.TextFaint)
				})
				ui.Text(c, "—").Width(fileColumn).TextColor(pal.TextFaint)
				ui.Text(c, locales.StartupRest(len(s.Files))).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).Grow(1)
			})
		case len(s.Lines) == 0:
			cardNote(c, startupNote(s))
		}
	})
	if len(s.Lines) > 0 && s.Lines[0].Took > 0 {
		toolNote(c, locales.StartupNote())
	} else if len(s.Lines) > 0 {
		toolNote(c, locales.StartupProfileNote())
	}
}

// startupNote says why the tab has no lines.
func startupNote(s *inventory.ShellStartup) string {
	switch {
	case s.Took < slowStart:
		return locales.StartupFast()
	case s.Shell == "powershell" || s.Shell == "pwsh":
		return locales.StartupProfileNoLines()
	case s.Shell == "zsh" || s.Shell == "bash":
		return locales.StartupNoTrace(s.Shell)
	}
	return locales.StartupNoTraceShell(s.Shell)
}

func (a *App) startupRow(c *ui.Context, s *inventory.ShellStartup, i int, l inventory.StartupLine) {
	pal := Colors(c.Theme())
	culprit := sysenv.StartupCulprit(l.Text)
	ui.Row(c.Key("startup-"+strconv.Itoa(i))).Gap(12).Padding(12, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Width(100).Gap(2).Children(func() {
			if l.Took > 0 {
				ui.Text(c, seconds(l.Took)).Font(FontMono).FontSize(12.5)
				ui.Text(c, locales.StartupShare(share(l.Took, s.Took))).FontSize(11).TextColor(pal.TextFaint)
			} else {
				ui.Text(c, "—").TextColor(pal.TextFaint)
			}
		})
		ui.Box(c).Width(fileColumn).Children(func() { fileChip(c, l.File) })
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			clipped(c, l.Text).Font(FontMono).FontSize(11.5)
			ui.Row(c).Gap(5).Wrap().Children(func() {
				chip(c, pal.TextFaint, pal.Muted, pal.MonoText, locales.StartupLineNumber(l.Line))
				if l.Took >= slowLine || l.Took == 0 && culprit != "" {
					chip(c, c.Theme().Warning, pal.WarningSoft, pal.WarningText, locales.StartupSlowChip())
				}
			})
			if advice := culpritAdvice(culprit); advice != "" {
				ui.Text(c, advice).FontSize(11.5).TextColor(c.Theme().TextMuted)
			}
		})
		if culprit == sysenv.CulpritNvm && a.BuildPlan != nil {
			ui.Button(c.Key("startup-nvm:"+strconv.Itoa(i)), locales.MoveToMise()).OnClick(func() { a.nvmMove(l) })
		}
		if a.canOpenFile() || a.Shell != nil {
			iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(m *ui.Menu) {
				if a.canOpenFile() && m.Item(locales.OpenInEditor()).Chosen() {
					a.openFile(c, l.File)
				}
				if a.Shell != nil && m.Item(showInFolder()).Chosen() {
					a.Shell.ShowItemInFolder(l.File)
				}
			})
		}
	})
}

// fileChip is a startup file's name in a frame, as zshrc, with its
// path in the tooltip.
func fileChip(c *ui.Context, file string) {
	pal := Colors(c.Theme())
	name := filepath.Base(file)
	if len(name) > 1 && name[0] == '.' {
		name = name[1:]
	}
	ui.Text(c, name).Font(FontMono).FontSize(11).SingleLine().Tooltip(file).Padding(2, 7).Radius(5).BorderWidth(1).BorderColor(pal.Divider)
}

// share is part of whole in percent.
func share(part, whole time.Duration) int {
	if whole <= 0 {
		return 0
	}
	return int(float64(part)/float64(whole)*100 + 0.5)
}

// culpritAdvice says what makes a known culprit fast, "" for none.
func culpritAdvice(k sysenv.Culprit) string {
	switch k {
	case sysenv.CulpritNvm:
		return locales.CulpritNvm()
	case sysenv.CulpritConda:
		return locales.CulpritConda()
	case sysenv.CulpritPyenv:
		return locales.CulpritPyenv()
	case sysenv.CulpritRbenv:
		return locales.CulpritRbenv()
	case sysenv.CulpritRvm:
		return locales.CulpritRvm()
	case sysenv.CulpritSdkman:
		return locales.CulpritSdkman()
	case sysenv.CulpritCompinit:
		return locales.CulpritCompinit()
	case sysenv.CulpritOhMyZsh:
		return locales.CulpritOhMyZsh()
	case sysenv.CulpritOhMyPosh:
		return locales.CulpritOhMyPosh()
	case sysenv.CulpritPoshGit:
		return locales.CulpritPoshGit()
	case sysenv.CulpritBrew:
		return locales.CulpritBrew()
	}
	return ""
}

// startupSide is the startup tab's column: how Hearth measures, and the
// startup files with their time.
func (a *App) startupSide(c *ui.Context, s *inventory.ShellStartup) {
	pal := Colors(c.Theme())
	value := func(text string) func() {
		return func() { ui.Text(c, text).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine() }
	}
	listCard(c, locales.StartupHow(), s.Shell, func() {
		if s.Bare > 0 {
			listRow(c, locales.StartupWhole(), false, locales.StartupWholeProfile(), value(seconds(s.Took)))
			listRow(c, locales.StartupBare(), false, locales.StartupBareCaption(), value(seconds(s.Bare)))
		} else {
			listRow(c, locales.StartupWhole(), false, locales.StartupWholeCaption(s.Shell), value(seconds(s.Took)))
			listRow(c, locales.StartupByLine(), false, locales.StartupByLineCaption(seconds(slowStart)), value("zsh, bash"))
		}
		listRow(c, locales.StartupTarget(), false, locales.StartupTargetCaption(seconds(fastStart)), value(seconds(fastStart)))
	})
	if len(s.Files) == 0 {
		return
	}
	listCard(c, locales.StartupFiles(), locales.StartupFilesOrder(), func() {
		for _, f := range s.Files {
			file := f.File
			openBtn := func() {
				if a.canOpenFile() {
					iconButton(c.Key("startup-file:"+file), "external-link", locales.OpenInEditor()).OnClick(func() { a.openFile(c, file) })
				}
			}
			// Windows' profiles have no time: each is listed.
			if s.Bare > 0 {
				listRow(c, tilde(file, a.Home), false, "", openBtn)
			} else if f.Took >= shownFile {
				took := f.Took
				listRow(c, tilde(file, a.Home), false, "", func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
						ui.Text(c, seconds(took)).Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
						openBtn()
					})
				})
			}
		}
	})
}

// nvmMove offers the plan that stops line from loading nvm, Node's
// default moved to mise.
func (a *App) nvmMove(l inventory.StartupLine) {
	a.nvmMovePlan(l, a.proposeOrSay)
}

func (a *App) nvmMovePlan(l inventory.StartupLine, done func(ops.Plan, error)) {
	node, _ := a.toolState("node")
	detect := a.detect("node")
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.NvmMovePlan(l, node, toolTitle("node"), env, detect)
	}, done)
}
