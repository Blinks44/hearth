package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
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
	kindHelm
	kindSecrets
	kindStatefulSets
	kindDaemonSets
	kindReplicaSets
	kindIngresses
	kindEndpoints
	kindNetworkPolicies
	kindConfigMaps
	kindPVCs
	kindPVs
	kindStorageClasses
	kindNodes
	kindPolicies
	kindNamespaces
	kindServiceAccounts
	kindAccess
	kindCustom
)

// kubeClusterState is the cluster page's: what it reads and what it read.
type kubeClusterState struct {
	// namespaces are those chosen by context; a context without an entry
	// reads its own namespace, an empty entry all of them.
	namespaces map[string][]string
	kind       int
	entry      clusterEntry
	// target is what the data was read from; running is what the watch
	// reads, with refresh, and stop ends it.
	target, running string
	refresh         int
	watch           int
	stop            func()
	data            *kube.Cluster
	batchData       bool
	// cache is the last read of each kind's view of the target, by what
	// it reads: going back to a kind shows it at once while it is read
	// again.
	cache    map[string]kube.Cluster
	dataView string
	err      error
	// failedAt is when the last read failed, after data was read.
	failedAt time.Time
	// reading is true from a read the user is waiting for, the first of a
	// watch, to its answer: the reads that follow every few seconds are
	// quiet.
	reading bool

	contextOpen, namespaceOpen bool
	kindsOpen                  bool
	kindQuery                  string
	kindSelected               int
	kindScroll                 bool
	jobFilter, cronJobFilter   int
	serviceFilter              int
	service                    string
	helmFilter                 int
	helmDetail                 *helmDetails
	secret                     secretView
	forward                    *forwardForm
	shell                      *shellForm
	other                      otherResourceView
	custom                     customResourceView
	resourceLink               resourceLink
	claimFilter, nodeFilter    int
	namespaceFilter            int
	namespaceCreate            *namespaceCreateForm
	access                     accessView
	policyTab                  int
	resources                  map[ops.KubeResource]*kubeResourceForm
	resourceDialog             *kubeResourceForm
	scale                      *scaleForm
	bulk                       resourceSelection
	// nsNames are the context's namespaces, listed as the namespace
	// popover opened; nsErr says why they could not be.
	nsRequest *kubeNamespaceRequest
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
	// toggled are the owner groups shown otherwise than by default:
	// open, as groups show closed.
	toggled map[string]bool
	// sorts are the tables' orders, by kind.
	sorts map[int]*tableSort
	// envShown are the pods' variables that look like secrets shown, by
	// pod UID, container and name.
	envShown map[string]bool
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

// clusterEntry records access for the last context opened this session.
type clusterEntry struct {
	context  inventory.KubeContext
	kubectl  string
	verified bool
}

func (a *App) kubeEntryPath() string {
	entry := a.cluster.entry
	current := a.kubeContext(entry.context.Name)
	kubectl, _ := cliCopy(a.cliState(), "kubectl")
	if a.WatchCluster == nil || !entry.verified || current == nil || *current != entry.context || kubectl.Path != entry.kubectl {
		return kubePage
	}
	return kubeClusterPath(entry.context.Name)
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
	if request := s.nsRequest; request != nil && (!s.namespaceOpen ||
		a.router.Path() != kubeClusterPage || a.router.Query("context") != request.context) {
		s.namespaceOpen = false
		s.stopNamespaceList()
	}
	var t kube.Target
	target := ""
	if a.router.Path() == kubeClusterPage && a.WatchCluster != nil {
		name := a.router.Query("context")
		if s.custom.context != name {
			s.custom.reset(name)
		}
		if name != "" {
			in, _ := cliCopy(a.cliState(), "kubectl")
			if ctx := a.kubeContext(name); ctx == nil {
				s.entry = clusterEntry{}
			} else if s.entry.context != *ctx || s.entry.kubectl != in.Path {
				check := a.kubeChecks[name]
				verified := check.context == *ctx && check.kubectl == in.Path && !check.running && check.res.State == detect.KubeReachable
				s.entry = clusterEntry{context: *ctx, kubectl: in.Path, verified: verified}
			}
			t = kube.Target{Kubectl: in.Path, Context: name, Namespaces: a.clusterNamespaces(name)}
			t.Overview = s.kind == kindOverview
			t.Metrics = s.kind == kindPods || t.Overview || s.kind == kindNodes
			t.Batch = t.Overview || isBatchKind(a.cluster.kind)
			t.Secrets = s.kind == kindSecrets
			t.Services = s.kind == kindServices
			t.Resource = otherResourceKind(s.kind)
			t.Policies = s.kind == kindPolicies
			if s.kind == kindAccess {
				t.AccessNamespace = a.accessNamespace(name)
			}
			t.DeploymentAutoscalers = s.kind == kindDeployments
			if s.kind == kindCustom {
				t.CustomKind = s.custom.name
			}
			target = fmt.Sprintf("%q", []string{t.Kubectl, t.Context, strings.Join(t.Namespaces, ","), t.AccessNamespace,
				s.entry.context.File, s.entry.context.Cluster, s.entry.context.Namespace, s.entry.context.Auth})
			if s.kind == kindHelm {
				if helm, found := cliCopy(a.cliState(), "helm"); found {
					t.Helm, t.HelmVersion = helm.Path, helm.Version
					target += fmt.Sprintf("%q", []string{t.Helm, t.HelmVersion})
				} else {
					target = ""
				}
			}
		}
	}
	if target != s.target || s.kind != s.bulk.kind {
		s.bulk.clear()
		s.bulk.kind = s.kind
		s.shell = nil
		s.service = ""
		s.other.reset()
		s.access.resetSelection()
		s.namespaceCreate = nil
		if s.resourceLink.kind != s.kind {
			s.resourceLink = resourceLink{}
		}
	}
	running := ""
	if target != "" && (s.kind != kindCustom || t.CustomKind != "") {
		if target == s.target {
			t.Events, t.Tail = a.detailRef(), a.detailTail()
		}
		running = fmt.Sprintf("%s#%d#%s#%s#%+v", target, s.refresh, viewKey(t), t.Events.UID, t.Tail)
	}
	ownerLog := s.logs
	if running != "" && target == s.target && ownerLog != nil && ownerLog.owner.UID != "" {
		t.LogOwner = ownerLog.owner
		running += fmt.Sprintf("#logs:%p", ownerLog)
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
		s.clearHelmDetails()
		s.target, s.data, s.err, s.failedAt = target, nil, nil, time.Time{}
		s.logs, s.forward, s.revealWorkload = nil, nil, ""
		s.resourceLink = resourceLink{}
		s.cache = nil
	}
	view := target + viewKey(t)
	if cached, ok := s.cache[view]; ok && running != "" && t.LogOwner.UID == "" && (s.data == nil || s.dataView != view) {
		s.data, s.dataView, s.err, s.failedAt = &cached, view, nil, time.Time{}
		s.batchData = t.Batch && !t.Overview
	}
	if running == "" {
		s.reading = false
		return
	}
	watch, refresh := s.watch, s.refresh
	entry, kind := s.entry, s.kind
	s.reading = true
	s.stop = a.WatchCluster(t, func(cl kube.Cluster, err error) {
		if s.watch != watch {
			return
		}
		s.reading = false
		if t.LogOwner.UID != "" {
			ownerLog.podsErr, ownerLog.podsRead = err, true
			if err == nil {
				ownerLog.pods = cl.Pods
			}
			return
		}
		check := a.kubeChecks[t.Context]
		checking := check.running && check.context == entry.context && check.kubectl == entry.kubectl
		if kind == s.kind && entry.kubectl == s.entry.kubectl && entry.context == s.entry.context && !checking {
			s.entry.verified = err == nil && kubeViewRead(kind, cl)
		}
		if err != nil {
			s.err, s.failedAt = err, time.Now()
			if kerr, ok := errors.AsType[*kube.Error](err); t.CustomKind != "" && ok && kerr.Access.State == detect.KubeForbidden {
				s.data = nil
				delete(s.cache, view)
				s.other.reset()
			}
			return
		}
		s.bulk.retain(cl)
		s.data, s.err, s.failedAt = &cl, nil, time.Time{}
		if s.cache == nil {
			s.cache = map[string]kube.Cluster{}
		}
		s.cache[view], s.dataView = cl, view
		if t.Helm != "" {
			a.syncHelmRelease(cl.HelmReleases)
		}
		if cl.Resources != nil && cl.Resources.Kind == kube.PersistentVolumeClaims &&
			(cl.Resources.Claims == nil || cl.Resources.Claims.ConsumersErr != nil) && s.claimFilter == 2 {
			s.claimFilter = 0
		}
		if s.refresh == refresh {
			s.followResourceLink(cl)
		}
		if s.other.uid != "" && s.otherResource() == nil {
			s.other.reset()
		}
		s.batchData = t.Batch && !t.Overview
	})
}

// Some views return independent replies and keep their errors in the snapshot.
func kubeViewRead(kind int, cl kube.Cluster) bool {
	switch kind {
	case kindJobs:
		return cl.Batch != nil && cl.Batch.JobsErr == nil
	case kindCronJobs:
		return cl.Batch != nil && cl.Batch.CronJobsErr == nil
	case kindAccess:
		return cl.Access != nil && (cl.Access.IdentityErr == nil || cl.Access.RulesErr == nil ||
			slices.ContainsFunc(cl.Access.Resources, func(list kube.ResourceList) bool { return list.Err == nil }))
	case kindPolicies:
		return slices.ContainsFunc(cl.Policies, func(list kube.ResourceList) bool { return list.Err == nil })
	default:
		return true
	}
}

// viewKey names what a target reads besides its cluster and namespaces:
// the kind's view, whose reads the cache keeps apart.
func viewKey(t kube.Target) string {
	return fmt.Sprintf("#%t#%t#%t#%t#%t#%t#%t#%s#%q#%s", t.Overview, t.Batch, t.Services, t.Secrets, t.Metrics, t.Policies,
		t.DeploymentAutoscalers, t.Resource, t.CustomKind, t.Helm)
}

// detailRef is the object whose details show, zero for none.
func (a *App) detailRef() kube.Ref {
	s := &a.cluster
	if s.data == nil {
		return kube.Ref{}
	}
	if item := s.otherResource(); item != nil {
		return kube.Ref{Namespace: item.Namespace, UID: item.UID}
	}
	if s.kind == kindServices {
		for _, service := range s.data.Services {
			if service.UID == s.service {
				return kube.Ref{Namespace: service.Namespace, UID: service.UID}
			}
		}
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
	a.cluster.clearHelmDetails()
	a.cluster.cache = nil
	a.cluster.namespaceOpen = false
	a.cluster.stopNamespaceList()
	if a.cluster.stop != nil {
		a.cluster.stop()
		a.cluster.stop, a.cluster.running, a.cluster.reading = nil, "", false
		a.cluster.watch++
	}
	a.stopLogs()
	a.cluster.shell = nil
	a.cluster.other.reset()
	a.cluster.custom.reset("")
	a.cluster.secret.reset()
	a.ClearSecretClipboard()
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

type kubeNamespaceRequest struct {
	context string
	stop    func()
}

func (s *kubeClusterState) stopNamespaceList() {
	if request := s.nsRequest; request != nil {
		s.nsRequest = nil
		if request.stop != nil {
			request.stop()
		}
	}
	s.nsLoading, s.nsNames, s.nsErr = false, nil, nil
}

// listNamespaces asks the context's cluster for its namespaces, for the
// namespace popover.
func (a *App) listNamespaces(name string) {
	s := &a.cluster
	in, _ := cliCopy(a.cliState(), "kubectl")
	if a.ListNamespaces == nil {
		return
	}
	s.stopNamespaceList()
	request := &kubeNamespaceRequest{context: name}
	s.nsRequest, s.nsLoading = request, true
	request.stop = a.ListNamespaces(kube.Target{Kubectl: in.Path, Context: name}, func(names []string, err error) {
		if s.nsRequest != request || !s.namespaceOpen || a.router.Path() != kubeClusterPage || a.router.Query("context") != name {
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
		a.resourceEditorDialog(c)
		a.namespaceCreate(c)
		a.kubeApplyView(c)
		if s.logs != nil {
			a.logsView(c)
			return
		}
		resourceKind := otherResourceKind(s.kind)
		split := s.kind == kindPods || s.kind == kindDeployments || s.kind == kindServices || s.kind == kindHelm || s.kind == kindSecrets || resourceKind != "" || s.kind == kindCustom || s.kind == kindPolicies || s.kind == kindAccess
		var viewport ui.Element
		if split {
			viewport = ui.Column(c.Key("cluster-workloads"))
		} else {
			viewport = ui.Scroll(c.Key("cluster-resources"))
		}
		viewport.Grow(1).MinHeight(0).Children(func() {
			bottom := float32(24)
			if len(s.bulk.selected) > 0 {
				bottom = 54
			}
			body := ui.Column(c).Gap(14).Padding(20, 28, bottom, 28)
			if split {
				body.Grow(1).MinHeight(0)
			}
			body.Children(func() {
				a.kindChips(c)
				a.forwardList(c, name)
				_, hasKubectl := cliCopy(a.cliState(), "kubectl")
				helm, hasHelm := cliCopy(a.cliState(), "helm")
				if a.Snapshot != nil && !hasKubectl {
					ui.Row(c.Key("kube-no-kubectl-notice")).Gap(10).Padding(8, 12).Radius(9).Background(pal.InfoSoft).AlignItems(ui.Center).Shrink(0).Children(func() {
						ui.Icon(c, Icon("info")).Size(15, 15).Shrink(0).TextColor(pal.InfoText)
						ui.Text(c, locales.KubeReadOnlyNoKubectl()).FontSize(12).TextColor(pal.InfoText).Wrap().Grow(1).MinWidth(0)
						a.installToolButton(c, "kubectl")
					})
				}
				waiting := s.data == nil || (s.kind == kindSecrets) != (s.data.Secrets != nil) || (s.kind == kindServices) != (s.data.Services != nil) || s.kind == kindOverview && s.data.Overview == nil || !isBatchKind(s.kind) && s.batchData || isBatchKind(s.kind) && s.data.Batch == nil
				if s.data != nil && (s.kind == kindAccess || s.data.Access != nil) {
					waiting = s.kind != kindAccess || s.data.Access == nil
				} else if s.kind == kindCustom && s.custom.name != "" {
					waiting = s.custom.list(s.data) == nil
				} else if s.data != nil && (s.kind == kindPolicies || s.data.Policies != nil) {
					waiting = s.kind != kindPolicies || s.data.Policies == nil
				} else if s.data != nil && (resourceKind != "" || s.data.Resources != nil) {
					waiting = s.data.Resources == nil || s.data.Resources.Kind != resourceKind
				}
				switch {
				case a.Snapshot == nil:
					toolNote(c, locales.CleanupWaiting())
				case s.kind == kindHelm && (!hasHelm || errors.Is(s.err, fs.ErrNotExist)):
					toolNote(c, locales.KubeHelmMissing())
					a.installPrimary(c, "helm")
				case s.kind == kindCustom && s.custom.name == "":
					a.customDefinitionsView(c)
				case waiting && s.err != nil:
					a.clusterError(c, name, s.err)
				case waiting:
					tableSkeleton(c, locales.KubeClusterReading(name))
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
						a.servicesView(c, s.data)
					case kindSecrets:
						a.secretsView(c, s.data.Secrets)
					case kindAccess:
						a.clusterAccessView(c, name, s.data.Access)
					case kindPolicies:
						view := *s.data
						view.Resources = s.activeResourceList(s.data)
						a.otherResourcesView(c, &view)
					case kindCustom:
						a.customResourcesView(c, s.data)
					case kindHelm:
						a.helmView(c, s.data.HelmReleases, helm.Version)
					default:
						a.otherResourcesView(c, s.data)
					}
					if s.resourceLink.missing && s.resourceLink.kind == s.kind {
						toolNote(c, locales.KubeStorageTargetMissing(s.resourceLink.name))
					}
				}
			})
		})
	})
}

// sortOf is the order of the kind's table.
func (s *kubeClusterState) sortOf(kind int) *tableSort {
	if s.sorts == nil {
		s.sorts = map[int]*tableSort{}
	}
	if s.sorts[kind] == nil {
		s.sorts[kind] = &tableSort{}
	}
	return s.sorts[kind]
}

// tableSkeleton stands for a table being read for the first time: its
// header and rows of bars under a line saying what is read; it fades as
// the table comes.
func tableSkeleton(c *ui.Context, reading string) {
	t := c.Theme()
	pal := Colors(t)
	bar := func(w float32) { ui.Box(c).Width(w).Height(8).Radius(4).Background(pal.Muted) }
	ui.Column(c.Key("table-skeleton")).Transition(skeletonExit).Gap(10).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(14, 14)
			ui.Text(c, reading).FontSize(12.5).TextColor(t.TextMuted)
		})
		ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
			ui.Row(c).Gap(40).Padding(12, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
				for _, w := range []float32{60, 50, 40, 50} {
					bar(w)
				}
			})
			for i, w := range []float32{220, 180, 260, 150, 200, 240} {
				ui.Row(c).Gap(10).Padding(14, 16).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
					ui.Box(c).Size(14, 14).Radius(4).Background(pal.Muted)
					ui.Box(c).Width(w).Height(10).Radius(5).Background(pal.Muted)
					ui.Box(c).Grow(1)
					ui.Box(c).Width(float32(60 + i%3*14)).Height(14).Radius(7).Background(pal.Muted)
					ui.Box(c).Width(120).Height(1)
				})
			}
		})
	})
}

// toggleWorkload keeps the clicked row in view when the list changes width.
func (s *kubeClusterState) toggleWorkload(selection *string, uid string) {
	s.revealWorkload = uid
	s.shell = nil
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
			ui.Column(c.Key("workload-detail-" + uid)).Transition(detailEnter).Grow(1).MinWidth(0).MinHeight(0).Children(detail)
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
	var contextWidth, namespaceWidth, kindWidth float32
	if s.logs != nil && narrow(c) {
		contextWidth, namespaceWidth, kindWidth = 76, 105, 92
	}
	ui.Row(c).Height(56).Shrink(0).AlignItems(ui.Center).Gap(6).Padding(0, right, 0, 24).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.Divider).DragWindow().Children(func() {
		headerBackButton(c, locales.BackToKubernetes(), func() { a.router.Push(kubePage) })
		anchor := pathButton(c, "cluster-context", "brand/kubernetes", name, contextWidth)
		if anchor.Clicked() {
			s.contextOpen = !s.contextOpen
		}
		ui.Popover(c, anchor, &s.contextOpen, func() { a.contextPopover(c, name) })
		pathSlash(c)
		ns := a.clusterNamespaces(name)
		if s.kind == kindAccess {
			ns = []string{a.accessNamespace(name)}
		}
		label := strings.Join(ns, ", ")
		if len(ns) == 0 {
			label = locales.KubeClusterAllNamespaces()
		}
		if s.logs != nil && s.logs.owner.UID != "" {
			label = s.logs.owner.Namespace
		}
		anchor = pathButton(c, "cluster-namespace", "lucide/folder-tree", label, namespaceWidth)
		if anchor.Clicked() {
			s.namespaceOpen = !s.namespaceOpen
			if s.namespaceOpen {
				a.listNamespaces(name)
			}
		}
		ui.Popover(c, anchor, &s.namespaceOpen, func() { a.namespacePopover(c, name) })
		pathSlash(c)
		kind := clusterKindOptions()[s.kind]
		if s.kind == kindCustom && s.custom.name != "" {
			kind.label = s.custom.label
		}
		anchor = pathButton(c, "cluster-kind", "lucide/"+kind.icon, kind.label, kindWidth)
		anchor.OnClick(a.toggleClusterKinds)
		ui.Popover(c, anchor, &s.kindsOpen, func() { a.clusterKindPicker(c) })
		if s.logs != nil {
			a.logsCrumb(c)
			ui.Box(c).Grow(1).Height(1)
			name := s.logs.owner.Name
			if name == "" {
				p, _, _ := a.logPod()
				name = p.Name
			}
			a.logActions(c, s.logs, name)
			return
		}
		ui.Box(c).Grow(1).Height(1)
		if cli, ok := cliCopy(a.cliState(), "kubectl"); ok && len(ns) == 1 && a.PickManifest != nil && a.BuildPlan != nil {
			iconButton(c.Key("cluster-apply"), "file-text", locales.KubeApplyOpen()).OnClick(func() {
				a.openProjectApply(projects.Project{}, cli.Path, name, ns[0])
			})
		}
		a.freshness(c)
		refresh := ui.ButtonBase(c.Key(locales.KubeClusterRefresh())).Size(24, 24).Radius(6).Center().
			Label(locales.KubeClusterRefresh()).Tooltip(locales.KubeClusterRefresh()).OnClick(func() {
			s.refresh++
			if s.kind == kindCustom {
				a.loadCustomDefinitions(name)
			}
		})
		if refresh.Hovered() {
			refresh.Background(pal.Muted)
		}
		refresh.Children(func() { spinIcon(c, "refresh-cw", 14, t.TextMuted, s.reading) })
	})
}

// pathButton is a select of the header's path: an icon, the value and a
// chevron.
func pathButton(c *ui.Context, key, icon, value string, width float32) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Height(controlHeight).Padding(0, 8).Radius(7).AlignItems(ui.Center).Gap(7).Label(value)
	if width > 0 {
		b.MinWidth(0).Shrink(1)
	}
	if b.Hovered() {
		b.Background(Colors(t).Muted)
	}
	return b.Children(func() {
		iconOrLogo(c, icon, 16, t.TextMuted)
		label := ui.Text(c, value).FontSize(14).FontWeight(600).SingleLine().MaxWidth(260)
		if width > 0 {
			label.MaxWidth(width).MinWidth(0).Shrink(1).Tooltip(value)
		}
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
	readErr := s.err
	var taken time.Time
	if s.kind == kindCustom && s.custom.name == "" {
		readErr = nil
		if catalog := s.custom.catalog; catalog != nil && catalog.result != nil {
			taken, readErr = catalog.taken, catalog.result.Err
		}
	} else if s.data != nil {
		taken = s.data.Taken
		if s.kind == kindAccess && s.access.tab == accessRulesTab && s.data.Access != nil {
			readErr = s.data.Access.RulesErr
		}
	}
	switch {
	case s.kind == kindHelm && a.Snapshot != nil && (cliFound(a.cliState(), []string{"helm"}) == 0 || errors.Is(s.err, fs.ErrNotExist)):
		dot, text = t.Warning, locales.KubeHelmUnavailable()
	case readErr != nil:
		dot, text = t.Warning, a.accessWords(readErr)
	case !taken.IsZero() || s.data != nil && s.kind != kindCustom:
		since := max(c.Now().Sub(taken), 0)
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
			if s.kind == kindCustom {
				title, text = locales.KubeCustomForbiddenTitle(), locales.KubeCustomForbiddenText()
			} else {
				title, text = locales.KubeClusterForbiddenTitle(), locales.KubeClusterForbiddenText(strings.Join(a.clusterNamespaces(name), ", "))
			}
		}
	}
	if s.kind == kindHelm {
		title, text = locales.KubeHelmReadFailed(), a.accessWords(err)
	}
	kerr, _ := errors.AsType[*kube.Error](err)
	icon := "log-in"
	if kerr != nil && (kerr.Access.State == detect.KubeUnreachable || kerr.Access.State == detect.KubeForbidden) {
		icon = "triangle-alert"
	}
	k := a.kubeContext(name)
	// The state is centered as the other empty states are: icon, title,
	// words and buttons on one axis, the checks under them full width.
	ui.Column(c).Width(520).MaxWidthPercent(100).Gap(12).Padding(56, 0, 0, 0).AlignSelf(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(48, 48).Radius(12).Background(pal.WarningSoft).Center().Children(func() {
			ui.Icon(c, Icon(icon)).Size(22, 22).TextColor(pal.WarningText)
		})
		ui.Text(c, title).FontSize(19).FontWeight(600).TextAlign(ui.Center).Wrap()
		ui.Text(c, text).FontSize(12.5).TextColor(t.TextMuted).TextAlign(ui.Center).Wrap()
		ui.Row(c).Gap(8).Margin(4, 0, 0, 0).Wrap().Justify(ui.Center).Children(func() {
			ui.PrimaryButton(c.Key("cluster-retry"), "").Height(podActionHeight).Label(locales.KubeClusterRetry()).OnClick(func() { s.refresh++ }).Children(func() {
				spinIcon(c, "refresh-cw", 14, c.Theme().AccentText, s.reading)
				ui.Text(c, locales.KubeClusterRetry()).FontSize(12).SingleLine()
			})
			if a.OpenTerminal != nil {
				kubeActionButton(c.Key("cluster-terminal"), "terminal", locales.KubeOpenTerminal(), podActionHeight).OnClick(func() {
					a.OpenTerminal(func(err error) {
						if err != nil {
							a.planErr = locales.KubeShellLaunchFailed(err.Error())
						}
					})
				})
			}
			kubeActionButton(c.Key("cluster-contexts"), "chevron-left", locales.KubeClusterAllContexts(), podActionHeight).OnClick(func() { a.router.Push(kubePage) })
		})
		// What Hearth checked, as the access check's steps.
		ui.Column(c).AlignSelf(ui.Stretch).Radius(10).Clip().BorderWidth(1).BorderColor(pal.Divider).Margin(4, 0, 0, 0).Children(func() {
			if k != nil {
				checkStep(c, true, locales.KubeStepContext(name), tilde(k.File, a.Home))
				if k.Auth != "" {
					found := kerr == nil || kerr.Access.State != detect.KubeNoAuthProgram
					label := locales.KubeStepProgramFound()
					if !found {
						label = locales.KubeStepProgramMissing()
					}
					checkStep(c, found, label, tilde(k.Auth, a.Home))
				}
			}
			if line := kubectlLine(detail); line != "" {
				checkStep(c, false, locales.KubeStepAnswer(), line)
			}
		})
	})
}

// checkStep is a step of an access check: done or not, what it is, and
// its detail in kubectl's words.
func checkStep(c *ui.Context, ok bool, label, detail string) {
	t := c.Theme()
	pal := Colors(t)
	ui.Row(c).Gap(10).Padding(9, 12).AlignItems(ui.Start).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		icon, color := "circle-check", t.Success
		if !ok {
			icon, color = "circle-alert", t.Warning
		}
		ui.Icon(c, Icon(icon)).Size(15, 15).Shrink(0).TextColor(color).Margin(1, 0, 0, 0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(12.5).FontWeight(500).Wrap()
			if detail != "" {
				ui.Text(c, detail).Font(FontMono).FontSize(11).TextColor(t.TextMuted).Wrap().Selectable()
			}
		})
	})
}

// klogPrefix is the header client-go's log puts before its lines, as
// "E1119 22:49:23.703365   18248 memcache.go:265] ".
var klogPrefix = regexp.MustCompile(`^[IWEF]\d{4} [\d:.]+\s+\d+ [\w./-]+:\d+\] `)

// kubectlLine is kubectl's answer without client-go's log headers,
// escaped quotes and repeated lines, at most three of them.
func kubectlLine(detail string) string {
	var lines []string
	for line := range strings.SplitSeq(strings.ReplaceAll(detail, `\"`, `"`), "\n") {
		line = strings.TrimSpace(klogPrefix.ReplaceAllString(strings.TrimSpace(line), ""))
		if quoted, ok := strings.CutPrefix(line, `"Unhandled Error" err="`); ok {
			line = strings.TrimSuffix(quoted, `"`)
		}
		if line != "" && !slices.Contains(lines, line) {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines[:min(len(lines), 3)], "\n")
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
	if s.kind == kindAccess {
		chosen = []string{a.accessNamespace(name)}
	}
	ui.Column(c).Width(300).Gap(4).Padding(10).Children(func() {
		if s.kind != kindAccess {
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
		}
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
					if s.kind == kindAccess {
						ui.Button(c.Key("ns-"+ns), ns).Disabled(on).OnClick(func() {
							a.setNamespaces(name, []string{ns})
							s.namespaceOpen = false
						})
						continue
					}
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

// kindChips are the cluster's overview, then the kinds pinned in ⌘J, each
// with how many of them need attention; a kind chosen there that is not
// pinned shows after them while it is chosen.
func (a *App) kindChips(c *ui.Context) {
	s := &a.cluster
	options := clusterKindOptions()
	ui.Row(c).Gap(6).Wrap().Children(func() {
		k := options[kindOverview]
		kindChip(c, &s.kind, kindOverview, k.icon, k.label, 0)
		pinned := a.pinnedKinds()
		if !slices.Contains(pinned, s.kind) && s.kind != kindOverview {
			pinned = append(pinned, s.kind)
		}
		for _, i := range pinned {
			k := options[i]
			if i == kindCustom && s.kind == kindCustom && s.custom.name != "" {
				k.label = s.custom.label
			}
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

// defaultPinned are the kinds pinned until the user pins others.
var defaultPinned = []string{"po", "deploy", "job", "cj", "svc", "secret"}

// pinnedKinds are the kinds pinned above the tables, in the order they
// were pinned.
func (a *App) pinnedKinds() []int {
	aliases := a.KubePinned
	if aliases == nil {
		aliases = defaultPinned
	}
	options := clusterKindOptions()
	var kinds []int
	for _, alias := range aliases {
		for i, k := range options {
			if k.alias == alias && i != kindOverview && !slices.Contains(kinds, i) {
				kinds = append(kinds, i)
			}
		}
	}
	return kinds
}

// togglePin pins the kind above the tables, or unpins it, and keeps the
// choice.
func (a *App) togglePin(kind int) {
	alias := clusterKindOptions()[kind].alias
	pinned := slices.Clone(a.KubePinned)
	if pinned == nil {
		pinned = slices.Clone(defaultPinned)
	}
	if i := slices.Index(pinned, alias); i >= 0 {
		pinned = slices.Delete(pinned, i, i+1)
	} else {
		pinned = append(pinned, alias)
	}
	// An empty list is kept as such, not as the defaults.
	a.KubePinned = append(make([]string, 0, len(pinned)), pinned...)
	a.savePreferences()
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
