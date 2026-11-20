package ui

import (
	"cmp"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// kubeReady is where a project's Kubernetes link stands, from what the
// scan found: Hearth does not ask the cluster for it.
type kubeReady struct {
	// ctx is the linked context, nil when kubeconfig has it no more.
	ctx *inventory.KubeContext
	// cluster is the kind cluster of a kind context, "" for another one;
	// kind is that cluster in the engine, nil when the engine has none
	// such or is down.
	cluster  string
	kind     *inventory.KindCluster
	engineUp bool
	// current is kubectl's current context, "" when none is chosen.
	current string
}

// kubeReadiness finds where link stands.
func (a *App) kubeReadiness(link store.KubeLink) kubeReady {
	cli := a.cliState()
	var r kubeReady
	for i, k := range cli.KubeContexts {
		if k.Name == link.Context && !k.Shadowed {
			r.ctx = &cli.KubeContexts[i]
		}
	}
	if cur := kubeCurrent(cli); cur != nil {
		r.current = cur.Name
	}
	if name, ok := strings.CutPrefix(link.Context, "kind-"); ok && a.Snapshot != nil {
		r.cluster = name
		_, d, up := a.kindEngine()
		r.engineUp = up
		if up {
			if i := slices.IndexFunc(d.KindClusters, func(k inventory.KindCluster) bool { return k.Name == name }); i >= 0 {
				r.kind = &d.KindClusters[i]
			}
		}
	}
	return r
}

// kubeContextNames are the contexts kubectl can use, the ones a project
// links to.
func kubeContextNames(cli inventory.CliState) []string {
	var names []string
	for _, k := range cli.KubeContexts {
		if !k.Shadowed {
			names = append(names, k.Name)
		}
	}
	return names
}

// kubeLinkDialog links a project to a context and namespace.
type kubeLinkDialog struct {
	open               bool
	dir                string
	context, namespace string
	// linked says the project has a link, which the dialog may remove.
	linked bool
}

// openKubeLink opens the link dialog of the project in dir, on its link,
// or else on kubectl's current context.
func (a *App) openKubeLink(dir string) {
	cli := a.cliState()
	names := kubeContextNames(cli)
	link, linked := a.KubeLinks[dir]
	d := kubeLinkDialog{open: true, dir: dir, linked: linked, namespace: link.Namespace}
	switch {
	case slices.Contains(names, link.Context):
		d.context = link.Context
	case kubeCurrent(cli) != nil && slices.Contains(names, kubeCurrent(cli).Name):
		d.context = kubeCurrent(cli).Name
	case len(names) > 0:
		d.context = names[0]
	}
	if d.context != link.Context {
		d.namespace = ""
	}
	a.kubeLink = d
}

// kubeLinkView is the link dialog: the context, its namespace and what
// linking changes, which is Hearth's settings alone.
func (a *App) kubeLinkView(c *ui.Context) {
	t := c.Theme()
	pal := Colors(t)
	d := &a.kubeLink
	dialogBase(c.Key("kube-link"), &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.25))
		panel.Width(600).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).
			Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 0.18))
		cli := a.cliState()
		names := kubeContextNames(cli)
		var ctx inventory.KubeContext
		if i := slices.IndexFunc(cli.KubeContexts, func(k inventory.KubeContext) bool { return k.Name == d.context && !k.Shadowed }); i >= 0 {
			ctx = cli.KubeContexts[i]
		}
		value := strings.TrimSpace(d.namespace)
		ns := value
		if ns == "" {
			ns = namespaceOf(ctx)
		}
		ok := a.LinkKube != nil && ctx.Name != "" && ops.KubeNamespaceValid(ns)
		save := func() {
			if !ok {
				return
			}
			d.open = false
			a.LinkKube(d.dir, &store.KubeLink{Context: ctx.Name, Namespace: ns})
		}
		row := func(label string, body func()) {
			ui.Row(c).Gap(12).MinHeight(32).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(13).Width(150).Shrink(0)
				body()
			})
		}
		ui.Column(c).Children(func() {
			ui.Column(c).Gap(4).Padding(20, 20, 12, 20).Children(func() {
				ui.Text(c, locales.KubeLinkTitle(filepath.Base(d.dir))).FontSize(18).FontWeight(600)
				ui.Text(c, locales.KubeLinkLead()).FontSize(12).TextColor(t.TextMuted)
			})
			ui.Column(c).Gap(4).Padding(4, 20, 14, 20).Children(func() {
				if len(names) == 0 {
					ui.Text(c, locales.KubeNoContexts()).FontSize(12).TextColor(pal.WarningText)
					return
				}
				row(locales.KubeContextLabel(), func() {
					ui.Select(c.Key("kube-link-context"), &d.context, names).Label(locales.KubeContextLabel()).Width(220)
					clipped(c, locales.KubeLinkContextHint(ctx.Cluster, tilde(ctx.File, a.Home))).FontSize(11.5).TextColor(pal.TextFaint).Grow(1).MinWidth(0)
				})
				row(locales.KubeNamespaceLabel(), func() {
					field := ui.TextInput(c.Key("kube-link-namespace"), &d.namespace).Placeholder(namespaceOf(ctx)).Label(locales.KubeNamespaceLabel()).Width(220)
					if field.Submitted() {
						save()
					}
					hint, color := locales.KubeNamespaceMayMiss(), pal.TextFaint
					if !ops.KubeNamespaceValid(ns) {
						hint, color = locales.KubeNamespaceInvalid(), pal.WarningText
					}
					ui.Text(c, hint).FontSize(11.5).TextColor(color).Grow(1).MinWidth(0)
				})
			})
			if ctx.Name != "" {
				lines := []string{locales.KubeLinkSaved()}
				if cur := kubeCurrent(cli); cur != nil && cur.Name != ctx.Name {
					lines = append(lines, locales.KubeLinkKubectlKept(cur.Name, ctx.Name))
				}
				lines = append(lines, locales.KubeLinkManifests())
				ui.Column(c).Gap(6).Padding(12, 20, 14, 20).Background(pal.Subtle).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
					ui.Text(c, strings.ToUpper(locales.KindWhatHappens())).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint)
					for i, line := range lines {
						ui.Row(c).Gap(12).Children(func() {
							ui.Text(c, strconv.Itoa(i+1)).FontSize(12).TextColor(pal.TextFaint).Width(10).Shrink(0)
							ui.Text(c, line).FontSize(12.5).Grow(1).MinWidth(0)
						})
					}
				})
			}
			ui.Row(c).Gap(8).Padding(12, 20).AlignItems(ui.Center).BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
				if d.linked && a.LinkKube != nil {
					ui.Button(c.Key("kube-unlink"), locales.KubeUnlink()).OnClick(func() {
						d.open = false
						a.LinkKube(d.dir, nil)
					})
				}
				ui.Box(c).Grow(1).Height(1)
				ui.Button(c, locales.Cancel()).OnClick(func() { d.open = false })
				ui.PrimaryButton(c.Key("kube-link-save"), locales.KubeLinkSave()).Disabled(!ok).OnClick(save)
			})
		})
	})
}

// projectKubeCard is the project's Kubernetes: the linked context and
// its access, the kind cluster behind it, kubectl's own context, and the
// manifests. Shown when the project has
// manifests or a link.
func (a *App) projectKubeCard(c *ui.Context, p projects.Project) {
	link, linked := a.KubeLinks[p.Dir]
	if !linked && len(p.Kube) == 0 {
		return
	}
	t := c.Theme()
	pal := Colors(t)
	var r kubeReady
	if linked {
		r = a.kubeReadiness(link)
	}
	cli := a.cliState()
	kubectl, hasKubectl := cliCopy(cli, "kubectl")
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(10, 14, 8, 14).MinHeight(44).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Kubernetes").FontSize(13.5).FontWeight(600).Grow(1).SingleLine()
			ui.Text(c, locales.KubeCardAside()).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			switch {
			case !linked || a.LinkKube == nil:
			case r.ctx == nil:
				ui.Button(c.Key("kube-unlink"), locales.KubeUnlink()).OnClick(func() { a.LinkKube(p.Dir, nil) })
			default:
				ui.Button(c.Key("kube-link"), locales.KubeLinkChange()).OnClick(func() { a.openKubeLink(p.Dir) })
			}
		})
		if !linked {
			listRow(c, locales.KubeNotLinked(), false, locales.KubeNotLinkedCaption(), func() {
				if a.LinkKube != nil && len(kubeContextNames(cli)) > 0 {
					ui.Button(c.Key("kube-link"), locales.KubeLinkButton()).Shrink(0).OnClick(func() { a.openKubeLink(p.Dir) })
				}
			})
		} else {
			title := locales.KubeLinkName(link.Context, link.Namespace)
			if r.ctx == nil {
				listRow(c, title, false, locales.KubeContextGone(), func() {
					aiTag(c, locales.KubeContextMissing(), pal.DangerSoft, pal.DangerText)
					if a.LinkKube != nil && len(kubeContextNames(cli)) > 0 {
						ui.Button(c.Key("kube-other"), locales.KubeChooseOther()).Shrink(0).OnClick(func() { a.openKubeLink(p.Dir) })
					}
				})
			} else {
				k := *r.ctx
				listRow(c, title, false, locales.KubeLinkCaption(tilde(k.File, a.Home)), func() {
					if check, ok := a.kubeChecks[k.Name]; ok {
						a.kubeAccessTag(c, k, check)
					}
					if hasKubectl && a.CheckKube != nil {
						ui.Button(c.Key("kube-check"), locales.KubeCheck()).Shrink(0).Disabled(a.kubeChecks[k.Name].running).OnClick(func() { a.checkKube(kubectl.Path, k) })
					}
					if hasKubectl && a.WatchCluster != nil {
						ui.Button(c.Key("kube-open"), locales.KubeClusterOpen()).Shrink(0).OnClick(func() {
							a.setNamespaces(k.Name, []string{cmp.Or(link.Namespace, namespaceOf(k))})
							a.cluster.kind = kindServices
							a.openCluster(k.Name)
						})
					}
				})
			}
			a.projectKindRow(c, p.Dir, r)
			if r.ctx != nil && r.current != link.Context {
				title := locales.KubeNoCurrentTitle()
				if r.current != "" {
					title = locales.KubeOtherCurrent(r.current)
				}
				listRow(c, title, false, locales.KubeOtherCurrentCaption(), func() {
					if r.current != "" {
						aiTag(c, locales.KubeOtherTag(), pal.WarningSoft, pal.WarningText)
					}
					if hasKubectl && a.BuildPlan != nil {
						from, to := r.current, link.Context
						ui.Button(c.Key("kube-use"), locales.KubeUseFor(to)).Shrink(0).OnClick(func() {
							a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
								return ops.KubeContextPlan(kubectl.Path, from, to, env, run.Exec{})
							}, a.proposeOrSay)
						})
					}
				})
			}
		}
		canApply := hasKubectl && r.ctx != nil && a.BuildPlan != nil && a.PickManifest != nil
		if len(p.Kube) > 0 || canApply {
			label, caption := locales.KubeApplySource(), ""
			if len(p.Kube) > 0 {
				label, caption = locales.KubeManifests(strings.Join(p.Kube, " · ")), locales.KubeManifestsCaption()
			}
			listRow(c, label, false, caption, func() {
				if a.Shell != nil && len(p.Kube) > 0 {
					first := filepath.Join(p.Dir, strings.TrimSuffix(p.Kube[0], "/"))
					ui.Button(c.Key("kube-manifests"), locales.PrepareOpenFolder()).Shrink(0).OnClick(func() { a.Shell.ShowItemInFolder(first) })
				}
				if canApply {
					ui.Button(c.Key("kube-apply"), locales.KubeApplyOpen()).Shrink(0).OnClick(func() {
						a.openProjectApply(p, kubectl.Path, link.Context, cmp.Or(link.Namespace, namespaceOf(*r.ctx)))
					})
				}
			})
		}
	})
}

// projectKindRow is the kind cluster behind a project's kind context: how
// its nodes run, or that the engine has it no more, to create again.
func (a *App) projectKindRow(c *ui.Context, dir string, r kubeReady) {
	if r.cluster == "" {
		return
	}
	pal := Colors(c.Theme())
	switch {
	case r.kind != nil:
		engine, _, _ := a.kindEngine()
		parts := []string{locales.KindNodes(r.kind.Nodes)}
		if r.kind.Version != "" {
			parts = append(parts, "Kubernetes "+r.kind.Version)
		}
		parts = append(parts, locales.KindInDocker(engineName(engine)))
		listRow(c, locales.KindClusterTitle(r.cluster), false, strings.Join(parts, " · "), func() {
			bg, fg := pal.SuccessSoft, pal.SuccessText
			if r.kind.Running != r.kind.Nodes {
				bg, fg = pal.WarningSoft, pal.WarningText
			}
			aiTag(c, kindState(*r.kind), bg, fg)
			ui.Button(c.Key("kind-open"), locales.SettingsOpen()).Shrink(0).OnClick(func() { a.router.Push(kubePage) })
		})
	case !r.engineUp:
		listRow(c, locales.KindEngineDown(), false, locales.KindEngineDownCaption(), func() {})
	default:
		listRow(c, locales.KubeKindGone(r.cluster), false, locales.KubeKindGoneCaption(), func() {
			if _, found := cliCopy(a.cliState(), "kind"); found && a.BuildPlan != nil {
				ui.Button(c.Key("kind-again"), locales.KubeKindCreate()).Shrink(0).OnClick(func() { a.openKindDialogFor(r.cluster, dir) })
			}
		})
	}
}

// kubeProjectTag says where a linked project stands, in a word: the
// context gone, its check, its kind cluster.
func (a *App) kubeProjectTag(c *ui.Context, r kubeReady) {
	pal := Colors(c.Theme())
	switch {
	case r.ctx == nil:
		aiTag(c, locales.KubeContextMissing(), pal.DangerSoft, pal.DangerText)
	case a.kubeChecks[r.ctx.Name] != (kubeCheck{}):
		a.kubeAccessTag(c, *r.ctx, a.kubeChecks[r.ctx.Name])
	case r.kind != nil && r.kind.Running == r.kind.Nodes:
		aiTag(c, kindState(*r.kind), pal.SuccessSoft, pal.SuccessText)
	case r.kind != nil:
		aiTag(c, kindState(*r.kind), pal.WarningSoft, pal.WarningText)
	case r.cluster != "" && r.engineUp:
		aiTag(c, locales.KubeKindMissingTag(), pal.WarningSoft, pal.WarningText)
	}
}

// kubeProjectsCount is how many projects the Projects tab lists: the
// linked ones and the ones with manifests.
func (a *App) kubeProjectsCount() int {
	n := 0
	for _, p := range a.Projects {
		if _, linked := a.KubeLinks[p.Dir]; linked || len(p.Kube) > 0 {
			n++
		}
	}
	return n
}

// kubeProjectTable lists the projects linked to a context, and the ones
// with manifests and no link.
func (a *App) kubeProjectTable(c *ui.Context) {
	pal := Colors(c.Theme())
	var linked, unlinked []projects.Project
	for _, p := range a.Projects {
		link, ok := a.KubeLinks[p.Dir]
		switch {
		case ok && a.kubeMatch(filepath.Base(p.Dir), p.Dir, link.Context, link.Namespace):
			linked = append(linked, p)
		case !ok && len(p.Kube) > 0 && a.kubeMatch(filepath.Base(p.Dir), p.Dir):
			unlinked = append(unlinked, p)
		}
	}
	row := func(p projects.Project, caption string, end func()) {
		ui.Row(c.Key("kube-project:"+p.Dir)).Gap(10).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				clipped(c, filepath.Base(p.Dir)).FontSize(13)
				clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint)
			})
			end()
		})
	}
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		if len(linked)+len(unlinked) == 0 {
			cardNote(c, locales.KubeNoProjects())
			return
		}
		if len(linked) > 0 {
			cliGroupHead(c, locales.KubeLinkedGroup(), len(linked))
		}
		for _, p := range linked {
			link := a.KubeLinks[p.Dir]
			r := a.kubeReadiness(link)
			row(p, locales.KubeLinkName(link.Context, link.Namespace)+" · "+tilde(p.Dir, a.Home), func() {
				a.kubeProjectTag(c, r)
				ui.Button(c.Key("open"), locales.SettingsOpen()).OnClick(func() { a.router.Push(projectPath(p.Dir)) })
			})
		}
		if len(unlinked) > 0 {
			cliGroupHead(c, locales.KubeUnlinkedGroup(), len(unlinked))
		}
		for _, p := range unlinked {
			row(p, strings.Join(p.Kube, " · ")+" · "+tilde(p.Dir, a.Home), func() {
				aiTag(c, locales.KubeNotLinkedTag(), pal.Muted, c.Theme().TextMuted)
				if a.LinkKube != nil && len(kubeContextNames(a.cliState())) > 0 {
					ui.Button(c.Key("link"), locales.KubeLinkButton()).OnClick(func() { a.openKubeLink(p.Dir) })
				}
			})
		}
	})
}

// kubeProjectsOf names the projects linked to the context.
func (a *App) kubeProjectsOf(context string) []string {
	var names []string
	for _, p := range a.Projects {
		if link, ok := a.KubeLinks[p.Dir]; ok && link.Context == context {
			names = append(names, filepath.Base(p.Dir))
		}
	}
	return names
}
