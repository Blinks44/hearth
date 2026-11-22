package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const instructionsPage = "/ai/instructions"

type projectInstructionCreate struct {
	dir  string
	text string
	open bool
}

func instructionState(state string) string {
	switch state {
	case "agents":
		return locales.InstructionsAgents()
	case "claude":
		return locales.InstructionsClaude()
	case "same":
		return locales.InstructionsSame()
	case "linked":
		return locales.InstructionsLinked()
	case "different":
		return locales.InstructionsDifferent()
	case "unreadable":
		return locales.InstructionsUnreadable()
	default:
		return locales.InstructionsMissing()
	}
}

func instructionProjectState(in projects.Instructions) string {
	state := in.State()
	if state == "missing" && len(in.ExtraFiles()) > 0 {
		return locales.InstructionsOther()
	}
	return instructionState(state)
}

func (a *App) connectInstructions(dir string) {
	a.BuildPlan(func(_ sysenv.Env, _ ops.Terminal) (ops.Plan, error) { return ops.InstructionPlan(dir) }, a.proposeOrSay)
}

func (a *App) createProjectInstructions(dir, text string) {
	a.BuildPlan(func(_ sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.CreateProjectInstructionsPlan(dir, text)
	}, a.proposeOrSay)
}

func (a *App) openCreateProjectInstructions(dir string) {
	a.projectInstructionsCreate = &projectInstructionCreate{
		dir:  dir,
		text: locales.ProjectInstructionsTemplate(),
		open: true,
	}
}

func (a *App) projectInstructionsCreateDialog(c *ui.Context) {
	d := a.projectInstructionsCreate
	if d == nil || !d.open {
		return
	}
	name := filepath.Base(d.dir)
	path := filepath.Join(d.dir, "AGENTS.md")
	dialogBase(c.Key("project-instructions-create"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Clip().Background(c.Theme().Background).BorderWidth(1).BorderColor(Colors(c.Theme()).Line)
		ui.Column(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, locales.InstructionsCreateTitle(name)).FontSize(18).FontWeight(600)
			ui.Text(c, locales.InstructionsCreatePrompt(name)).FontSize(12).TextColor(c.Theme().TextMuted)
			ui.Text(c, tilde(path, a.Home)).Font(FontMono).FontSize(12)
			ui.TextArea(c.Key("project-instructions-text"), &d.text).Height(240).Font(FontMono).Label(locales.InstructionsCreateTitle(name))
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.Cancel()).OnClick(func() { d.open = false })
				ui.PrimaryButton(c, locales.GlobalInstructionsPlan()).Disabled(strings.TrimSpace(d.text) == "").OnClick(func() {
					d.open = false
					a.createProjectInstructions(d.dir, d.text)
				})
			})
		})
	})
}

func (a *App) globalInstructionAgents() []inventory.Agent {
	if a.Snapshot == nil {
		return nil
	}
	var out []inventory.Agent
	for _, ag := range a.Snapshot.Agents {
		if ag.Instructions != nil {
			out = append(out, ag)
		}
	}
	return out
}

func (a *App) instructionsView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				globalAgents := a.globalInstructionAgents()
				different, connected := 0, 0
				for _, pr := range a.Projects {
					switch pr.Instructions.State() {
					case "different":
						different++
					case "linked":
						connected++
					}
				}
				// Counters of nothing only add noise before the first project.
				var stats []aiStat
				if len(a.Projects) > 0 {
					stats = []aiStat{{strconv.Itoa(len(a.Projects)), locales.StatProjects()}, {strconv.Itoa(connected), locales.StatLinked()}, {strconv.Itoa(different), locales.StatProblems(different)}}
				}
				aiIntro(c, "file-text", locales.InstructionsTitle(), locales.InstructionsLead(), stats)
				toolNote(c, locales.InstructionsScope())
				if len(a.Projects) == 0 && len(globalAgents) == 0 {
					listFrame(c, func() { a.noProjects(c, locales.InstructionsEmpty()) })
					return
				}
				query := strings.ToLower(strings.TrimSpace(a.instructionsQuery))
				var shownGlobal []inventory.Agent
				for _, ag := range globalAgents {
					title := strings.ToLower(agentTitle(ag.ID))
					path := strings.ToLower(ag.Instructions.Native.Path)
					if query == "" || strings.Contains(title, query) || strings.Contains(strings.ToLower(ag.ID), query) || strings.Contains(path, query) {
						shownGlobal = append(shownGlobal, ag)
					}
				}
				var shownProjects []projects.Project
				for _, pr := range a.Projects {
					if query == "" || strings.Contains(strings.ToLower(pr.Dir), query) {
						shownProjects = append(shownProjects, pr)
					}
				}
				var chosenAgent *inventory.Agent
				var chosenProject *projects.Project
				for i := range shownGlobal {
					if a.instructionsChosen == "agent:"+shownGlobal[i].ID {
						chosenAgent = &shownGlobal[i]
						break
					}
				}
				if chosenAgent == nil {
					for i := range shownProjects {
						if a.instructionsChosen == "project:"+shownProjects[i].Dir || a.instructionsChosen == shownProjects[i].Dir {
							chosenProject = &shownProjects[i]
							break
						}
					}
				}
				if chosenAgent == nil && chosenProject == nil {
					if len(shownProjects) > 0 {
						chosenProject = &shownProjects[0]
					} else if len(shownGlobal) > 0 {
						chosenAgent = &shownGlobal[0]
					}
				}
				pal := Colors(c.Theme())
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
						listFrame(c, func() {
							ui.Row(c).Padding(6, 10).Children(func() {
								ui.SearchField(c.Key("instructions-search"), &a.instructionsQuery).Placeholder(locales.InstructionsSearch()).Grow(1)
							})
							if query != "" && len(shownGlobal) == 0 && len(shownProjects) == 0 {
								cardNote(c, locales.NothingMatches())
							}
							if len(shownGlobal) > 0 {
								ui.Row(c).Padding(6, 14).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
									ui.Text(c, strings.ToUpper(locales.InstructionsGlobal())).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
								})
								for i := range shownGlobal {
									ag := shownGlobal[i]
									selected := chosenAgent != nil && chosenAgent.ID == ag.ID
									title := agentTitle(ag.ID)
									path := ag.Instructions.Native.Path
									if path == "" {
										path = ag.Instructions.Shared.Path
									}
									e := ui.ButtonBase(c.Key("instructions:agent:"+ag.ID)).Padding(9, 14).Gap(10).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(title)
									if selected {
										e.Background(pal.ActiveRow)
									}
									e.Children(func() {
										ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
											ui.Text(c, title).FontSize(13.5).SingleLine()
											clipped(c, tilde(path, a.Home)).Font(FontMono).FontSize(11).TextColor(c.Theme().TextMuted)
										})
										ui.Text(c, globalInstructionState(ag.Instructions.State)).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
									})
									if e.Clicked() {
										a.instructionsChosen = "agent:" + ag.ID
									}
								}
							}
							if len(globalAgents) > 0 && (len(shownProjects) > 0 || (len(a.Projects) == 0 && query == "")) {
								ui.Row(c).Padding(6, 14).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
									ui.Text(c, strings.ToUpper(locales.InstructionsProjects())).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
								})
							}
							if len(a.Projects) == 0 && query == "" {
								a.noProjects(c, locales.InstructionsEmpty())
							}
							for i := range shownProjects {
								pr := shownProjects[i]
								selected := chosenProject != nil && chosenProject.Dir == pr.Dir
								e := ui.ButtonBase(c.Key("instructions:"+pr.Dir)).Padding(9, 14).Gap(10).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Label(filepath.Base(pr.Dir))
								if selected {
									e.Background(pal.ActiveRow)
								}
								e.Children(func() {
									ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
										ui.Text(c, filepath.Base(pr.Dir)).FontSize(13.5).SingleLine()
										clipped(c, tilde(pr.Dir, a.Home)).Font(FontMono).FontSize(11).TextColor(c.Theme().TextMuted)
									})
									ui.Text(c, instructionProjectState(pr.Instructions)).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
								})
								if e.Clicked() {
									a.instructionsChosen = "project:" + pr.Dir
								}
							}
						})
					})
					detailColumn(c).Children(func() {
						switch {
						case chosenAgent != nil:
							a.globalInstructionsCard(c, chosenAgent.ID, chosenAgent.Instructions)
						case chosenProject != nil:
							a.instructionDetail(c, *chosenProject)
						}
					})
				})
			})
		})
	})
}

func (a *App) instructionDetail(c *ui.Context, p projects.Project) {
	in := p.Instructions
	state := in.State()
	listCard(c, filepath.Base(p.Dir), instructionProjectState(in), func() {
		files := []struct {
			name string
			file projects.InstructionFile
		}{{"AGENTS.md", in.Agents}, {"CLAUDE.md", in.Claude}}
		for _, ef := range in.ExtraFiles() {
			files = append(files, struct {
				name string
				file projects.InstructionFile
			}{ef.Path, ef})
		}
		for _, f := range files {
			if !f.file.Exists {
				continue
			}
			fullPath := filepath.Join(p.Dir, filepath.FromSlash(f.name))
			listRow(c, f.name, true, tilde(fullPath, a.Home), func() {
				if a.Shell != nil && iconButton(c.Key("open:"+f.name), "external-link", f.name).Clicked() {
					a.openFile(c, fullPath)
				}
			})
			if f.file.Err != "" {
				cardNote(c, f.file.Err)
			} else {
				text := f.file.Text
				if len([]rune(text)) > 2000 {
					text = string([]rune(text)[:2000]) + "…"
				}
				ui.Text(c, text).Font(FontMono).FontSize(11).Padding(10, 16)
			}
		}
		if state == "different" {
			pal := Colors(c.Theme())
			ui.Column(c).Gap(6).Padding(10, 16).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, locales.InstructionsDiffTitle()).FontSize(12).FontWeight(600).TextColor(c.Theme().TextMuted)
				if len(in.Agents.Text) > 32*1024 || len(in.Claude.Text) > 32*1024 || strings.Count(in.Agents.Text, "\n") > 500 || strings.Count(in.Claude.Text, "\n") > 500 {
					ui.Text(c, locales.GlobalInstructionsTextLimit()).FontSize(12).TextColor(c.Theme().TextMuted)
				} else {
					ui.Scroll(c).MaxHeight(280).Children(func() {
						diff := (ops.FileEdit{Before: in.Agents.Text, After: in.Claude.Text}).PreviewDiff()
						compareDiffCard(c, "AGENTS.md ↔ CLAUDE.md", diff)
					})
				}
			})
		}
		cardNote(c, locales.InstructionsReviewText())
		canConnect := a.BuildPlan != nil && state != "missing" && state != "linked" && state != "unreadable" && !in.Agents.Link && !in.Claude.Link
		canCreate := a.BuildPlan != nil && !in.Agents.Exists && in.Agents.Err == ""
		if canConnect || canCreate {
			ui.Row(c).Gap(8).Padding(12, 16).Children(func() {
				if canConnect {
					if ui.Button(c.Key("instructions-connect:"+p.Dir), locales.InstructionsConnect()).Clicked() {
						a.connectInstructions(p.Dir)
					}
				}
				if canCreate {
					if ui.Button(c.Key("instructions-create:"+p.Dir), locales.InstructionsCreate()).Clicked() {
						a.openCreateProjectInstructions(p.Dir)
					}
				}
			})
		}
	})
}
