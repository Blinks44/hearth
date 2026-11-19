package ui

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// kubeClusterPage is a context's cluster: its pods and Deployments, read
// while the page shows.
const kubeClusterPage = kubePage + "/cluster"

// kubeClusterPath is the cluster page of the context name.
func kubeClusterPath(name string) string {
	return kubeClusterPage + "?context=" + url.QueryEscape(name)
}

// The kinds of resources the cluster page lists, in its chips' order.
const (
	kindPods = iota
	kindDeployments
	kindOverview
	kindJobs
	kindCronJobs
	kindServices
)

// kubeClusterState is the cluster page's: what it reads and what it read.
type kubeClusterState struct {
	// namespaces are those chosen by context; a context without an entry
	// reads its own namespace, an empty entry all of them.
	namespaces map[string][]string
	kind       int
	// target is what the data was read from; running is what the watch
	// reads, with refresh, and stop ends it.
	target, running string
	refresh         int
	watch           int
	stop            func()
	data            *kube.Cluster
	batchData       bool
	err             error
	// failedAt is when the last read failed, after data was read.
	failedAt time.Time

	contextOpen, namespaceOpen bool
	kindsOpen                  bool
	kindQuery                  string
	kindSelected               int
	jobFilter, cronJobFilter   int
	serviceFilter              int
	serviceSort                string
	forward                    *forwardForm
	// nsNames are the context's namespaces, listed as the namespace
	// popover opened; nsErr says why they could not be.
	nsLoading bool
	nsNames   []string
	nsErr     error
	nsTyped   string

	// podFilter and deploymentFilter are the lists' tabs; query filters
	// both.
	podFilter, deploymentFilter int
	query                       string
	// queryField is the search's own: the Kubernetes page's filter,
	// bound to App.filterField, is built beside it while the page slides
	// in.
	queryField ui.Handle
	// toggled are the owner groups shown otherwise than by default: open
	// for those in trouble, closed for the rest.
	toggled map[string]bool
	// pod and deployment are the UIDs of those whose details show beside
	// the list, tab the details' tab.
	pod, deployment string
	revealWorkload  string
	tab             int
	// logs is the log shown in place of the list, nil for none;
	// logRunning is what its kubectl logs reads, and logStop ends it.
	logs       *logView
	logRunning string
	logStop    func()
}

// clusterNamespaces are the namespaces the page reads in the context:
// none means all of them.
func (a *App) clusterNamespaces(name string) []string {
	if ns, ok := a.cluster.namespaces[name]; ok {
		return ns
	}
	if k := a.kubeContext(name); k != nil {
		return []string{namespaceOf(*k)}
	}
	return []string{"default"}
}

// kubeContext is the context of that name kubectl uses, nil when no file
// defines it.
func (a *App) kubeContext(name string) *inventory.KubeContext {
	cli := a.cliState()
	for i, k := range cli.KubeContexts {
		if k.Name == name && !k.Shadowed {
			return &cli.KubeContexts[i]
		}
	}
	return nil
}

// syncCluster keeps the watch of the cluster page's target running while
// the page shows, and stops it once it does not: the router tells no page
// that it left. It starts and stops only as the target changes, so a view
// built again does nothing. The events of the object whose details show
// are read with the rest; choosing another keeps what was read.
func (a *App) syncCluster() {
	s := &a.cluster
	var t kube.Target
	target := ""
	if a.router.Path() == kubeClusterPage && a.WatchCluster != nil {
		name := a.router.Query("context")
		if in, ok := cliCopy(a.cliState(), "kubectl"); ok && name != "" {
			t = kube.Target{Kubectl: in.Path, Context: name, Namespaces: a.clusterNamespaces(name)}
			t.Overview = a.cluster.kind == kindOverview
			t.Batch = t.Overview || isBatchKind(a.cluster.kind)
			t.Services = s.kind == kindServices
			target = fmt.Sprintf("%q", []string{t.Kubectl, t.Context, strings.Join(t.Namespaces, ",")})
		}
	}
	running := ""
	if target != "" {
		t.Events, t.Tail = a.detailRef(), a.detailTail()
		running = fmt.Sprintf("%s#%d#%t#%t#%t#%s#%+v", target, s.refresh, t.Overview, t.Batch, t.Services, t.Events.UID, t.Tail)
	}
	if running == s.running {
		return
	}
	if s.stop != nil {
		s.stop()
		s.stop = nil
	}
	s.running = running
	s.watch++
	if target != s.target {
		// Another cluster or namespace: what was read is not its.
		s.target, s.data, s.err, s.failedAt = target, nil, nil, time.Time{}
		s.logs, s.forward, s.revealWorkload = nil, nil, ""
	}
	if running == "" {
		return
	}
	watch := s.watch
	s.stop = a.WatchCluster(t, func(cl kube.Cluster, err error) {
		if s.watch != watch {
			return
		}
		if err != nil {
			s.err, s.failedAt = err, time.Now()
			return
		}
		s.data, s.err, s.failedAt = &cl, nil, time.Time{}
		s.batchData = t.Batch && !t.Overview
	})
}

// detailRef is the object whose details show, zero for none.
func (a *App) detailRef() kube.Ref {
	s := &a.cluster
	if s.data == nil {
		return kube.Ref{}
	}
	if s.kind == kindDeployments {
		for _, d := range s.data.Deployments {
			if d.UID == s.deployment {
				return kube.Ref{Namespace: d.Namespace, UID: d.UID}
			}
		}
		return kube.Ref{}
	}
	if s.kind == kindPods {
		for _, p := range s.data.Pods {
			if p.UID == s.pod {
				return kube.Ref{Namespace: p.Namespace, UID: p.UID}
			}
		}
	}
	return kube.Ref{}
}

// detailTail is the container of the pod whose details show that keeps
// failing, whose failed run's last line they tell; zero for none.
func (a *App) detailTail() kube.Tail {
	s := &a.cluster
	if s.data == nil || s.kind != kindPods || s.logs != nil {
		return kube.Tail{}
	}
	for _, p := range s.data.Pods {
		if p.UID != s.pod {
			continue
		}
		if story, ok := storyOf(p, nil, time.Now()); ok && story.crashed != "" {
			return kube.Tail{Namespace: p.Namespace, Pod: p.Name, Container: story.crashed, Previous: !story.ended}
		}
	}
	return kube.Tail{}
}

// StopCluster ends the cluster page's reads, as the window closes.
func (a *App) StopCluster() {
	if a.cluster.stop != nil {
		a.cluster.stop()
		a.cluster.stop, a.cluster.running = nil, ""
		a.cluster.watch++
	}
	a.stopLogs()
}

// openCluster shows the cluster of the context name.
func (a *App) openCluster(name string) {
	a.cluster.contextOpen, a.cluster.kindsOpen = false, false
	a.router.Push(kubeClusterPath(name))
}

// setNamespaces chooses the namespaces the page reads in the context.
func (a *App) setNamespaces(name string, ns []string) {
	if a.cluster.namespaces == nil {
		a.cluster.namespaces = map[string][]string{}
	}
	a.cluster.namespaces[name] = ns
}

// listNamespaces asks the context's cluster for its namespaces, for the
// namespace popover.
func (a *App) listNamespaces(name string) {
	s := &a.cluster
	in, ok := cliCopy(a.cliState(), "kubectl")
	if !ok || a.ListNamespaces == nil {
		return
	}
	s.nsLoading, s.nsErr, s.nsNames = true, nil, nil
	a.ListNamespaces(kube.Target{Kubectl: in.Path, Context: name}, func(names []string, err error) {
		if a.router.Query("context") != name {
			return
		}
		s.nsLoading, s.nsNames, s.nsErr = false, names, err
	})
}

func (a *App) kubeClusterView(c *ui.Context, name string) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Fill().Children(func() {
		a.clusterHeader(c, name)
		if s.logs != nil {
			a.logsView(c)
			return
		}
		split := s.kind == kindPods || s.kind == kindDeployments
		var viewport ui.Element
		if split {
			viewport = ui.Column(c.Key("cluster-workloads"))
		} else {
			viewport = ui.Scroll(c.Key("cluster-resources"))
		}
		viewport.Grow(1).MinHeight(0).Children(func() {
			body := ui.Column(c).Gap(14).Padding(20, 28, 24, 28)
			if split {
				body.Grow(1).MinHeight(0)
			}
			body.Children(func() {
				a.kindChips(c)
				a.forwardList(c, name)
				_, hasKubectl := cliCopy(a.cliState(), "kubectl")
				waiting := s.data == nil || (s.kind == kindServices) != (s.data.Services != nil) || s.kind == kindOverview && s.data.Overview == nil || !isBatchKind(s.kind) && s.batchData || isBatchKind(s.kind) && s.data.Batch == nil
				switch {
				case a.Snapshot == nil:
					toolNote(c, locales.CleanupWaiting())
				case !hasKubectl:
					toolNote(c, locales.KubeClusterNoKubectl())
				case waiting && s.err != nil:
					a.clusterError(c, name, s.err)
				case waiting:
					ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(12, 0).Children(func() {
						ui.Spinner(c).Size(14, 14)
						ui.Text(c, locales.KubeClusterReading(name)).FontSize(12.5).TextColor(t.TextMuted)
					})
				default:
					if s.err != nil {
						ui.Text(c, locales.KubeClusterStale(s.err.Error())).FontSize(11.5).TextColor(pal.WarningText).Wrap()
					}
					switch s.kind {
					case kindOverview:
						a.clusterOverview(c, s.data)
					case kindDeployments:
						a.deploymentsView(c, s.data)
					case kindPods:
						a.podsView(c, s.data)
					case kindJobs:
						a.jobsView(c, s.data.Batch)
					case kindCronJobs:
						a.cronJobsView(c, s.data.Batch)
					case kindServices:
						a.servicesView(c, s.data.Services)
					}
				}
			})
		})
	})
}

// toggleWorkload keeps the clicked row in view when the list changes width.
func (s *kubeClusterState) toggleWorkload(selection *string, uid string) {
	s.revealWorkload = uid
	if *selection == uid {
		*selection = ""
		return
	}
	*selection, s.tab = uid, 0
}

func (s *kubeClusterState) revealWorkloadRow(row ui.Element, uid string) {
	if s.revealWorkload == uid {
		row.ScrollIntoView()
		s.revealWorkload = ""
	}
}

// workloadPanes keeps the list's scroll position as the details open and
// gives each selected resource its own details viewport.
func workloadPanes(c *ui.Context, header, list func(), uid string, detail func()) {
	ui.Row(c).Grow(1).MinHeight(0).Gap(16).AlignItems(ui.Stretch).Children(func() {
		left := ui.Column(c).MinWidth(0).MinHeight(0)
		if uid == "" {
			left.Grow(1)
		} else if narrow(c) {
			left.Width(300).Shrink(0)
		} else {
			left.Width(390).Shrink(0)
		}
		left.Children(func() {
			border := Colors(c.Theme()).Divider
			ui.Column(c).Radius(12, 12, 0, 0).Clip().Shrink(0).BorderWidth(1, 1, 0, 1).BorderColor(border).Children(header)
			ui.Scroll(c.Key("workload-list")).Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).Radius(0, 0, 12, 12).Clip().BorderWidth(0, 1, 1, 1).BorderColor(border).Children(list)
			})
		})
		if uid != "" {
			ui.Column(c.Key("workload-detail-" + uid)).Grow(1).MinWidth(0).MinHeight(0).Children(detail)
		}
	})
}

// clusterHeader is the page's path: context, namespaces and kind, each a
// select; and how fresh the data is, with a button reading it again.
func (a *App) clusterHeader(c *ui.Context, name string) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	right := float32(20)
	if bar := c.TitleBar(); bar.Right > 0 {
		right = bar.Right + 12
	}
	ui.Row(c).Height(56).Shrink(0).AlignItems(ui.Center).Gap(6).Padding(0, right, 0, 24).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.Divider).DragWindow().Children(func() {
		headerBackButton(c, locales.BackToKubernetes(), func() { a.router.Push(kubePage) })
		anchor := pathButton(c, "cluster-context", "brand/kubernetes", name)
		if anchor.Clicked() {
			s.contextOpen = !s.contextOpen
		}
		ui.Popover(c, anchor, &s.contextOpen, func() { a.contextPopover(c, name) })
		pathSlash(c)
		ns := a.clusterNamespaces(name)
		label := strings.Join(ns, ", ")
		if len(ns) == 0 {
			label = locales.KubeClusterAllNamespaces()
		}
		anchor = pathButton(c, "cluster-namespace", "lucide/folder-tree", label)
		if anchor.Clicked() {
			s.namespaceOpen = !s.namespaceOpen
			if s.namespaceOpen {
				a.listNamespaces(name)
			}
		}
		ui.Popover(c, anchor, &s.namespaceOpen, func() { a.namespacePopover(c, name) })
		pathSlash(c)
		kind := clusterKindOptions()[s.kind]
		anchor = pathButton(c, "cluster-kind", "lucide/"+kind.icon, kind.label)
		anchor.OnClick(a.toggleClusterKinds)
		ui.Popover(c, anchor, &s.kindsOpen, func() { a.clusterKindPicker(c) })
		if s.logs != nil {
			a.logsCrumb(c)
			ui.Box(c).Grow(1).Height(1)
			ui.Button(c.Key("logs-copy"), "").Label(locales.KubeLogsCopy()).Disabled(len(s.logs.lines) == 0).OnClick(func() {
				c.WriteClipboard(s.logs.text())
				c.Toast(locales.KubeLogsCopied(len(s.logs.lines)))
			}).Children(func() {
				ui.Icon(c, Icon("copy")).Size(14, 14)
				ui.Text(c, locales.KubeLogsCopy()).SingleLine()
			})
			return
		}
		ui.Box(c).Grow(1).Height(1)
		a.freshness(c)
		iconButton(c, "refresh-cw", locales.KubeClusterRefresh()).OnClick(func() { s.refresh++ })
	})
}

// pathButton is a select of the header's path: an icon, the value and a
// chevron.
func pathButton(c *ui.Context, key, icon, value string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Height(30).Padding(0, 8).Radius(7).AlignItems(ui.Center).Gap(7).Label(value)
	if b.Hovered() {
		b.Background(Colors(t).Muted)
	}
	return b.Children(func() {
		iconOrLogo(c, icon, 16, t.TextMuted)
		ui.Text(c, value).FontSize(14).FontWeight(600).SingleLine().MaxWidth(260)
		ui.Icon(c, Icon("chevron-down")).Size(12, 12).TextColor(Colors(t).TextFaint)
	})
}

func pathSlash(c *ui.Context) {
	ui.Text(c, "/").FontSize(14).TextColor(Colors(c.Theme()).TextFaint)
}

// freshness says when the data was read: as it keeps coming, how long ago
// the last read was; after a failure, why.
func (a *App) freshness(c *ui.Context) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	dot, text := pal.TextFaint, locales.KubeClusterConnecting()
	switch {
	case s.err != nil:
		dot, text = t.Warning, a.accessWords(s.err)
	case s.data != nil:
		since := max(c.Now().Sub(s.data.Taken), 0)
		c.After(time.Second - since%time.Second)
		dot, text = t.Success, locales.KubeClusterFresh(int(since/time.Second))
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(dot)
		ui.Text(c, text).FontSize(11.5).TextColor(t.TextMuted).SingleLine().MaxWidth(260)
	})
}

// accessWords is what a failed read says in the header: the access
// check's words for what kubectl said.
func (a *App) accessWords(err error) string {
	kerr, ok := errors.AsType[*kube.Error](err)
	if !ok {
		return locales.KubeAccessUnknown()
	}
	switch kerr.Access.State {
	case detect.KubeUnreachable:
		return locales.KubeAccessUnreachable()
	case detect.KubeNoAuthProgram:
		return locales.KubeAccessNoProgram(kerr.Access.Detail)
	case detect.KubeNeedsLogin:
		return locales.KubeAccessLogin()
	case detect.KubeAuthFailed:
		return locales.KubeAccessAuthFailed()
	case detect.KubeForbidden:
		return locales.KubeClusterForbidden()
	}
	return locales.KubeAccessUnknown()
}

// clusterError is the page when nothing could be read: why, kubectl's
// own line, and a button to read again.
func (a *App) clusterError(c *ui.Context, name string, err error) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	title, text := locales.KubeClusterUnknownTitle(), locales.KubeClusterUnknownText()
	detail := err.Error()
	if kerr, ok := errors.AsType[*kube.Error](err); ok {
		detail = kerr.Access.Detail
		switch kerr.Access.State {
		case detect.KubeUnreachable:
			title, text = locales.KubeClusterUnreachableTitle(), locales.KubeClusterUnreachableText(name)
		case detect.KubeNoAuthProgram:
			title, text = locales.KubeClusterNoProgramTitle(kerr.Access.Detail), locales.KubeClusterNoProgramText(name, kerr.Access.Detail)
			detail = ""
		case detect.KubeNeedsLogin, detect.KubeAuthFailed:
			title, text = locales.KubeClusterLoginTitle(), locales.KubeClusterLoginText(name)
		case detect.KubeForbidden:
			title, text = locales.KubeClusterForbiddenTitle(), locales.KubeClusterForbiddenText(strings.Join(a.clusterNamespaces(name), ", "))
		}
	}
	ui.Column(c).MaxWidth(500).Gap(12).Padding(40, 0, 0, 0).AlignSelf(ui.Center).Children(func() {
		ui.Box(c).Size(48, 48).Radius(12).Background(pal.WarningSoft).Center().Children(func() {
			ui.Icon(c, Icon("log-in")).Size(22, 22).TextColor(pal.WarningText)
		})
		ui.Text(c, title).FontSize(19).FontWeight(600).Wrap()
		ui.Text(c, text).FontSize(12.5).TextColor(t.TextMuted).Wrap()
		if detail != "" {
			ui.Text(c, detail).Font(FontMono).FontSize(11.5).Padding(9, 12).Radius(8).Background(pal.Subtle).
				BorderWidth(1).BorderColor(pal.Line).Wrap().Selectable()
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c.Key("cluster-retry"), locales.KubeClusterRetry()).OnClick(func() { s.refresh++ })
			ui.Button(c.Key("cluster-contexts"), locales.KubeClusterAllContexts()).OnClick(func() { a.router.Push(kubePage) })
		})
	})
}

// contextPopover chooses the context the page shows, by kubeconfig file,
// with what the last access check found; it does not change kubectl's,
// which its own plan does.
func (a *App) contextPopover(c *ui.Context, name string) {
	t := c.Theme()
	pal := Colors(t)
	cli := a.cliState()
	current := kubeCurrent(cli)
	ui.Column(c).Width(440).Gap(2).Padding(6).Children(func() {
		for _, f := range cli.KubeFiles {
			var ks []inventory.KubeContext
			for _, k := range cli.KubeContexts {
				if k.File == f.Path && !k.Shadowed {
					ks = append(ks, k)
				}
			}
			if len(ks) == 0 {
				continue
			}
			ui.Text(c, tilde(f.Path, a.Home)).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).Padding(8, 10, 4, 10)
			for _, k := range ks {
				row := ui.ButtonBase(c.Key("context-"+k.File+"\n"+k.Name)).Padding(7, 10).Radius(7).AlignItems(ui.Center).Gap(10).Label(k.Name)
				if k.Name == name {
					row.Background(pal.ActiveRow)
				} else if row.Hovered() {
					row.Background(pal.Muted)
				}
				row.OnClick(func() {
					a.cluster.contextOpen = false
					if k.Name != name {
						a.router.Replace(kubeClusterPath(k.Name))
					}
				}).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							clipped(c, k.Name).FontSize(13).FontWeight(500)
							if current != nil && current.Name == k.Name {
								aiTag(c, locales.KubeKubectlNow(), pal.InfoSoft, pal.InfoText)
							}
						})
						clipped(c, locales.KubeCluster(k.Cluster)).FontSize(10.5).TextColor(pal.TextFaint)
					})
					if ch, ok := a.kubeChecks[k.Name]; ok {
						a.kubeAccessTag(c, k, ch)
					}
				})
			}
		}
		ui.Box(c).Height(1).Background(pal.Line).Margin(4, 0)
		note := locales.KubeClusterPickNoteNone()
		if current != nil {
			note = locales.KubeClusterPickNote(current.Name)
		}
		ui.Row(c).Gap(8).Padding(4, 10).Children(func() {
			ui.Icon(c, Icon("info")).Size(14, 14).TextColor(pal.Info)
			ui.Text(c, note).FontSize(11.5).TextColor(t.TextMuted).Wrap().Grow(1)
		})
		ui.Row(c).Gap(8).Padding(4, 10, 6, 10).AlignItems(ui.Center).Children(func() {
			in, found := cliCopy(cli, "kubectl")
			if found && a.BuildPlan != nil && (current == nil || current.Name != name) && a.kubeContext(name) != nil {
				from := ""
				if current != nil {
					from = current.Name
				}
				ui.Button(c.Key("context-use"), locales.KubeClusterUse(name)).OnClick(func() {
					a.cluster.contextOpen = false
					a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
						return ops.KubeContextPlan(in.Path, from, name, env, run.Exec{})
					}, a.proposeOrSay)
				})
			}
			ui.Box(c).Grow(1)
			ui.Button(c.Key("context-all"), locales.KubeClusterAllContexts()+" ›").OnClick(func() {
				a.cluster.contextOpen = false
				a.router.Push(kubePage)
			})
		})
	})
}

// namespacePopover chooses the namespaces the page reads: all of them,
// or some of those the cluster lists, or one typed when it lets no one
// list them.
func (a *App) namespacePopover(c *ui.Context, name string) {
	s := &a.cluster
	t := c.Theme()
	pal := Colors(t)
	chosen := a.clusterNamespaces(name)
	ui.Column(c).Width(300).Gap(4).Padding(10).Children(func() {
		all := len(chosen) == 0
		if ui.Checkbox(c.Key("ns-all"), &all, locales.KubeClusterAllNamespaces()).Changed() {
			if all {
				a.setNamespaces(name, []string{})
			} else if k := a.kubeContext(name); k != nil {
				a.setNamespaces(name, []string{namespaceOf(*k)})
			} else {
				a.setNamespaces(name, []string{"default"})
			}
		}
		ui.Box(c).Height(1).Background(pal.Line).Margin(4, 0)
		names := slices.Clone(s.nsNames)
		for _, ns := range chosen {
			if !slices.Contains(names, ns) {
				names = append(names, ns)
			}
		}
		slices.Sort(names)
		switch {
		case s.nsLoading:
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(12, 12)
				ui.Text(c, locales.KubeClusterListingNamespaces()).FontSize(11.5).TextColor(t.TextMuted)
			})
		case s.nsErr != nil:
			ui.Text(c, locales.KubeClusterNoNamespaces(a.accessWords(s.nsErr))).FontSize(11.5).TextColor(pal.WarningText).Wrap()
		}
		ui.Scroll(c).MaxHeight(280).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				for _, ns := range names {
					on := slices.Contains(chosen, ns)
					if ui.Checkbox(c.Key("ns-"+ns), &on, ns).Changed() {
						next := slices.DeleteFunc(slices.Clone(chosen), func(n string) bool { return n == ns })
						if on {
							next = append(next, ns)
							slices.Sort(next)
						}
						if len(next) == 0 {
							// None chosen is not all: keep the last one.
							next = []string{ns}
						}
						a.setNamespaces(name, next)
					}
				}
			})
		})
		if s.nsErr != nil {
			ui.TextInput(c.Key("ns-typed"), &s.nsTyped).Placeholder(locales.KubeClusterTypeNamespace()).OnSubmit(func() {
				if ns := strings.TrimSpace(s.nsTyped); ns != "" {
					a.setNamespaces(name, []string{ns})
					s.nsTyped, s.namespaceOpen = "", false
				}
			})
		}
	})
}

// kindChips are the kinds the page lists, each with how many of them
// need attention.
func (a *App) kindChips(c *ui.Context) {
	s := &a.cluster
	options := clusterKindOptions()
	ui.Row(c).Gap(6).Wrap().Children(func() {
		if s.kind != kindOverview {
			k := options[kindOverview]
			kindChip(c, &s.kind, kindOverview, k.icon, k.label, 0)
		}
		for _, i := range []int{kindPods, kindDeployments, kindJobs, kindCronJobs, kindServices} {
			k := options[i]
			_, trouble, _ := clusterKindCounts(s.data, i)
			kindChip(c, &s.kind, i, k.icon, k.label, trouble)
		}
		t := c.Theme()
		pal := Colors(t)
		b := ui.ButtonBase(c.Key("all-kinds")).Height(28).Padding(0, 11).Radius(14).AlignItems(ui.Center).Gap(6).
			BorderWidth(1).BorderColor(pal.ControlBorder).Label(locales.KubeKindsAll()).OnClick(a.toggleClusterKinds)
		if b.Hovered() {
			b.Background(pal.Muted)
		}
		b.Children(func() {
			ui.Icon(c, Icon("plus")).Size(14, 14).TextColor(t.TextMuted)
			ui.Text(c, locales.KubeKindsAll()).FontSize(12.5).TextColor(t.TextMuted)
			if !narrow(c) {
				ui.Text(c, cmdKey("J")).FontSize(10).TextColor(pal.TextFaint)
			}
		})
	})
}

func kindChip(c *ui.Context, kind *int, value int, icon, label string, trouble int) {
	t := c.Theme()
	pal := Colors(t)
	chosen := *kind == value
	b := ui.ButtonBase(c.Key("kind-"+label)).Height(28).Padding(0, 11).Radius(14).AlignItems(ui.Center).Gap(6).
		BorderWidth(1).BorderColor(pal.ControlBorder).Label(label)
	fg := t.Text
	switch {
	case chosen:
		b.Background(t.Text).BorderColor(t.Text)
		fg = t.Background
	case b.Hovered():
		b.Background(pal.Muted)
	}
	b.OnClick(func() { *kind = value }).Children(func() {
		ui.Icon(c, Icon(icon)).Size(14, 14).TextColor(fg)
		ui.Text(c, label).FontSize(12.5).FontWeight(500).TextColor(fg)
		if trouble > 0 {
			ui.Text(c, strconv.Itoa(trouble)).FontSize(11).TextColor(t.Danger)
		}
	})
}

// podTrouble reports a pod that needs attention: a reason, a phase gone
// wrong or containers not ready while it runs.
func podTrouble(p kube.Pod) bool {
	switch p.Phase {
	case "Succeeded":
		return false
	case "Failed", "Unknown", "Pending":
		return true
	}
	return p.Reason != "" || p.Ready < len(p.Containers)
}

// deploymentTrouble reports a Deployment with fewer pods ready than it
// wants.
func deploymentTrouble(d kube.Deployment) bool { return d.Ready < d.Replicas }

// age says how old a resource is, as kubectl does: 40s, 5m, 2h, 12d.
func age(c *ui.Context, t time.Time) string { return span(c.Now().Sub(t)) }
