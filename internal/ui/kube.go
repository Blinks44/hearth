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

// kubePage is Kubernetes: kubeconfig's contexts by file, kubectl and Helm.
const kubePage = "/tools/kubernetes"

// kubeCommands are the Kubernetes tools the page lists, which CLI & clouds
// leaves to it.
var kubeCommands = []string{"kubectl", "helm"}

// kubeNamespaceDialog asks for the namespace of a context.
type kubeNamespaceDialog struct {
	open                bool
	context, from, file string
	value               string
}

// kubeCurrent is kubectl's current context, nil when none is chosen.
func kubeCurrent(cli inventory.CliState) *inventory.KubeContext {
	cc := cliContext(cli, "kubectl")
	if cc == nil {
		return nil
	}
	for i, k := range cli.KubeContexts {
		if k.Name == cc.Name && !k.Shadowed {
			return &cli.KubeContexts[i]
		}
	}
	// A current context no file defines.
	return &inventory.KubeContext{Name: cc.Name, File: cc.File}
}

// kubeContextCount is how many contexts kubectl can use: a shadowed one
// is not among them.
func kubeContextCount(cli inventory.CliState) int {
	n := 0
	for _, k := range cli.KubeContexts {
		if !k.Shadowed {
			n++
		}
	}
	return n
}

// namespaceOf is the namespace a context selects, default when it names none.
func namespaceOf(k inventory.KubeContext) string {
	if k.Namespace == "" {
		return "default"
	}
	return k.Namespace
}

func (a *App) kubeView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		a.kubeNamespaceDialog(c)
		t := c.Theme()
		pal := Colors(t)
		cli := a.cliState()
		current := kubeCurrent(cli)
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						iconOrLogo(c, "brand/kubernetes", 26, t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						lead := p.subtitle()
						if len(cli.KubeFiles) > 0 {
							lead = locales.KubeLead(len(cli.KubeFiles))
						}
						ui.Text(c, lead).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					stateCard(c, pal.TextFaint, locales.KubeContextsCard(), strconv.Itoa(kubeContextCount(cli)), locales.KubeFromFiles(len(cli.KubeFiles)))
					if current != nil {
						stateCard(c, t.Success, locales.KubeKubectlNow(), current.Name, locales.KubeNamespace(namespaceOf(*current)))
					} else {
						stateCard(c, pal.TextFaint, locales.KubeKubectlNow(), "–", locales.KubeNoCurrent())
					}
					var versions []string
					for _, cmd := range kubeCommands {
						if in, ok := cliCopy(cli, cmd); ok {
							versions = append(versions, cmd+" "+in.Version)
						}
					}
					caption := strings.Join(versions, " · ")
					if caption == "" {
						caption = locales.KubeNoTools()
					}
					stateCard(c, pal.TextFaint, locales.KubeToolsTab(), strconv.Itoa(len(versions)), caption)
				})
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "kube-tabs", &a.kubeTab, []segment{
						{locales.KubeContextsCard(), strconv.Itoa(kubeContextCount(cli))},
						{locales.KubeToolsTab(), strconv.Itoa(cliFound(cli, kubeCommands))},
						{locales.KubeFilesTab(), strconv.Itoa(len(cli.KubeFiles))},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("kube-filter"), &a.kubeFilter).Bind(&a.filterField).Placeholder(locales.KubeFilter()).Width(220)
				})
				columns(c, 12, func() {
					if a.Snapshot == nil {
						toolNote(c, locales.CleanupWaiting())
						return
					}
					switch a.kubeTab {
					case 1:
						a.kubeTools(c, cli)
					case 2:
						a.kubeFiles(c, cli)
					default:
						a.kubeContextTable(c, cli, current)
					}
				}, func() {
					a.kubeTerminalCard(c, cli, current)
					listCard(c, locales.KubeHow(), "", func() {
						listRow(c, locales.KubeHowScan(), false, locales.KubeHowScanCaption(), func() {})
						listRow(c, locales.KubeHowPlan(), false, locales.KubeHowPlanCaption(), func() {})
						listRow(c, locales.KubeHowSecrets(), false, locales.KubeHowSecretsCaption(), func() {})
					})
				})
			})
		})
	})
}

// kubeMatch reports that a row fits the page's filter.
func (a *App) kubeMatch(words ...string) bool {
	filter := strings.ToLower(strings.TrimSpace(a.kubeFilter))
	return filter == "" || slices.ContainsFunc(words, func(w string) bool { return strings.Contains(strings.ToLower(w), filter) })
}

// kubeContextTable lists the contexts by kubeconfig file, in kubectl's
// merge order.
func (a *App) kubeContextTable(c *ui.Context, cli inventory.CliState, current *inventory.KubeContext) {
	pal := Colors(c.Theme())
	shown := 0
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		for _, f := range cli.KubeFiles {
			rows := slices.DeleteFunc(slices.Clone(cli.KubeContexts), func(k inventory.KubeContext) bool {
				return k.File != f.Path || !a.kubeMatch(k.Name, k.Cluster, k.Namespace)
			})
			if len(rows) == 0 {
				continue
			}
			cliGroupHead(c, tilde(f.Path, a.Home), len(rows))
			for _, k := range rows {
				a.kubeContextRow(c.Key("kube:"+k.File+":"+k.Name), cli, k, current)
			}
			shown += len(rows)
		}
		if shown == 0 {
			text := locales.KubeNoContexts()
			if len(cli.KubeContexts) > 0 {
				text = locales.CleanupNone()
			}
			cardNote(c, text)
		}
	})
}

// kubeContextRow is a context: its cluster and namespace, whether kubectl
// uses it, and the plans to use it or change its namespace.
func (a *App) kubeContextRow(c *ui.Context, cli inventory.CliState, k inventory.KubeContext, current *inventory.KubeContext) {
	pal := Colors(c.Theme())
	isCurrent := current != nil && !k.Shadowed && k.Name == current.Name
	parts := []string{locales.KubeCluster(k.Cluster), locales.KubeNamespace(namespaceOf(k))}
	if k.Shadowed {
		parts = []string{locales.KubeShadowedCaption()}
	}
	in, found := cliCopy(cli, "kubectl")
	plans := found && a.BuildPlan != nil && !k.Shadowed
	ui.Row(c).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, k.Name).FontSize(13)
			clipped(c, strings.Join(parts, " · ")).FontSize(10.5).TextColor(pal.TextFaint)
		})
		switch {
		case isCurrent:
			aiTag(c, locales.KubeInKubectl(), pal.SuccessSoft, pal.SuccessText)
		case k.Shadowed:
			aiTag(c, locales.KubeShadowed(), pal.Muted, c.Theme().TextMuted)
		default:
			from := ""
			if current != nil {
				from = current.Name
			}
			ui.Button(c.Key("use"), locales.KubeUse()).Disabled(!plans).OnClick(func() {
				a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
					return ops.KubeContextPlan(in.Path, from, k.Name, env, run.Exec{})
				}, a.proposeOrSay)
			})
		}
		if plans {
			iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) {
				if menu.Item(locales.KubeNamespaceChange()).Chosen() {
					a.kubeNS = kubeNamespaceDialog{open: true, context: k.Name, from: k.Namespace, file: k.File, value: k.Namespace}
				}
			})
		}
	})
}

// kubeTools are kubectl's and Helm's rows, as CLI & clouds had them.
func (a *App) kubeTools(c *ui.Context, cli inventory.CliState) {
	pal := Colors(c.Theme())
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		shown := 0
		for _, cmd := range kubeCommands {
			if a.kubeMatch(cmd) {
				a.cliRow(c.Key("cli:"+cmd), cli, cmd)
				shown++
			}
		}
		if shown == 0 {
			cardNote(c, locales.CleanupNone())
		}
	})
}

// kubeFiles are the kubeconfig files kubectl reads, in merge order.
func (a *App) kubeFiles(c *ui.Context, cli inventory.CliState) {
	pal := Colors(c.Theme())
	cc := cliContext(cli, "kubectl")
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		shown := 0
		for i, f := range cli.KubeFiles {
			if !a.kubeMatch(f.Path) {
				continue
			}
			shown++
			var parts []string
			switch {
			case f.Missing:
				parts = append(parts, locales.KubeFileMissing())
			case f.Invalid:
				parts = append(parts, locales.KubeFileInvalid())
			default:
				n := 0
				for _, k := range cli.KubeContexts {
					if k.File == f.Path {
						n++
					}
				}
				parts = append(parts, locales.KubeFileContexts(n))
				if cc != nil && cc.File == f.Path {
					parts = append(parts, locales.KubeFileCurrent(cc.Name))
				}
			}
			cliTableRow(c.Key("kube-file:"+f.Path), tilde(f.Path, a.Home), strings.Join(parts, " · "), strconv.Itoa(i+1), func() {
				if a.Shell != nil && !f.Missing {
					iconButton(c, "external-link", locales.OpenInEditor()).OnClick(func() { a.openFile(c, f.Path) })
				}
			})
		}
		if shown == 0 {
			cardNote(c, locales.CleanupNone())
		}
	})
}

// kubeTerminalCard says what kubectl in a terminal uses: the context and
// namespace, the files, the tools.
func (a *App) kubeTerminalCard(c *ui.Context, cli inventory.CliState, current *inventory.KubeContext) {
	listCard(c, locales.KubeTerminal(), "", func() {
		if current != nil {
			// The file that sets current-context, which need not define it.
			cc := cliContext(cli, "kubectl")
			listRow(c, current.Name+" · "+namespaceOf(*current), false, locales.KubeCurrentFrom(tilde(cc.File, a.Home)), func() {})
		} else {
			listRow(c, locales.KubeNoCurrent(), false, "", func() {})
		}
		var files []string
		for _, f := range cli.KubeFiles {
			files = append(files, tilde(f.Path, a.Home))
		}
		switch {
		case len(files) == 0:
		case cli.KubeconfigSet:
			listRow(c, locales.KubeconfigSet(len(files)), false, strings.Join(files, ", "), func() {})
		default:
			listRow(c, locales.KubeconfigUnset(), false, locales.KubeconfigOnly(strings.Join(files, ", ")), func() {})
		}
		var versions, sources []string
		for _, cmd := range kubeCommands {
			if in, ok := cliCopy(cli, cmd); ok {
				versions = append(versions, cmd+" "+in.Version)
				sources = append(sources, cmd+": "+sourceName(in.Source))
			}
		}
		if len(versions) > 0 {
			listRow(c, strings.Join(versions, " · "), false, strings.Join(sources, " · "), func() {})
		}
	})
}

// kubeNamespaceDialog asks for the namespace of a context, then proposes
// the plan that sets it.
func (a *App) kubeNamespaceDialog(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	d := &a.kubeNS
	dialogBase(c.Key("kube-namespace"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(460).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		in, found := cliCopy(a.cliState(), "kubectl")
		value := strings.TrimSpace(d.value)
		ok := found && a.BuildPlan != nil && ops.KubeNamespaceValid(value) && value != d.from
		next := func() {
			if !ok {
				return
			}
			name, from, file := d.context, d.from, d.file
			d.open = false
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.KubeNamespacePlan(in.Path, name, from, value, tilde(file, a.Home), env, run.Exec{})
			}, a.proposeOrSay)
		}
		ui.Column(c).Children(func() {
			ui.Column(c).Gap(4).Padding(20, 20, 16, 20).Children(func() {
				ui.Text(c, locales.KubeNamespaceDialogTitle(d.context)).FontSize(18).FontWeight(600)
				ui.Text(c, locales.KubeNamespaceDialogLead(tilde(d.file, a.Home))).FontSize(12).TextColor(t.TextMuted)
				field := ui.TextInput(c.Key("kube-namespace-name"), &d.value).Placeholder("default").Label(locales.KubeNamespaceLabel()).Margin(10, 0, 0, 0).AutoFocus()
				if field.Submitted() {
					next()
				}
				if value != "" && !ops.KubeNamespaceValid(value) {
					ui.Text(c, locales.KubeNamespaceInvalid()).FontSize(12).TextColor(pal.WarningText)
				}
			})
			ui.Row(c).Gap(8).Padding(12, 20).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Box(c).Grow(1).Height(1)
				ui.Button(c, locales.Cancel()).OnClick(func() { d.open = false })
				ui.PrimaryButton(c, locales.Next()).Disabled(!ok).OnClick(next)
			})
		})
	})
}
