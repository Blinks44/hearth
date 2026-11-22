package ui

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type globalInstructionPreview struct {
	agent                     string
	open, create, loading     bool
	text, shared, native, err string
	files                     inventory.GlobalInstructions
}

func globalInstructionState(state string) string {
	switch state {
	case "shared":
		return locales.GlobalInstructionsShared()
	case "native":
		return locales.GlobalInstructionsReady()
	case "override":
		return locales.GlobalInstructionsOverride()
	case "linked":
		return locales.GlobalInstructionsLinked()
	case "same":
		return locales.GlobalInstructionsSame()
	case "different":
		return locales.InstructionsDifferent()
	case "unreadable":
		return locales.InstructionsUnreadable()
	case "empty":
		return locales.GlobalInstructionsEmpty()
	default:
		return locales.InstructionsMissing()
	}
}

func (a *App) globalInstructionsCard(c *ui.Context, agentID string, in *inventory.GlobalInstructions) {
	if in == nil {
		return
	}
	name := agentTitle(agentID)
	t, pal := c.Theme(), Colors(c.Theme())
	listFrame(c, func() {
		ui.Row(c).Gap(8).Padding(12, 14, 10, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.GlobalInstructionsTitle()).FontSize(14).FontWeight(600).Grow(1).SingleLine()
			switch in.State {
			case "linked":
				chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.GlobalInstructionsChipLinked())
			case "native", "same":
				chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.GlobalInstructionsChipReady())
			case "shared":
				chip(c, Accent, pal.AccentSoft, pal.AccentInk, locales.GlobalInstructionsChipShared())
			case "missing":
				chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.GlobalInstructionsChipMissing())
			default:
				chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.GlobalInstructionsChipCheck())
			}
		})
		// One sentence says what the state means and what to do.
		lead := globalInstructionState(in.State)
		switch in.State {
		case "linked":
			if in.Native.Link {
				lead = locales.GlobalInstructionsLinkedLead(name)
			} else {
				lead = locales.GlobalInstructionsImportedLead(name)
			}
		case "shared":
			lead = locales.GlobalInstructionsLead(name)
		}
		ui.Text(c, lead).FontSize(12.5).TextColor(t.TextMuted).Padding(0, 14, 12, 14)
		for i, row := range []struct {
			label string
			f     inventory.InstructionFile
		}{
			{locales.GlobalInstructionsSharedFile(), in.Shared},
			{locales.GlobalInstructionsAgentFile(), in.Native},
			{locales.GlobalInstructionsOverrideFile(), in.Override},
		} {
			if i == 2 && (row.f.Path == "" || (!row.f.Exists && row.f.Err == "")) {
				continue
			}
			status := locales.GlobalInstructionsNotFound()
			switch {
			case row.f.Err != "":
				status = locales.InstructionsUnreadable()
			case i == 1 && in.State == "linked":
				if row.f.Link {
					status = locales.GlobalInstructionsLinkFile()
				} else {
					status = locales.GlobalInstructionsImportedFile()
				}
			case in.Active != "" && row.f.Path == in.Active:
				status = locales.GlobalInstructionsActive(name)
			case row.f.Exists && row.f.Empty:
				status = locales.GlobalInstructionsEmptyFile()
			case row.f.Exists:
				status = locales.GlobalInstructionsFound()
			}
			ui.Row(c.Key(row.label)).Gap(10).Padding(8, 12, 8, 14).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, row.label).FontSize(12.5).Width(136).Shrink(0).SingleLine()
				clipped(c, tilde(row.f.Path, a.Home)).Font(FontMono).FontSize(12).Grow(1)
				color := pal.TextFaint
				if in.Active != "" && row.f.Path == in.Active {
					color = t.TextMuted
				}
				ui.Text(c, status).FontSize(11.5).TextColor(color).SingleLine()
				if row.f.Exists && a.Shell != nil {
					path := row.f.Path
					iconButton(c, "external-link", locales.OpenInEditor()).OnClick(func() { a.openFile(c, path) })
				} else {
					ui.Box(c).Size(24, 24).Shrink(0)
				}
			})
		}
		// A linked file is the shared one: comparing it with itself says nothing.
		compare := in.Shared.Exists && in.Native.Exists && in.State != "unreadable" && in.State != "linked"
		if in.State != "shared" && in.State != "missing" && !compare {
			return
		}
		ui.Row(c).Gap(8).Padding(10, 14, 12, 14).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
			switch in.State {
			case "shared":
				ui.PrimaryButton(c.Key("global-connect:"+agentID), locales.GlobalInstructionsConnect()).Disabled(a.BuildPlan == nil).OnClick(func() { a.globalInstructionPlan(agentID, "link", "") })
			case "missing":
				ui.Button(c.Key("global-create:"+agentID), locales.GlobalInstructionsCreate()).Disabled(a.BuildPlan == nil).OnClick(func() { a.previewGlobalInstructions(agentID, true) })
			}
			if compare {
				ui.Button(c.Key("global-compare:"+agentID), locales.GlobalInstructionsCompare()).Disabled(a.BuildPlan == nil).OnClick(func() { a.previewGlobalInstructions(agentID, false) })
			}
		})
	})
}

func (a *App) globalInstructionPlan(agentID, mode, text string) {
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.GlobalInstructionPlan(agentID, agentTitle(agentID), env, mode, text)
	}, a.proposeOrSay)
}

func (a *App) previewGlobalInstructions(agentID string, create bool) {
	p := &globalInstructionPreview{agent: agentID, open: true, create: create, loading: true}
	a.globalInstructions = p
	var files inventory.GlobalInstructions
	var shared, native string
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		var ok bool
		files, ok = ai.AgentInstructions(agentID, env)
		if !ok {
			return ops.Plan{}, errors.New(locales.GlobalInstructionsChanged())
		}
		var err error
		if !create {
			shared, err = ai.InstructionText(files.Shared.Path)
			if err == nil {
				native, err = ai.InstructionText(files.Native.Path)
			}
			// The shared diff widget computes a line LCS on the UI thread.
			// Keep this preview within the same bounds as operation plans.
			if err == nil && (len(shared) > 32*1024 || len(native) > 32*1024 || strings.Count(shared, "\n") > 500 || strings.Count(native, "\n") > 500) {
				err = errors.New(locales.GlobalInstructionsTextLimit())
			}
		}
		return ops.Plan{}, err
	}, func(_ ops.Plan, err error) {
		if a.globalInstructions != p {
			return
		}
		p.loading = false
		p.files, p.shared, p.native = files, shared, native
		if err != nil {
			p.err = err.Error()
		}
		if create {
			p.text = locales.GlobalInstructionsTemplate()
		}
	})
}

func (a *App) globalInstructionsDialog(c *ui.Context) {
	p := a.globalInstructions
	if p == nil || !p.open {
		return
	}
	name := agentTitle(p.agent)
	dialogBase(c.Key("global-instructions"), &p.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Clip().Background(c.Theme().Background).BorderWidth(1).BorderColor(Colors(c.Theme()).Line)
		ui.Column(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, locales.GlobalInstructionsTitle()).FontSize(18).FontWeight(600)
			switch {
			case p.loading:
				ui.Text(c, locales.OpVerifying()).FontSize(12)
			case p.err != "":
				ui.Text(c, p.err).FontSize(12)
			case p.create:
				ui.Text(c, locales.GlobalInstructionsCreateWhy(filepath.Base(p.files.Native.Path), name)).FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Text(c, tilde(p.files.Native.Path, a.Home)).Font(FontMono).FontSize(12)
				ui.TextArea(c.Key("global-instructions-text"), &p.text).Height(240).Font(FontMono).Label(locales.GlobalInstructionsTitle())
			default:
				ui.Text(c, locales.GlobalInstructionsCompareWhy(name)).FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Scroll(c).MaxHeight(340).Children(func() {
					a.editDiff(c, ops.FileEdit{Path: tilde(p.files.Native.Path, a.Home) + " ↔ " + tilde(p.files.Shared.Path, a.Home), Before: p.native, After: p.shared}, true)
				})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.Cancel()).OnClick(func() { p.open = false })
				if p.create {
					ui.PrimaryButton(c, locales.GlobalInstructionsPlan()).Disabled(p.loading || p.err != "" || strings.TrimSpace(p.text) == "").OnClick(func() { p.open = false; a.globalInstructionPlan(p.agent, "create", p.text) })
				}
			})
		})
	})
}

type sharedAgentOption struct {
	id          string
	name        string
	nativePath  string
	hasFile     bool
	isLink      bool
	isEmpty     bool
	hasOverride bool
	migrable    bool
	selected    bool
}

type sharedInstructionsSetup struct {
	open, loading bool
	mode          int // 0 = template, 1 = migrate existing file
	sharedPath    string
	sourceAgent   string
	templateText  string
	sourceTexts   map[string]string
	sourceErrs    map[string]string
	agents        []sharedAgentOption
	err           string
}

type globalMarkdownCache struct {
	path, sum string
	loading   bool
	text, err string
}

// sharedInstructionsCandidates returns installed agents with file-based global
// instructions when ~/.agents/AGENTS.md does not yet exist.
func (a *App) sharedInstructionsCandidates() []inventory.Agent {
	if a.Snapshot == nil {
		return nil
	}
	var out []inventory.Agent
	for _, ag := range a.foundAgents() {
		if ag.Instructions == nil || ag.Instructions.Shared.Path == "" {
			continue
		}
		if ag.Instructions.Shared.Exists || ag.Instructions.Shared.Err != "" {
			return nil
		}
		out = append(out, ag)
	}
	return out
}

// sharedInstructionsBanner shows a calm, unobtrusive suggestion to create or
// migrate to ~/.agents/AGENTS.md when at least one agent is installed and the
// shared file does not exist yet.
func (a *App) sharedInstructionsBanner(c *ui.Context) {
	if a.BuildPlan == nil {
		return
	}
	candidates := a.sharedInstructionsCandidates()
	if len(candidates) == 0 {
		return
	}
	var migratePath string
	for _, ag := range candidates {
		in := ag.Instructions
		if in.Native.Exists && !in.Native.Link && !in.Native.Empty && in.Native.Err == "" && !in.Override.Exists {
			migratePath = tilde(in.Native.Path, a.Home)
			break
		}
	}
	desc := locales.SharedInstructionsBannerText()
	if migratePath != "" {
		desc = locales.SharedInstructionsBannerMigrateText(migratePath)
	}
	t := c.Theme()
	pal := Colors(t)
	row := ui.Row(c.Key("shared-instructions-banner")).Gap(12).Padding(12, 16).Radius(12).AlignItems(ui.Center).
		Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line)
	if narrow(c) {
		row.Wrap()
	}
	row.Children(func() {
		ui.Box(c).Size(32, 32).Shrink(0).Radius(8).Center().Background(pal.AccentSoft).Children(func() {
			ui.Icon(c, Icon("file-text")).Size(16, 16).TextColor(Accent)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, locales.SharedInstructionsBannerTitle()).FontSize(13).FontWeight(600)
			ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Button(c.Key("shared-instructions-setup"), locales.SharedInstructionsSetup()).Height(controlHeight).
			OnClick(func() { a.openSharedInstructionsSetup() })
	})
}

func (a *App) openSharedInstructionsSetup() {
	candidates := a.sharedInstructionsCandidates()
	if len(candidates) == 0 || a.BuildPlan == nil {
		return
	}
	ids := make([]string, 0, len(candidates))
	for _, ag := range candidates {
		ids = append(ids, ag.ID)
	}
	s := &sharedInstructionsSetup{
		open:         true,
		loading:      true,
		templateText: locales.GlobalInstructionsTemplate(),
		sourceTexts:  map[string]string{},
		sourceErrs:   map[string]string{},
	}
	a.sharedInstructions = s
	var loaded []sharedAgentOption
	var sharedPath string
	sourceTexts := map[string]string{}
	sourceErrs := map[string]string{}
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		for _, id := range ids {
			in, ok := ai.AgentInstructions(id, env)
			if !ok {
				continue
			}
			if in.Shared.Exists || in.Shared.Err != "" {
				return ops.Plan{}, errors.New(locales.GlobalInstructionsChanged())
			}
			if sharedPath == "" {
				sharedPath = in.Shared.Path
			}
			migrable := in.Native.Exists && !in.Native.Link && !in.Native.Empty && in.Native.Err == "" && !in.Override.Exists
			if migrable {
				txt, err := ai.InstructionText(in.Native.Path)
				if err != nil {
					sourceErrs[id] = err.Error()
					migrable = false
				} else if len(txt) > 32*1024 || strings.Count(txt, "\n") > 500 {
					sourceErrs[id] = locales.GlobalInstructionsTextLimit()
					migrable = false
				} else {
					sourceTexts[id] = txt
				}
			}
			canLink := !in.Native.Exists && in.Native.Err == "" && !in.Override.Exists
			loaded = append(loaded, sharedAgentOption{
				id:          id,
				name:        agentTitle(id),
				nativePath:  in.Native.Path,
				hasFile:     in.Native.Exists,
				isLink:      in.Native.Link,
				isEmpty:     in.Native.Empty,
				hasOverride: in.Override.Exists,
				migrable:    migrable,
				selected:    canLink,
			})
		}
		if sharedPath == "" {
			return ops.Plan{}, errors.New(locales.GlobalInstructionsChanged())
		}
		return ops.Plan{}, nil
	}, func(_ ops.Plan, err error) {
		if a.sharedInstructions != s {
			return
		}
		s.loading = false
		if err != nil {
			s.err = err.Error()
			return
		}
		s.sharedPath = sharedPath
		s.agents = loaded
		s.sourceTexts = sourceTexts
		s.sourceErrs = sourceErrs
		for _, opt := range loaded {
			if opt.migrable {
				s.mode = 1
				s.sourceAgent = opt.id
				break
			}
		}
	})
}

func (a *App) proposeSharedInstructions(req ops.SharedInstructionsRequest) {
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.SharedInstructionsPlan(req, env)
	}, a.proposeOrSay)
}

func (a *App) sharedInstructionsDialog(c *ui.Context) {
	s := a.sharedInstructions
	if s == nil || !s.open {
		return
	}
	t, pal := c.Theme(), Colors(c.Theme())
	dialogBase(c.Key("shared-instructions"), &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(640).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Line)
		ui.Column(c).Padding(20).Gap(12).Children(func() {
			ui.Text(c, locales.SharedInstructionsDialogTitle()).FontSize(18).FontWeight(600)
			if s.loading {
				ui.Text(c, locales.OpVerifying()).FontSize(12).TextColor(t.TextMuted)
			} else if s.err != "" {
				ui.Text(c, s.err).FontSize(12).TextColor(pal.DangerText)
			} else {
				var migrable []sharedAgentOption
				for _, opt := range s.agents {
					if opt.migrable {
						migrable = append(migrable, opt)
					}
				}
				if len(migrable) > 0 {
					segments(c, "shared-inst-mode", &s.mode, []segment{
						{locales.SharedInstructionsModeTemplate(), ""},
						{locales.SharedInstructionsModeMigrate(), ""},
					})
				} else {
					s.mode = 0
				}
				if s.mode == 1 && len(migrable) > 0 {
					if s.sourceAgent == "" {
						s.sourceAgent = migrable[0].id
					}
					ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
						ui.Text(c, locales.SharedInstructionsSourceLabel()).FontSize(12).TextColor(t.TextMuted).SingleLine()
						for _, opt := range migrable {
							active := s.sourceAgent == opt.id
							btn := ui.ButtonBase(c.Key("src:"+opt.id)).Gap(6).Padding(4, 10).Radius(8).
								AlignItems(ui.Center).BorderWidth(1).Label(opt.name)
							fg := t.Text
							if active {
								btn.Background(t.Text).BorderColor(t.Text)
								fg = t.Background
							} else {
								btn.Background(t.Background).BorderColor(pal.ControlBorder)
							}
							btn.Children(func() {
								ui.Text(c, opt.name).FontSize(12).TextColor(fg).SingleLine()
								ui.Text(c, filepath.Base(opt.nativePath)).Font(FontMono).FontSize(11).TextColor(fg.Alpha(0.75)).SingleLine()
							})
							if btn.Clicked() {
								s.sourceAgent = opt.id
							}
						}
					})
				}
				ui.Text(c, tilde(s.sharedPath, a.Home)).Font(FontMono).FontSize(12).TextColor(t.TextMuted)
				textPtr := &s.templateText
				areaKey := "shared-inst-template-text"
				if s.mode == 1 && s.sourceAgent != "" {
					cur := s.sourceTexts[s.sourceAgent]
					textPtr = &cur
					areaKey = "shared-inst-migrate-text:" + s.sourceAgent
					ui.TextArea(c.Key(areaKey), textPtr).Height(150).Font(FontMono).Label(locales.SharedInstructionsDialogTitle())
					s.sourceTexts[s.sourceAgent] = cur
				} else {
					ui.TextArea(c.Key(areaKey), textPtr).Height(150).Font(FontMono).Label(locales.SharedInstructionsDialogTitle())
				}
				ui.Text(c, locales.SharedInstructionsAgentsLabel()).FontSize(12.5).FontWeight(600)
				ui.Scroll(c).MaxHeight(130).Children(func() {
					ui.Column(c).Radius(10).Clip().BorderWidth(1).BorderColor(pal.Line).Children(func() {
						for i := range s.agents {
							opt := &s.agents[i]
							isMigrated := s.mode == 1 && opt.id == s.sourceAgent
							blocked := !isMigrated && (opt.hasFile || opt.hasOverride)
							ui.Row(c.Key("shared-agent:"+opt.id)).Gap(10).Padding(8, 12).AlignItems(ui.Center).
								BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
								checked := opt.selected
								if isMigrated {
									checked = true
								} else if blocked {
									checked = false
								}
								if ui.Checkbox(c.Key("chk:"+opt.id), &checked, "").Label(opt.name).Disabled(isMigrated || blocked).Changed() {
									opt.selected = checked
								}
								agentTile(c, opt.id, 22)
								ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
									ui.Text(c, opt.name).FontSize(12.5).FontWeight(500).SingleLine()
									clipped(c, tilde(opt.nativePath, a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint)
								})
								note := locales.SharedInstructionsAgentLinkNote()
								if isMigrated {
									note = locales.SharedInstructionsAgentMigrateNote()
								} else if blocked {
									note = locales.SharedInstructionsAgentExistsNote()
								}
								ui.Text(c, note).FontSize(11).TextColor(t.TextMuted)
							})
						}
					})
				})
			}
			activeText := s.templateText
			if s.mode == 1 && s.sourceAgent != "" {
				activeText = s.sourceTexts[s.sourceAgent]
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				ui.Button(c, locales.Cancel()).OnClick(func() { s.open = false })
				ui.PrimaryButton(c, locales.GlobalInstructionsPlan()).
					Disabled(s.loading || s.err != "" || strings.TrimSpace(activeText) == "").
					OnClick(func() {
						req := ops.SharedInstructionsRequest{Text: activeText}
						if s.mode == 1 && s.sourceAgent != "" {
							req.MigrateFrom = s.sourceAgent
							req.MigrateFromName = agentTitle(s.sourceAgent)
						}
						for _, opt := range s.agents {
							if s.mode == 1 && opt.id == s.sourceAgent {
								continue
							}
							if opt.selected && !opt.hasFile && !opt.hasOverride {
								req.LinkAgents = append(req.LinkAgents, opt.id)
							}
						}
						s.open = false
						a.proposeSharedInstructions(req)
					})
			})
		})
	})
}

// globalInstructionMarkdownCard renders a read-only Markdown preview of the
// agent's active (or available shared) global instruction file, loaded off the
// UI thread via ReadInstruction.
func (a *App) globalInstructionMarkdownCard(c *ui.Context, agentID string, in *inventory.GlobalInstructions) {
	if in == nil || a.ReadInstruction == nil {
		return
	}
	var path, sum string
	switch {
	case in.Active != "" && in.Active == in.Native.Path && in.Native.Exists && in.Native.Err == "":
		path, sum = in.Native.Path, in.Native.Sum
	case in.Active != "" && in.Active == in.Override.Path && in.Override.Exists && in.Override.Err == "":
		path, sum = in.Override.Path, in.Override.Sum
	case in.Native.Exists && in.Native.Err == "":
		path, sum = in.Native.Path, in.Native.Sum
	case in.Shared.Exists && in.Shared.Err == "":
		path, sum = in.Shared.Path, in.Shared.Sum
	default:
		return
	}
	if a.globalPreviewCache == nil {
		a.globalPreviewCache = map[string]*globalMarkdownCache{}
	}
	key := agentID + ":" + path
	entry := a.globalPreviewCache[key]
	if entry == nil || entry.sum != sum {
		entry = &globalMarkdownCache{path: path, sum: sum, loading: true}
		a.globalPreviewCache[key] = entry
		cur := entry
		a.ReadInstruction(path, func(txt string, err error) {
			if a.globalPreviewCache[key] != cur {
				return
			}
			cur.loading = false
			if err != nil {
				cur.err = err.Error()
			} else {
				cur.text = txt
			}
		})
	}
	listCard(c, locales.InstructionsPreview(), tilde(path, a.Home), func() {
		switch {
		case entry.loading:
			cardNote(c, locales.OpVerifying())
		case entry.err != "":
			cardNote(c, entry.err)
		default:
			a.instructionMarkdownBody(c, "global-md:"+key, entry.text)
		}
	})
}
