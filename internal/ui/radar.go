package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) hasHearthHooks(agentID string) bool {
	if a.RadarHooksInstalled[agentID] {
		return true
	}
	if ag, ok := a.agent(agentID); ok {
		needle := "notify --agent " + agentID
		for _, h := range ag.Hooks {
			if strings.Contains(h.Command, needle) {
				return true
			}
		}
	}
	return false
}

func (a *App) planAgentHooks(agentID string, install bool) {
	if a.BuildPlan == nil || a.op.running() {
		return
	}
	title := agentTitle(agentID)
	home := a.Home
	exe := a.HearthExecutable
	a.BuildPlan(func(_ sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		if install {
			return ops.PlanInstallAgentHooks(agentID, title, home, exe)
		}
		return ops.PlanRemoveAgentHooks(agentID, title, home)
	}, func(p ops.Plan, err error) {
		if err != nil {
			a.planErr = err.Error()
			return
		}
		a.Propose(p)
	})
}

func radarSessionKey(s inventory.ActiveAgentSession) string {
	return s.Agent + ":" + s.PID + ":" + s.SessionID + ":" + s.Cwd
}

func radarStatusChip(c *ui.Context, status inventory.AgentSessionStatus) {
	t := c.Theme()
	pal := Colors(t)
	switch status {
	case inventory.AgentStatusWaiting:
		chip(c, t.Warning, pal.WarningSoft, pal.WarningText, locales.RadarStatusWaiting())
	case inventory.AgentStatusWorking:
		chip(c, t.Success, pal.SuccessSoft, pal.SuccessText, locales.RadarStatusWorking())
	case inventory.AgentStatusFinished:
		chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.RadarStatusFinished())
	default:
		chip(c, pal.TextFaint, pal.Muted, t.TextMuted, locales.RadarStatusQuiet())
	}
}

func (a *App) radarSessionProject(s inventory.ActiveAgentSession) (label, projDir string) {
	if s.Cwd == "" {
		return "", ""
	}
	if dir := a.portProject([]string{s.Cwd}); dir != "" {
		return filepath.Base(dir), dir
	}
	if a.Home != "" && filepath.Clean(s.Cwd) == filepath.Clean(a.Home) {
		return "", ""
	}
	return filepath.Base(s.Cwd), ""
}

func (a *App) supportedInstalledHookAgents() []inventory.Agent {
	var out []inventory.Agent
	for _, ag := range a.foundAgents() {
		if slices.Contains(ops.SupportedHookAgents, ag.ID) {
			out = append(out, ag)
		}
	}
	return out
}

// agentRadarCard displays active AI coding sessions (waiting for approval,
// working, quiet, finished) and opt-in lifecycle hook setup controls.
func (a *App) agentRadarCard(c *ui.Context) {
	supported := a.supportedInstalledHookAgents()
	if len(a.RadarSessions) == 0 && len(supported) == 0 {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	waiting := a.WaitingAgentsCount()
	todayCost := a.TodayUsageCostUSD()

	ui.Column(c.Key("agent-radar-card")).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		// Card header: title + sessions/waiting summary chip + right today's cost & refresh button.
		ui.Row(c).Gap(8).Padding(10, 14).MinHeight(42).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.RadarCardTitle()).FontSize(13.5).FontWeight(600).SingleLine()
			if len(a.RadarSessions) > 0 {
				summary := locales.RadarSessionsCount(len(a.RadarSessions))
				if waiting > 0 {
					summary += " · " + locales.TrayPopoverWaitingChip(waiting)
					chip(c, t.Warning, pal.WarningSoft, pal.WarningText, summary)
				} else {
					aiTag(c, summary, pal.Muted, t.TextMuted)
				}
			}
			ui.Box(c).Grow(1).MinWidth(0).Height(1)
			if todayCost > 0 {
				ui.Text(c, locales.TrayPopoverTodayCost(usageAxisCost(todayCost))).
					Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
			}
			if a.RefreshRadar != nil {
				ui.Button(c.Key("radar-refresh"), "").Size(26, 26).Padding(0).Radius(6).
					Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
					Disabled(a.RadarRefreshing).
					Tooltip(locales.RadarRefresh()).
					OnClick(func() { a.RefreshRadarNow() }).
					Children(func() {
						spinIcon(c, "refresh-cw", 13, t.TextMuted, a.RadarRefreshing)
					})
			}
		})

		if len(a.RadarSessions) == 0 {
			cardNote(c, locales.RadarEmpty())
		} else {
			seenKeys := map[string]int{}
			for _, sess := range a.RadarSessions {
				s := sess
				baseKey := radarSessionKey(s)
				seenKeys[baseKey]++
				key := baseKey
				if seenKeys[baseKey] > 1 {
					key += "#" + strconv.Itoa(seenKeys[baseKey])
				}
				projLabel, projDir := a.radarSessionProject(s)
				var meta []string
				if s.Cwd != "" {
					meta = append(meta, tilde(s.Cwd, a.Home))
				}
				if s.Exact {
					meta = append(meta, locales.RadarExactHint())
				} else {
					meta = append(meta, locales.RadarPassiveHint())
				}
				if s.Model != "" {
					meta = append(meta, s.Model)
				}
				if s.Tokens > 0 {
					meta = append(meta, usageCompactTokens(s.Tokens))
				}
				if s.PID != "" && s.Model == "" && s.Tokens == 0 {
					meta = append(meta, locales.PortsPID(s.PID))
				}
				if !s.UpdatedAt.IsZero() {
					meta = append(meta, ago(c, s.UpdatedAt))
				}

				row := ui.Row(c.Key("radar-row:"+key)).
					Gap(10).Padding(10, 14).MinHeight(54).AlignItems(ui.Center).
					BorderWidth(1, 0, 0, 0).BorderColor(pal.Line)
				if s.Status == inventory.AgentStatusWaiting {
					row.Background(pal.WarningSoft.Alpha(0.45))
				}
				row.Children(func() {
					agentTile(c, s.Agent, 28)
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						title := agentTitle(s.Agent)
						if projLabel != "" {
							title += " · " + projLabel
						}
						clipped(c, title).FontSize(13).FontWeight(600)
						if len(meta) > 0 {
							metaColor := t.TextMuted
							if s.Status == inventory.AgentStatusWaiting {
								metaColor = pal.WarningText
							}
							clipped(c, strings.Join(meta, " · ")).FontSize(11.5).TextColor(metaColor)
						}
					})
					radarStatusChip(c, s.Status)
					if s.CostUSD > 0 {
						ui.Text(c, usageAxisCost(s.CostUSD)).Font(FontMono).FontSize(12).FontWeight(500).TextColor(t.Text).SingleLine()
					}
					var onOpen func()
					if projDir != "" {
						dir := projDir
						onOpen = func() { a.OpenProject(dir) }
					} else if _, ok := a.agent(s.Agent); ok {
						id := s.Agent
						onOpen = func() { a.router.Push(agentPath(id)) }
					}
					if onOpen != nil {
						if s.Status == inventory.AgentStatusWaiting {
							ui.PrimaryButton(c.Key("radar-act:"+key), locales.RadarActionGo()).
								Height(28).OnClick(onOpen)
						} else {
							ui.Button(c.Key("radar-act:"+key), locales.RadarActionDetails()).
								Height(28).OnClick(onOpen)
						}
					}
				})
			}
		}

		// Compact privacy & hooks footer strip matching Sketch 07.
		if len(supported) > 0 && a.BuildPlan != nil {
			ui.Row(c.Key("radar-hooks-strip")).Gap(10).Padding(9, 14).MinHeight(42).AlignItems(ui.Center).
				Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					ui.Text(c, locales.RadarHooksPrivacyLine()).FontSize(12).TextColor(t.TextMuted).Wrap()
					var installedAgents []inventory.Agent
					for _, ag := range supported {
						if a.hasHearthHooks(ag.ID) {
							installedAgents = append(installedAgents, ag)
						}
					}
					if len(installedAgents) > 0 {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
							for _, ag := range installedAgents {
								aiTag(c, agentTitle(ag.ID)+" · "+locales.RadarHooksInstalled(), pal.SuccessSoft, pal.SuccessText)
							}
						})
					}
				})
				if len(supported) == 1 && !a.hasHearthHooks(supported[0].ID) {
					agentID := supported[0].ID
					ui.Button(c.Key("radar-hook-add:"+agentID), locales.RadarHooksConnect()).
						Height(28).OnClick(func() { a.planAgentHooks(agentID, true) })
				} else {
					btn := ui.Button(c.Key("radar-hooks-menu"), locales.RadarHooksConnect()).Height(28)
					btn.PopoverMenuButton(func(m *ui.Menu) {
						for _, ag := range supported {
							agentID := ag.ID
							installed := a.hasHearthHooks(agentID)
							label := agentTitle(agentID) + " · " + locales.RadarHooksInstall()
							if installed {
								label = agentTitle(agentID) + " · " + locales.RadarHooksRemove()
							}
							if m.Item(label).Checked(installed).Chosen() {
								a.planAgentHooks(agentID, !installed)
							}
						}
					}, ui.PopoverMenuButtonOptions{
						At:   ui.AnchorBottomEnd,
						Self: ui.AnchorTopEnd,
					})
				}
			})
		}
	})
}
