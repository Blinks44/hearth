package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// cliPage is "CLI & clouds": command-line tools without a page of their
// own, one table.
const cliPage = "/tools/cli"

// cliGroups are the table's groups of known commands, in its order.
var cliGroups = []struct {
	title    func() string
	commands []string
}{
	{locales.CliGroupKubernetes, []string{"kubectl", "helm"}},
	{locales.CliGroupClouds, []string{"aws", "gcloud", "az"}},
	{locales.CliGroupInfra, []string{"terraform", "tofu"}},
}

// cliProducts name what a missing command comes with.
var cliProducts = map[string]string{"kubectl": "Kubernetes", "helm": "Helm", "aws": "AWS CLI", "gcloud": "Google Cloud CLI",
	"az": "Azure CLI", "terraform": "Terraform", "tofu": "OpenTofu"}

// cliTabs are the page's tabs after All: the groups by cliGroups' index,
// clouds first, then mise's (-1).
var cliTabs = []int{1, 0, 2, -1}

// cliState is what the CLI detector found; empty before a scan.
func (a *App) cliState() inventory.CliState {
	if st, _ := a.Snapshot.Tool("cli"); st.Cli != nil {
		return *st.Cli
	}
	return inventory.CliState{}
}

// cliCopy is the copy of a command the terminal runs, else the first the
// apps find; ok is false when none was found.
func cliCopy(cli inventory.CliState, command string) (inventory.Installation, bool) {
	var ins []inventory.Installation
	for _, in := range cli.Tools {
		if in.Tool == command {
			ins = append(ins, in)
		}
	}
	if len(ins) == 0 {
		return inventory.Installation{}, false
	}
	if i := inventory.TerminalCopy(ins); i >= 0 {
		return ins[i], true
	}
	return ins[0], true
}

// cliFound counts the commands of a group found.
func cliFound(cli inventory.CliState, commands []string) int {
	n := 0
	for _, cmd := range commands {
		if _, ok := cliCopy(cli, cmd); ok {
			n++
		}
	}
	return n
}

// cliCount is the sidebar's number: the commands found and mise's tools.
func (a *App) cliCount() int {
	n := len(a.miseTools())
	for _, g := range cliGroups {
		n += cliFound(a.cliState(), g.commands)
	}
	return n
}

// miseTools are the tools mise installed without a page of their own;
// the table's own commands are left to their groups.
func (a *App) miseTools() []inventory.MiseTool {
	st, _ := a.Snapshot.Tool("mise")
	return slices.DeleteFunc(slices.Clone(st.MiseTools), func(mt inventory.MiseTool) bool {
		_, known := cliProducts[mt.Name]
		return known
	})
}

// cliContext is the current context of a tool, nil when none was read.
func cliContext(cli inventory.CliState, tool string) *inventory.CliContext {
	for i := range cli.Contexts {
		if cli.Contexts[i].Tool == tool {
			return &cli.Contexts[i]
		}
	}
	return nil
}

// cliContextText says where a tool points: "context kind-dev", "profile
// work · eu-central-1", "no project chosen".
func cliContextText(cc inventory.CliContext) string {
	switch cc.Tool {
	case "kubectl":
		return locales.CliContext(cc.Name)
	case "aws":
		return strings.Join(slices.DeleteFunc([]string{locales.CliProfile(cc.Name), cc.Region}, func(s string) bool { return s == "" }), " · ")
	case "gcloud":
		if cc.Name == "" {
			return locales.CliNoProject()
		}
		return locales.CliProject(cc.Name)
	case "az":
		if cc.Name == "" {
			return locales.CliNoSubscription()
		}
		return locales.CliSubscription(cc.Name)
	}
	return cc.Name
}

func (a *App) cliView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		t := c.Theme()
		pal := Colors(t)
		cli := a.cliState()
		tools := a.miseTools()
		known := len(tools)
		var updates []string
		for _, g := range cliGroups {
			known += len(g.commands)
			for _, cmd := range g.commands {
				if in, ok := cliCopy(cli, cmd); ok {
					if u, _ := a.packageUpdate(in); u != "" {
						updates = append(updates, cmd)
					}
				}
			}
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						ui.Icon(c, Icon("square-terminal")).Size(24, 24).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.CliLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.CleanupFound(), strconv.Itoa(a.cliCount()), locales.CliFoundCaption(known))
					updatesCaption := locales.CliNoUpdates()
					if len(updates) > 0 {
						updatesCaption = strings.Join(updates, ", ")
					}
					stateCard(c, pal.Info, locales.PkgUpdates(), strconv.Itoa(len(updates)), updatesCaption)
					stateCard(c, t.Success, locales.CliFromMise(), strconv.Itoa(len(tools)), locales.CliNoPage())
				})
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segs := []segment{{locales.TabAll(), strconv.Itoa(a.cliCount())}}
					for _, g := range cliTabs {
						if g < 0 {
							segs = append(segs, segment{locales.CliFromMise(), strconv.Itoa(len(tools))})
						} else {
							segs = append(segs, segment{cliGroups[g].title(), strconv.Itoa(cliFound(cli, cliGroups[g].commands))})
						}
					}
					segments(c, "cli-tabs", &a.cliTab, segs)
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("cli-filter"), &a.cliFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Width(200)
				})
				columns(c, 12, func() {
					if a.Snapshot == nil {
						toolNote(c, locales.CleanupWaiting())
						return
					}
					a.cliTable(c, cli, tools)
				}, func() {
					a.kubeContexts(c, cli)
					if len(cli.Contexts) > 0 {
						listCard(c, locales.CliContexts(), "", func() {
							for _, cc := range cli.Contexts {
								name := cc.Name
								if cc.Tool == "gcloud" || cc.Tool == "az" || name == "" {
									name = cliContextText(cc)
								}
								listRow(c, cc.Tool+" · "+name, false, locales.CliContextFrom(tilde(cc.File, a.Home)), func() {})
							}
						})
					}
					listCard(c, locales.CliHow(), "", func() {
						listRow(c, locales.CliHowPath(), false, locales.CliHowPathCaption(), func() {})
						listRow(c, locales.CliHowMise(), false, locales.CliHowMiseCaption(), func() {})
						listRow(c, locales.CliHowNoSignIn(), false, locales.CliHowNoSignInCaption(), func() {})
					})
				})
			})
		})
	})
}

// cliTable is the table of the chosen tab: the known commands by group,
// found or not, then mise's tools.
func (a *App) cliTable(c *ui.Context, cli inventory.CliState, tools []inventory.MiseTool) {
	pal := Colors(c.Theme())
	filter := strings.ToLower(a.cliFilter)
	match := func(name string) bool { return strings.Contains(strings.ToLower(name), filter) }
	tab := -2 // All
	if a.cliTab > 0 && a.cliTab <= len(cliTabs) {
		tab = cliTabs[a.cliTab-1]
	}
	shown := 0
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		for i, g := range cliGroups {
			if tab != -2 && tab != i {
				continue
			}
			commands := slices.DeleteFunc(slices.Clone(g.commands), func(cmd string) bool { return !match(cmd) })
			if len(commands) == 0 {
				continue
			}
			cliGroupHead(c, g.title(), cliFound(cli, commands))
			for _, cmd := range commands {
				a.cliRow(c.Key("cli:"+cmd), cli, cmd)
			}
			shown += len(commands)
		}
		if tab == -2 || tab == -1 {
			rows := slices.DeleteFunc(slices.Clone(tools), func(mt inventory.MiseTool) bool { return !match(mt.Name) })
			if len(rows) > 0 {
				cliGroupHead(c, locales.CliGroupMise(), len(rows))
			}
			for _, mt := range rows {
				a.miseToolRow(c.Key("mise:"+mt.Name), mt)
			}
			shown += len(rows)
		}
		if shown == 0 {
			cardNote(c, locales.CleanupNone())
		}
	})
}

func cliGroupHead(c *ui.Context, title string, n int) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Text(c, strings.ToUpper(title)).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
		ui.Text(c, strconv.Itoa(n)).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
	})
}

// cliRow is a known command: where it came from, where it points and an
// update its package manager has, or that it was not found.
func (a *App) cliRow(c *ui.Context, cli inventory.CliState, cmd string) {
	in, found := cliCopy(cli, cmd)
	var parts []string
	version := "–"
	if found {
		parts = append(parts, sourceName(in.Source))
		if cc := cliContext(cli, cmd); cc != nil {
			parts = append(parts, cliContextText(*cc))
		}
		if cmd == "helm" && cli.HelmRepos > 0 {
			parts = append(parts, locales.CliHelmRepos(cli.HelmRepos))
		}
		if u, _ := a.packageUpdate(in); u != "" {
			parts = append(parts, u)
		}
		version = in.Version
		if version == "" {
			version = locales.UnknownVersion()
		}
	} else {
		parts = append(parts, locales.CliMissing(cliProducts[cmd]))
	}
	cliTableRow(c, cmd, strings.Join(parts, " · "), version, func() {
		if !found {
			return
		}
		if m, pkg, ok := a.copyPackage(in); ok {
			if a.packageCan(m, pkg) {
				iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) { a.packageItems(menu, m, pkg) })
			}
		}
	})
}

// cliTableRow is a row of the table: the command, a line about it, its
// version and its actions.
func cliTableRow(c *ui.Context, name, about, version string, actions func()) {
	pal := Colors(c.Theme())
	ui.Row(c).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, name).Font(FontMono).FontSize(13)
			clipped(c, about).FontSize(10.5).TextColor(pal.TextFaint)
		})
		ui.Text(c, version).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
		if actions != nil {
			actions()
		}
	})
}

// miseToolRow is a tool of mise's: its name, where it stands and its other
// versions, and the version that runs, else the default, else the newest.
func (a *App) miseToolRow(c *ui.Context, mt inventory.MiseTool) {
	shown := 0
	for i, in := range mt.Installations {
		if in.OnPath > 0 {
			shown = i
			break
		}
		if in.Default {
			shown = i
		}
	}
	in := mt.Installations[shown]
	parts := []string{"mise"}
	switch {
	case in.OnPath > 0:
		parts = append(parts, locales.ChipInPath())
	case in.Default:
		parts = append(parts, locales.CliDefault())
	default:
		parts = append(parts, locales.CliNotSet())
	}
	if n := len(mt.Installations) - 1; n > 0 {
		var others []string
		for i, o := range mt.Installations {
			if i != shown {
				others = append(others, o.Version)
			}
		}
		parts = append(parts, locales.CliMoreVersions(n, strings.Join(others, ", ")))
	}
	cliTableRow(c, mt.Name, strings.Join(parts, " · "), in.Version, nil)
}

// kubeContexts lists every locally configured context, with explicit selection.
func (a *App) kubeContexts(c *ui.Context, cli inventory.CliState) {
	if len(cli.KubeContexts) == 0 {
		return
	}
	current := ""
	if cc := cliContext(cli, "kubectl"); cc != nil {
		current = cc.Name
	}
	in, found := cliCopy(cli, "kubectl")
	listCard(c, locales.KubeContexts(), "", func() {
		for _, cc := range cli.KubeContexts {
			namespace := cc.Namespace
			if namespace == "" {
				namespace = "default"
			}
			caption := locales.KubeContextDetails(cc.Cluster, namespace)
			listRow(c, cc.Name, true, caption, func() {
				if cc.Name == current {
					aiTag(c, locales.KubeContextCurrent(), Colors(c.Theme()).SuccessSoft, Colors(c.Theme()).SuccessText)
					return
				}
				ui.Button(c.Key("kube:"+cc.Name), locales.KubeContextSwitch()).Disabled(!found || a.BuildPlan == nil).OnClick(func() {
					a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
						return ops.KubeContextPlan(in.Path, current, cc.Name, env, run.Exec{})
					}, a.proposeOrSay)
				})
			})
		}
	})
}
