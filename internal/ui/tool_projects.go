package ui

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

// toolProject is an added project that declares a requirement for one tool,
// paired with its resolved status against the current snapshot.
type toolProject struct {
	project projects.Project
	status  projects.Status
}

// toolProjects returns every added project requiring tool, in the order added.
func (a *App) toolProjects(tool string) []toolProject {
	var out []toolProject
	lines := a.nodeLines()
	for _, p := range a.Projects {
		for _, st := range projects.Resolve(p, a.Snapshot, lines) {
			if st.Tool == tool {
				out = append(out, toolProject{project: p, status: st})
				break
			}
		}
	}
	return out
}

// toolProjectCard draws the top-right Project stateCard on a language page.
// When projects use tool, it surfaces the first one with a problem (warning
// dot) or else the first project (accent dot), showing the version the terminal
// runs there and the project name + source file.
func (a *App) toolProjectCard(c *ui.Context, tool string, items []toolProject) {
	t := c.Theme()
	pal := Colors(t)
	if len(items) == 0 {
		stateCard(c, pal.TextFaint, locales.Project(), "—", locales.NoProjects())
		return
	}
	chosen := items[0]
	dot := Accent
	for _, item := range items {
		if statusProblem(item.status) != "" {
			chosen = item
			dot = t.Warning
			break
		}
	}
	value := "—"
	if chosen.status.Terminal != nil {
		value = toolProjectVersion(tool, *chosen.status.Terminal)
	}
	caption := filepath.Base(chosen.project.Dir)
	if src := primaryCheck(chosen.status); src.File != "" {
		caption += " · " + src.File
	}
	stateCard(c, dot, locales.Project(), value, caption)
}

func primaryCheck(st projects.Status) projects.Check {
	for _, ch := range st.Checks {
		if ch.Want != "" {
			return ch
		}
	}
	if len(st.Checks) > 0 {
		return st.Checks[0]
	}
	return projects.Check{}
}

func toolProjectVersion(tool string, in inventory.Installation) string {
	switch tool {
	case "node", "bun":
		return versionText(in.Version)
	case "go":
		if in.Version != "" {
			return in.Version
		}
	}
	return copyVersion(in)
}

// toolProjectsTab renders the Projects tab shared by language pages: every
// added project requiring tool, its pin/constraint, what runs there, quick
// install when missing, and a footer row with the terminal default.
func (a *App) toolProjectsTab(c *ui.Context, tool string, st inventory.ToolState, items []toolProject) {
	t := c.Theme()
	pal := Colors(t)
	title := locales.ToolProjectsTitle(toolTitle(tool))
	listCard(c, title, toolProjectFilesAside(tool), func() {
		if len(items) == 0 {
			cardNote(c, locales.ToolNoProjects(toolTitle(tool)))
		}
		for _, item := range items {
			p := item.project
			pst := item.status
			dir := p.Dir
			var parts []string
			parts = append(parts, tilde(dir, a.Home))
			parts = append(parts, toolProjectCheckParts(pst)...)
			run := "—"
			if pst.Terminal != nil {
				run = toolProjectVersion(tool, *pst.Terminal)
			}
			listRow(c, filepath.Base(dir), false, strings.Join(parts, " · "), func() {
				ui.Text(c, run).Font(FontMono).FontSize(12).TextColor(pal.MonoText).SingleLine()
				label, runInstall, canInstall := a.projectInstallAction(pst, a.proposeOrSay)
				switch {
				case canInstall:
					ui.Button(c.Key("tool-proj-install:"+dir), "").Height(controlHeight).Label(label).OnClick(runInstall).Children(func() {
						ui.Icon(c, Icon("download")).Size(13, 13).TextColor(t.TextMuted)
						ui.Text(c, locales.Install()).SingleLine()
					})
				case !pst.Checked:
					chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.ChipNotChecked())
				case statusProblem(pst) == "":
					chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.ChipReady())
				default:
					chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.ChipNotReady())
				}
				iconButton(c.Key("tool-proj-open:"+dir), "chevron-right", locales.Details()).OnClick(func() {
					a.router.Push(projectPath(dir))
				})
			})
			if pst.Checked {
				if prob := statusProblem(pst); prob != "" {
					ui.Box(c).Padding(0, 12, 10, 12).Children(func() {
						warningRow(c, problemBanner([]projects.Status{pst}))
					})
				}
			}
		}
		v := "—"
		if active := inventory.TerminalCopy(st.Installations); active >= 0 {
			v = toolProjectVersion(tool, st.Installations[active])
		}
		listRow(c, locales.ToolDefaultRow(), false, "", func() {
			ui.Text(c, v).Font(FontMono).FontSize(12).SingleLine()
		})
	})
}

func toolProjectFilesAside(tool string) string {
	switch tool {
	case "node":
		return ".nvmrc · package.json · mise.toml"
	case "bun":
		return ".bun-version · bun.lock · mise.toml"
	case "python":
		return ".python-version · pyproject.toml · Pipfile"
	case "php":
		return ".php-version · composer.json"
	case "rust":
		return "rust-toolchain.toml · Cargo.toml"
	case "go":
		return ".go-version · go.mod · mise.toml"
	case "java":
		return ".java-version · pom.xml · build.gradle"
	case "ruby":
		return ".ruby-version · Gemfile"
	case "dotnet":
		return "global.json · *.csproj"
	}
	return ""
}

// toolProjectCheckParts formats a project's checks for one tool grouped by
// file with duplicate requirements removed.
func toolProjectCheckParts(pst projects.Status) []string {
	type fileGroup struct {
		file  string
		wants []string
		errs  []string
	}
	var groups []fileGroup
	for _, ch := range pst.Checks {
		idx := slices.IndexFunc(groups, func(g fileGroup) bool { return g.file == ch.File })
		if idx < 0 {
			groups = append(groups, fileGroup{file: ch.File})
			idx = len(groups) - 1
		}
		if want := strings.TrimSpace(ch.Want); want != "" && !slices.Contains(groups[idx].wants, want) {
			groups[idx].wants = append(groups[idx].wants, want)
		}
		if pst.Checked && ch.Err != "" && !slices.Contains(groups[idx].errs, ch.Err) {
			groups[idx].errs = append(groups[idx].errs, ch.Err)
		}
	}
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		s := g.file
		if len(g.wants) > 0 {
			s += " · " + strings.Join(g.wants, ", ")
		}
		if len(g.errs) > 0 {
			s += " · " + strings.Join(g.errs, ", ")
		}
		out = append(out, s)
	}
	return out
}
