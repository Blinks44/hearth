package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const instructionsPage = "/ai/instructions"

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

func (a *App) connectInstructions(dir string) {
	a.BuildPlan(func(_ sysenv.Env, _ ops.Terminal) (ops.Plan, error) { return ops.InstructionPlan(dir) }, a.proposeOrSay)
}

func (a *App) instructionsView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				different, connected := 0, 0
				for _, pr := range a.Projects {
					switch pr.Instructions.State() {
					case "different":
						different++
					case "linked":
						connected++
					}
				}
				aiIntro(c, "file-text", locales.InstructionsTitle(), locales.InstructionsLead(), []aiStat{{strconv.Itoa(len(a.Projects)), locales.StatProjects()}, {strconv.Itoa(connected), locales.InstructionsLinked()}, {strconv.Itoa(different), locales.StatProblems(different)}})
				toolNote(c, locales.InstructionsScope())
				if len(a.Projects) == 0 {
					listFrame(c, func() {
						ui.Column(c).Padding(16).Gap(12).Children(func() {
							ui.Text(c, locales.NoProjectsHeading()).FontSize(14).FontWeight(600)
							ui.Text(c, locales.InstructionsEmpty()).FontSize(12).TextColor(c.Theme().TextMuted)
							if a.ChooseFolder != nil {
								ui.Button(c, locales.AddFolder()).OnClick(a.ChooseFolder)
							}
						})
					})
					return
				}
				var shown []projects.Project
				for _, pr := range a.Projects {
					if strings.Contains(strings.ToLower(pr.Dir), strings.ToLower(a.instructionsQuery)) {
						shown = append(shown, pr)
					}
				}
				chosen := 0
				for i, pr := range shown {
					if pr.Dir == a.instructionsChosen {
						chosen = i
					}
				}
				ui.Row(c).Gap(18).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
						listFrame(c, func() {
							ui.Row(c).Padding(6, 10).Children(func() {
								ui.SearchField(c.Key("instructions-search"), &a.instructionsQuery).Placeholder(locales.InstructionsSearch()).Grow(1)
							})
							if len(shown) == 0 {
								cardNote(c, locales.NothingMatches())
							}
							for i, pr := range shown {
								e := ui.ButtonBase(c.Key("instructions:"+pr.Dir)).Padding(9, 14).Gap(10).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(Colors(c.Theme()).Line).Label(filepath.Base(pr.Dir))
								if i == chosen {
									e.Background(Colors(c.Theme()).ActiveRow)
								}
								e.Children(func() {
									ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
										ui.Text(c, filepath.Base(pr.Dir)).FontSize(13.5).SingleLine()
										ui.Text(c, tilde(pr.Dir, a.Home)).Font(FontMono).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
									})
									ui.Text(c, instructionState(pr.Instructions.State())).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
								})
								if e.Clicked() {
									a.instructionsChosen = pr.Dir
								}
							}
						})
					})
					detailColumn(c).Children(func() {
						if len(shown) > 0 {
							a.instructionDetail(c, shown[chosen])
						}
					})
				})
			})
		})
	})
}

func (a *App) instructionDetail(c *ui.Context, p projects.Project) {
	in := p.Instructions
	listCard(c, filepath.Base(p.Dir), instructionState(in.State()), func() {
		for _, f := range []struct {
			name string
			file projects.InstructionFile
		}{{"AGENTS.md", in.Agents}, {"CLAUDE.md", in.Claude}} {
			if !f.file.Exists {
				continue
			}
			listRow(c, f.name, true, tilde(filepath.Join(p.Dir, f.name), a.Home), func() {
				if a.Shell != nil && iconButton(c, "external-link", f.name).Clicked() {
					a.openFile(c, filepath.Join(p.Dir, f.name))
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
		cardNote(c, locales.InstructionsReviewText())
		if state := in.State(); a.BuildPlan != nil && state != "missing" && state != "linked" && state != "unreadable" && !in.Agents.Link && !in.Claude.Link {
			ui.Row(c).Padding(12, 16).Children(func() {
				if ui.Button(c.Key("instructions-connect:"+p.Dir), locales.InstructionsConnect()).Clicked() {
					a.connectInstructions(p.Dir)
				}
			})
		}
	})
}
