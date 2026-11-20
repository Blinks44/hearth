package ui

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

const allLogPods = 1

type logSource struct {
	restarts int
	previous bool
	stop     func()
	ended    bool
	err      error
}

func (a *App) ownerLogsButton(c *ui.Context, ref ops.KubeResource) {
	if a.StreamLogs != nil && a.WatchCluster != nil {
		kubeActionButton(c.Key("owner-logs"), "scroll-text", locales.KubeLogsOpen(), podActionHeight).
			OnClick(func() { a.openOwnerLogs(ref) })
	}
}

func (a *App) openOwnerLogs(ref ops.KubeResource) {
	a.cluster.logs = &logView{
		owner: kube.LogOwner{Kind: ref.Kind, Namespace: ref.Namespace, Name: ref.Name, UID: ref.UID},
		scope: allLogPods, since: 2, times: true,
	}
}

func (l *logView) ownerContainers() []string {
	var names []string
	for _, p := range l.pods {
		if l.scope != allLogPods && p.UID != l.pod {
			continue
		}
		for _, ct := range p.Containers {
			if !slices.Contains(names, ct.Name) {
				names = append(names, ct.Name)
			}
		}
	}
	return names
}

func (a *App) syncOwnerLogs(l *logView, kubectl string) {
	if len(l.pods) > 0 && !slices.ContainsFunc(l.pods, func(p kube.Pod) bool { return p.UID == l.pod }) {
		l.pod = l.pods[0].UID
	}
	if names := l.ownerContainers(); len(names) > 0 && !slices.Contains(names, l.container) {
		l.container = names[0]
	}
	podUID := ""
	if l.scope != allLogPods {
		podUID = l.pod
	}
	context := a.router.Query("context")
	key := fmt.Sprintf("%p%q", l, []string{kubectl, context, l.container, podUID, strconv.Itoa(l.since)})
	if a.setLogRun(key) {
		l.sources = make(map[string]*logSource)
		a.cluster.logStop = func() {
			for uid, source := range l.sources {
				delete(l.sources, uid)
				source.stop()
			}
		}
	}

	keep := make(map[string]bool, len(l.pods))
	active, matched := 0, 0
	l.err = nil
	for _, p := range l.pods {
		if podUID != "" && p.UID != podUID {
			continue
		}
		i := slices.IndexFunc(p.Containers, func(ct kube.Container) bool { return ct.Name == l.container })
		if i < 0 {
			continue
		}
		matched++
		ct := p.Containers[i]
		previous := ct.State == "waiting" && ct.Last != nil
		source := l.sources[p.UID]
		if previous && source != nil && source.restarts == ct.Restarts-1 && !source.previous && source.err == nil {
			// This stream already read the run that just became previous.
			keep[p.UID] = true
			active++
			continue
		}
		if ct.State == "" || ct.State == "waiting" && !previous {
			active++
			continue
		}
		keep[p.UID] = true
		if source != nil && (source.restarts != ct.Restarts || source.previous != previous) {
			delete(l.sources, p.UID)
			source.stop()
			source = nil
		}
		if source == nil {
			source = &logSource{restarts: ct.Restarts, previous: previous}
			l.sources[p.UID] = source
			source.stop = a.StreamLogs(kube.Logs{
				Kubectl: kubectl, Context: context, Namespace: p.Namespace, Pod: p.Name, Container: ct.Name,
				Since: logPeriods[l.since], Previous: previous, Terminated: ct.State == "terminated",
			}, func(lines []kube.LogLine) {
				if a.cluster.logs != l || l.sources[p.UID] != source {
					return
				}
				for i := range lines {
					lines[i].Pod = p.Name
				}
				l.addLines(lines)
			}, func(err error) {
				if a.cluster.logs == l && l.sources[p.UID] == source {
					source.ended, source.err = true, err
				}
			})
		}
		if !source.ended || previous {
			active++
		}
		if l.err == nil && source.err != nil {
			l.err = fmt.Errorf("%s: %w", p.Name, source.err)
		}
	}
	for uid, source := range l.sources {
		if !keep[uid] {
			delete(l.sources, uid)
			source.stop()
		}
	}
	l.ended = matched > 0 && active == 0
}

func (l *logView) ownerLogControls(c *ui.Context) {
	segments(c, "logs-scope", &l.scope, []segment{
		{locales.KubeLogsOnePod(), ""}, {locales.KubeLogsAllPods(), strconv.Itoa(len(l.pods))},
	})
	if l.scope != allLogPods {
		for _, p := range l.pods {
			if p.UID != l.pod {
				continue
			}
			logSelect(c, "logs-source-pod", "", p.Name).PopoverMenuButton(func(m *ui.Menu) {
				for _, other := range l.pods {
					if m.Item(other.Name).Radio(other.UID == l.pod).Chosen() {
						l.pod = other.UID
					}
				}
			})
			break
		}
	}
}
