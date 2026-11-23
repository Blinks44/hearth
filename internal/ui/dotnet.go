package ui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

func (a *App) dotnet(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() { a.installHeader(c, "dotnet", a.dotnetRecommended()) }})
		st, ok := a.toolState("dotnet")
		if !ok {
			a.toolEmpty(c, "dotnet", ".NET")
			return
		}
		ins, active, sdks, runtimes := dotnetRows(st)
		rows := listed(ins)
		projs := a.toolProjects("dotnet")
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				pal := Colors(c.Theme())
				toolHead(c, "dotnet", ".NET", func() {}, locales.DotnetSubtitle(), func() {
					stateCard(c, pal.TextFaint, locales.Installed(), locales.DotnetSDKCount(sdks), locales.DotnetRuntimeCount(runtimes))
					v, dot := "—", pal.TextFaint
					if i := inventory.TerminalCopy(st.Installations); i >= 0 && st.Installations[i].Version != "" {
						v, dot = st.Installations[i].Version, c.Theme().Success
					}
					stateCard(c, dot, locales.InTerminal(), v, "dotnet · SDK")
					a.toolProjectCard(c, "dotnet", projs)
				})
				toolTabs(c, "dotnet-tabs", &a.dotnetTab, len(rows), len(projs))
				switch a.dotnetTab {
				case 0:
					columns(c, 12, func() {
						a.versionTable(c, ins, rows, active, func(in inventory.Installation) (string, bool) {
							if in.Tool == "dotnet-runtime" {
								return in.Dotnet.Runtimes[0].Name, false
							}
							for _, l := range a.dotnetLines() {
								if strings.HasPrefix(in.Version, dotnetLinePrefix(l.Version)+".") {
									return strings.ToUpper(l.Support), l.Phase == "eol"
								}
							}
							return "", false
						})
						toolNote(c, locales.DotnetNote())
						a.dotnetSelection(c, st)
						for _, in := range st.Installations {
							if in.Dotnet != nil && in.Dotnet.Err != "" {
								toolNote(c, locales.DotnetProbeFailed(in.Dotnet.Err))
							}
						}
					}, func() {
						a.dotnetAvailable(c, st)
						a.managerCard(c, st, listed(st.Installations), inventory.TerminalCopy(st.Installations), nil)
						a.dotnetEnvironment(c, st)
						if i := inventory.TerminalCopy(st.Installations); i >= 0 {
							globalsCard(c, locales.DotnetTools(), locales.DotnetNoTools(), st.Installations[i].Packages, "dotnet tool -g")
						}
					})
				case 1:
					a.dotnetAvailable(c, st)
				case 2:
					a.toolProjectsTab(c, "dotnet", st, projs)
				case 3:
					pathCard(c, "dotnet", st.Installations, inventory.TerminalCopy(st.Installations), a.Home)
					a.dotnetEnvironment(c, st)
				}
			})
		})
	})
}

func dotnetRows(st inventory.ToolState) (ins []inventory.Installation, active, sdks, runtimes int) {
	active = -1
	host := inventory.TerminalCopy(st.Installations)
	for i, in := range st.Installations {
		if in.Dotnet == nil {
			continue
		}
		for _, sdk := range in.Dotnet.SDKs {
			row := in
			row.Version, row.Path, row.RealPath = sdk.Version, sdk.Path, sdk.Path
			row.OnPath, row.InGUI = 0, 0
			row.Default = i == host && sdk.Version == in.Version
			if row.Default {
				active = len(ins)
			}
			ins = append(ins, row)
			sdks++
		}
		for _, runtime := range in.Dotnet.Runtimes {
			row := in
			row.Tool, row.Version, row.Path, row.RealPath = "dotnet-runtime", runtime.Version, runtime.Path, runtime.Path
			row.OnPath, row.InGUI = 0, 0
			row.Dotnet = &inventory.Dotnet{Runtimes: []inventory.DotnetEntry{runtime}}
			ins = append(ins, row)
			runtimes++
		}
	}
	return
}

func (a *App) dotnetSelection(c *ui.Context, st inventory.ToolState) {
	listCard(c, locales.DotnetSelection(), "global.json", func() {
		count := 0
		for _, p := range a.Projects {
			for _, s := range projects.Resolve(p, a.Snapshot, nil) {
				if s.Tool != "dotnet" {
					continue
				}
				count++
				value := "—"
				caption := ""
				if s.Terminal != nil {
					value = s.Terminal.Version
				}
				for _, ch := range s.Checks {
					caption = ch.File
					if ch.Want != "" {
						caption += " · " + ch.Want
					}
					if ch.SDK != nil && ch.SDK.RollForward != "" {
						caption += " · " + ch.SDK.RollForward
					}
					if !s.Checked {
						caption = locales.NotChecked()
					} else if ch.Err != "" {
						caption = locales.DotnetUnknown(ch.Err)
					}
				}
				listRow(c, filepath.Base(p.Dir), false, caption, func() { ui.Text(c, value).Font(FontMono).FontSize(12) })
				if s.Checked {
					if _, bad := s.Unmet(); bad {
						ui.Box(c).Padding(0, 12, 10, 12).Children(func() { warningRow(c, locales.DotnetMissing(filepath.Base(p.Dir))) })
					}
				}
			}
		}
		if count == 0 {
			cardNote(c, locales.DotnetNoProjects())
		}
		v := "—"
		if i := inventory.TerminalCopy(st.Installations); i >= 0 && st.Installations[i].Version != "" {
			v = st.Installations[i].Version
		}
		listRow(c, locales.DotnetDefault(), false, "", func() { ui.Text(c, v).Font(FontMono).FontSize(12) })
	})
}

func (a *App) dotnetEnvironment(c *ui.Context, st inventory.ToolState) {
	listCard(c, locales.DotnetEnvironment(), "", func() {
		for _, key := range []string{"DOTNET_ROOT", "DOTNET_ROOT_ARM64", "DOTNET_ROOT_X64", "DOTNET_ROOT_X86"} {
			raw := st.Vars[key]
			if key != "DOTNET_ROOT" && raw == "" {
				continue
			}
			value := tilde(raw, a.Home)
			if value == "" {
				value = locales.DotnetRootDefault()
			}
			k, p := key, raw
			listRow(c, k, true, value, func() {
				if p != "" {
					a.pathActions(c, "dotnet-"+k, p)
				}
			})
		}
		value := st.Vars["DOTNET_CLI_TELEMETRY_OPTOUT"]
		caption := "DOTNET_CLI_TELEMETRY_OPTOUT=" + value
		if value == "" {
			caption = locales.DotnetTelemetryDefault()
		}
		on := locales.DotnetTelemetryOn()
		if value == "1" || strings.EqualFold(value, "true") {
			on = locales.DotnetTelemetryOff()
		}
		listRow(c, locales.DotnetTelemetry(), false, caption, func() { ui.Text(c, on).FontSize(12).TextColor(c.Theme().TextMuted) })
	})
}

func (a *App) dotnetLines() []versions.DotnetLine {
	if a.DotnetVersions == nil {
		return nil
	}
	return a.DotnetVersions.Lines
}
func (a *App) dotnetAvailable(c *ui.Context, st inventory.ToolState) {
	listCard(c, locales.AvailableTitle(), a.availableAside(st, ""), func() {
		if a.DotnetVersions == nil {
			text := locales.VersionsLoading()
			if a.DotnetVersionsErr != "" {
				text = locales.VersionsFailed(a.DotnetVersionsErr)
			}
			cardNote(c, text)
			return
		}
		for _, l := range a.DotnetVersions.Lines {
			needed := false
			for _, p := range a.Projects {
				for _, r := range p.Requirements {
					if r.Tool == "dotnet" && strings.HasPrefix(r.Want, dotnetLinePrefix(l.Version)+".") {
						needed = true
					}
				}
			}
			if l.Phase == "eol" && !needed {
				continue
			}
			caption := strings.ToUpper(l.Support)
			if strings.Contains(l.Version, "-") {
				caption += " · " + locales.DotnetPreview()
			}
			if l.End != "" {
				caption += " · " + l.End
			}
			if l.Phase == "eol" {
				caption += " · " + locales.ChipSupportEnded()
			}
			listRow(c, "SDK "+l.Version, true, caption, func() { a.installButton(c, st, l.Version, "SDK "+l.Version) })
		}
	})
}

func dotnetLinePrefix(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return v
	}
	return strings.Join(parts[:2], ".")
}

// dotnetRecommended is the newest SDK of the newest line with long-term
// support still in it.
func (a *App) dotnetRecommended() string {
	for _, l := range a.dotnetLines() {
		if l.Support == "lts" && l.Phase == "active" && !strings.Contains(l.Version, "-") {
			return l.Version
		}
	}
	return ""
}
