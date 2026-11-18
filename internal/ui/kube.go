package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// kubePage is Kubernetes: kubeconfig's contexts by file, kubectl and Helm.
const kubePage = "/tools/kubernetes"

// kubeCommands are the Kubernetes tools the page lists, which CLI & clouds
// leaves to it.
var kubeCommands = []string{"kubectl", "helm", "kind"}

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
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			if a.Snapshot != nil {
				ui.PrimaryButton(c.Key("kind-create"), locales.KindCreateButton()).OnClick(a.openKindDialog)
			}
		}})
		a.kubeNamespaceDialog(c)
		a.kindDialog(c)
		a.kubeLinkView(c)
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
						{locales.KubeProjectsTab(), strconv.Itoa(a.kubeProjectsCount())},
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
						a.kubeProjectTable(c)
					case 3:
						a.kubeFiles(c, cli)
					default:
						a.kubeContextTable(c, cli, current)
					}
				}, func() {
					if a.Snapshot != nil {
						a.kindClustersCard(c, cli, current)
					}
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

// kubeContextRow is a context: its cluster, namespace and sign-in program,
// whether kubectl uses it, what its last check found, and the check and
// the plans to use it or change its namespace.
func (a *App) kubeContextRow(c *ui.Context, cli inventory.CliState, k inventory.KubeContext, current *inventory.KubeContext) {
	pal := Colors(c.Theme())
	isCurrent := current != nil && !k.Shadowed && k.Name == current.Name
	parts := []string{locales.KubeCluster(k.Cluster), locales.KubeNamespace(namespaceOf(k))}
	if k.Auth != "" {
		// Shown before a check runs it.
		parts = append(parts, locales.KubeAuthVia(tilde(k.Auth, a.Home)))
	}
	for _, name := range a.kubeProjectsOf(k.Name) {
		parts = append(parts, locales.KubeProjectOf(name))
	}
	if k.Shadowed {
		parts = []string{locales.KubeShadowedCaption()}
	}
	in, found := cliCopy(cli, "kubectl")
	plans := found && a.BuildPlan != nil && !k.Shadowed
	// A shadowed context is never checked: its name is the other one's.
	check, checked := a.kubeChecks[k.Name]
	checked = checked && !k.Shadowed
	ui.Row(c).Gap(10).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, k.Name).FontSize(13)
			clipped(c, strings.Join(parts, " · ")).FontSize(10.5).TextColor(pal.TextFaint)
			if detail := kubeAccessDetail(k, check); checked && detail != "" {
				clipped(c, detail).Font(FontMono).FontSize(10.5).TextColor(c.Theme().TextMuted)
			}
		})
		if isCurrent {
			aiTag(c, locales.KubeInKubectl(), pal.SuccessSoft, pal.SuccessText)
		}
		if k.Shadowed {
			aiTag(c, locales.KubeShadowed(), pal.Muted, c.Theme().TextMuted)
			return
		}
		if checked {
			a.kubeAccessTag(c, k, check)
		}
		if found && a.CheckKube != nil {
			ui.Button(c.Key("check"), locales.KubeCheck()).Disabled(check.running).OnClick(func() { a.checkKube(in.Path, k) })
		}
		if plans {
			from := ""
			if current != nil {
				from = current.Name
			}
			iconButton(c, "ellipsis", locales.MoreActions()).Menu(func(menu *ui.Menu) {
				if !isCurrent && menu.Item(locales.KubeUse()).Chosen() {
					a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
						return ops.KubeContextPlan(in.Path, from, k.Name, env, run.Exec{})
					}, a.proposeOrSay)
				}
				if menu.Item(locales.KubeNamespaceChange()).Chosen() {
					a.kubeNS = kubeNamespaceDialog{open: true, context: k.Name, from: k.Namespace, file: k.File, value: k.Namespace}
				}
			})
		}
	})
}

// kubeCheck is the last check of a context's cluster this session.
type kubeCheck struct {
	running bool
	res     detect.KubeAccess
}

// checkKube asks the context's cluster through kubectl, in the
// background; a check started again replaces the last one's result.
func (a *App) checkKube(kubectl string, k inventory.KubeContext) {
	if a.kubeChecks == nil {
		a.kubeChecks = map[string]kubeCheck{}
	}
	a.kubeChecks[k.Name] = kubeCheck{running: true}
	a.CheckKube(kubectl, k, func(res detect.KubeAccess) {
		a.kubeChecks[k.Name] = kubeCheck{res: res}
	})
}

// kubeAccessDetail is the line that says why a check failed: kubectl's or
// the sign-in program's own, or that the program is not on the PATH.
func kubeAccessDetail(k inventory.KubeContext, ch kubeCheck) string {
	switch {
	case ch.running || ch.res.State == detect.KubeReachable:
		return ""
	case ch.res.State == detect.KubeNoAuthProgram && ch.res.Detail == k.Auth:
		return locales.KubeAccessNoProgramDetail(k.Auth)
	}
	return ch.res.Detail
}

// kubeAccessTag says what a check found, as the design's chips do.
func (a *App) kubeAccessTag(c *ui.Context, k inventory.KubeContext, ch kubeCheck) {
	pal := Colors(c.Theme())
	if ch.running {
		aiTag(c, locales.KubeChecking(), pal.InfoSoft, pal.InfoText)
		return
	}
	switch ch.res.State {
	case detect.KubeReachable:
		aiTag(c, locales.KubeAccessOk(), pal.SuccessSoft, pal.SuccessText).Tooltip(locales.KubeAccessOkTip(ch.res.Version, ch.res.Took.Round(time.Millisecond).String()))
	case detect.KubeUnreachable:
		aiTag(c, locales.KubeAccessUnreachable(), pal.DangerSoft, pal.DangerText)
	case detect.KubeNoAuthProgram:
		aiTag(c, locales.KubeAccessNoProgram(k.Auth), pal.WarningSoft, pal.WarningText)
	case detect.KubeNeedsLogin:
		aiTag(c, locales.KubeAccessLogin(), pal.WarningSoft, pal.WarningText)
	case detect.KubeAuthFailed:
		aiTag(c, locales.KubeAccessAuthFailed(), pal.WarningSoft, pal.WarningText)
	case detect.KubeForbidden:
		aiTag(c, locales.KubeAccessForbidden(), pal.DangerSoft, pal.DangerText)
	default:
		aiTag(c, locales.KubeAccessUnknown(), pal.Muted, c.Theme().TextMuted)
	}
}

// kubeTools are kubectl's, Helm's and kind's rows, as CLI & clouds had
// them; one not found installs from the catalog.
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

// kindEngine is the selected local Docker context, the engine kind runs
// its nodes in, the Docker state the scan found, and whether the engine
// answered the scan.
func (a *App) kindEngine() (inventory.DockerContext, *inventory.DockerState, bool) {
	st, ok := a.Snapshot.Tool("docker")
	if !ok || st.Docker == nil {
		return inventory.DockerContext{}, nil, false
	}
	for _, ctx := range st.Docker.Contexts {
		if ctx.Name == st.Docker.Selected && ctx.Local {
			return ctx, st.Docker, ctx.Status == "available"
		}
	}
	return inventory.DockerContext{}, st.Docker, false
}

// engineName names a Docker context's engine, its context's name when
// the engine is not known.
func engineName(ctx inventory.DockerContext) string {
	if ctx.Engine != "" {
		return ctx.Engine
	}
	return ctx.Name
}

// kindState says how many of a cluster's nodes run.
func kindState(k inventory.KindCluster) string {
	switch k.Running {
	case k.Nodes:
		return locales.KindRunning()
	case 0:
		return locales.KindStopped()
	}
	return locales.KindPartly(strconv.Itoa(k.Running), strconv.Itoa(k.Nodes))
}

// kindClustersCard lists the kind clusters in the Docker engine, each
// deleted through a plan.
func (a *App) kindClustersCard(c *ui.Context, cli inventory.CliState, current *inventory.KubeContext) {
	engine, d, up := a.kindEngine()
	in, found := cliCopy(cli, "kind")
	aside := ""
	if up {
		aside = locales.KindInDocker(engineName(engine))
	}
	listCard(c, locales.KindLocalClusters(), aside, func() {
		switch {
		case !up:
			listRow(c, locales.KindEngineDown(), false, locales.KindEngineDownCaption(), func() {})
			return
		case len(d.KindClusters) == 0:
			cardNote(c, locales.KindNoClusters())
			return
		}
		cur := ""
		if current != nil {
			cur = current.Name
		}
		for _, k := range d.KindClusters {
			parts := []string{"kind"}
			if k.Version != "" {
				parts = append(parts, k.Version)
			}
			parts = append(parts, locales.KindNodes(k.Nodes), kindState(k))
			kc := c.Key("kind:" + k.Name)
			listRow(kc, k.Name, false, strings.Join(parts, " · "), func() {
				if !found || a.BuildPlan == nil {
					return
				}
				ui.Button(kc.Key("delete"), locales.KindDelete()).Shrink(0).OnClick(func() {
					a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
						return ops.KindDeletePlan(in.Path, k.Name, cur, a.kubeProjectsOf("kind-"+k.Name), env, run.Exec{})
					}, a.proposeOrSay)
				})
			})
		}
	})
}

// kindCreateDialog is the dialog that creates a local kind cluster.
type kindCreateDialog struct {
	open bool
	name string
	// nodes is the nodes segment: 0 one node, 1 one and two workers.
	nodes int
	// project is the label of the project to link the cluster to once it
	// is created, the first of kindProjects' labels for none.
	project string
}

// kindProjects are the labels the kind dialog offers to link a cluster
// to, not linking first, and the projects' folders by label.
func (a *App) kindProjects() (labels, dirs []string) {
	labels, dirs = []string{locales.KindNoProject()}, []string{""}
	for _, p := range a.Projects {
		labels, dirs = append(labels, filepath.Base(p.Dir)), append(dirs, p.Dir)
	}
	return labels, dirs
}

// openKindDialog opens the dialog with the first free name of dev,
// dev-2, dev-3…
func (a *App) openKindDialog() {
	var taken []inventory.KindCluster
	if _, d, _ := a.kindEngine(); d != nil {
		taken = d.KindClusters
	}
	name := "dev"
	for i := 2; slices.ContainsFunc(taken, func(k inventory.KindCluster) bool { return k.Name == name }); i++ {
		name = "dev-" + strconv.Itoa(i)
	}
	a.kind = kindCreateDialog{open: true, name: name, project: locales.KindNoProject()}
}

// openKindDialogFor opens the dialog to create again the cluster name,
// which the project in dir links to.
func (a *App) openKindDialogFor(name, dir string) {
	a.kind = kindCreateDialog{open: true, name: name, project: filepath.Base(dir)}
}

// kindDialog asks for the new cluster's name and nodes, says what will
// happen, then proposes the plan that creates it.
func (a *App) kindDialog(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	d := &a.kind
	dialogBase(c.Key("kind-create"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(600).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		cli := a.cliState()
		kind, found := cliCopy(cli, "kind")
		kubectl, _ := cliCopy(cli, "kubectl")
		engine, docker, up := a.kindEngine()
		from := ""
		if current := kubeCurrent(cli); current != nil && kubectl.Path != "" {
			from = current.Name
		}
		name := strings.TrimSpace(d.name)
		valid := ops.KindNameValid(name)
		taken := docker != nil && slices.ContainsFunc(docker.KindClusters, func(k inventory.KindCluster) bool { return k.Name == name })
		workers := 2 * d.nodes
		ok := found && up && a.BuildPlan != nil && valid && !taken
		labels, dirs := a.kindProjects()
		project := ""
		if i := slices.Index(labels, d.project); i >= 0 {
			project = dirs[i]
		}
		next := func() {
			if !ok {
				return
			}
			d.open = false
			done := a.proposeOrSay
			if project != "" && a.LinkKube != nil {
				done = a.proposeThen(func() {
					a.LinkKube(project, &store.KubeLink{Context: "kind-" + name, Namespace: "default"})
				})
			}
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.KindCreatePlan(kind.Path, kubectl.Path, name, workers, from, env, run.Exec{})
			}, done)
		}
		row := func(label string, body func()) {
			ui.Row(c).Gap(12).MinHeight(32).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(13).Width(150).Shrink(0)
				body()
			})
		}
		ui.Column(c).Children(func() {
			ui.Column(c).Gap(4).Padding(20, 20, 12, 20).Children(func() {
				ui.Text(c, locales.KindDialogTitle()).FontSize(18).FontWeight(600)
				ui.Text(c, locales.KindDialogLead()).FontSize(12).TextColor(t.TextMuted)
			})
			ui.Column(c).Gap(4).Padding(4, 20, 14, 20).Children(func() {
				row(locales.KindTool(), func() {
					if found {
						ui.Text(c, "kind "+kind.Version+" · "+sourceName(kind.Source)).FontSize(12.5)
					} else {
						ui.Text(c, locales.KindNotInstalled()).FontSize(12).TextColor(pal.WarningText)
					}
				})
				row(locales.KindNameLabel(), func() {
					field := ui.TextInput(c.Key("kind-name"), &d.name).Placeholder("dev").Label(locales.KindNameLabel()).Width(200).AutoFocus()
					if field.Submitted() {
						next()
					}
					hint, color := locales.KindContextHint("kind-"+name), pal.TextFaint
					switch {
					case name != "" && !valid:
						hint, color = locales.KindNameInvalid(), pal.WarningText
					case taken:
						hint, color = locales.KindExists(name), pal.WarningText
					}
					ui.Text(c, hint).FontSize(11.5).TextColor(color).Grow(1).MinWidth(0)
				})
				row(locales.KindNodesLabel(), func() {
					segments(c, "kind-nodes", &d.nodes, []segment{{locales.KindOneNode(), ""}, {locales.KindWithWorkers(), ""}})
				})
				if found {
					row("Kubernetes", func() {
						ui.Text(c, locales.KindVersionDefault(kind.Version)).FontSize(12.5).TextColor(t.TextMuted)
					})
				}
				row(locales.KindEngine(), func() {
					if up {
						aiTag(c, locales.KindEngineReady(engineName(engine)), pal.SuccessSoft, pal.SuccessText)
					} else {
						aiTag(c, locales.KindEngineDown(), pal.WarningSoft, pal.WarningText)
					}
				})
				if len(labels) > 1 && a.LinkKube != nil {
					row(locales.KindProject(), func() {
						ui.Select(c.Key("kind-project"), &d.project, labels).Label(locales.KindProject()).Width(200)
						ui.Text(c, locales.KindProjectHint()).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
					})
				}
			})
			if found && valid {
				argv := []string{"kind", "create", "cluster", "--name", name, "--wait", "3m"}
				if workers > 0 {
					argv = append(argv, "--config", "…")
				}
				lines := []string{ops.Command(argv), locales.KindContextAdded("kind-" + name)}
				if from != "" {
					lines = append(lines, locales.KindContextKept(from))
				} else {
					lines = append(lines, locales.KindContextNow("kind-"+name))
				}
				if project != "" && a.LinkKube != nil {
					lines = append(lines, locales.KindLinkLine(filepath.Base(project), "kind-"+name))
				}
				lines = append(lines, locales.KindImageNoteText())
				ui.Column(c).Gap(6).Padding(12, 20, 14, 20).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
					ui.Text(c, strings.ToUpper(locales.KindWhatHappens())).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint)
					for i, line := range lines {
						ui.Row(c).Gap(12).Children(func() {
							ui.Text(c, strconv.Itoa(i+1)).FontSize(12).TextColor(pal.TextFaint).Width(10).Shrink(0)
							text := ui.Text(c, line).FontSize(12.5).Grow(1).MinWidth(0)
							if i == 0 {
								text.Font(FontMono).FontSize(11.5).SingleLine()
							}
						})
					}
				})
			}
			ui.Row(c).Gap(8).Padding(12, 20).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				ui.Box(c).Grow(1).Height(1)
				ui.Button(c, locales.Cancel()).OnClick(func() { d.open = false })
				ui.PrimaryButton(c, locales.Next()).Disabled(!ok).OnClick(next)
			})
		})
	})
}
