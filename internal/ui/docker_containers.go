package ui

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type dockerContainersState struct {
	target        docker.Target
	watching      string
	revision      int
	stop          func()
	items         []docker.Container
	err, statsErr error
	loaded        bool
	filter        int
	query         string
	sort          tableSort
	open          map[string]bool
	bulk          resourceSelection
	planning      bool
	detail        dockerDetailState
	pending       *dockerContainerRequest
	logs          *dockerLogState
}

type dockerContainerRequest struct {
	target docker.Target
	item   docker.LogTarget
	logs   bool
}

func (s *dockerContainersState) stopWatch() {
	if s.stop != nil {
		s.stop()
	}
	s.stop, s.watching = nil, ""
	s.revision++
	s.planning = false
}

func (a *App) syncDockerContainers(t docker.Target, key string) {
	s := &a.engine.containers
	if a.engine.view != dockerContainers || a.WatchDockerContainers == nil {
		key = ""
	}
	if key == "" || s.pending != nil && s.pending.target != t {
		s.pending = nil
	}
	if key == s.watching {
		if s.loaded && s.pending != nil && !s.pending.logs {
			s.applyPendingContainer()
		}
		return
	}
	s.stopWatch()
	if t != (docker.Target{}) && t != s.target {
		s.closeLogs()
		s.closeDetail()
		s.items, s.err, s.statsErr, s.loaded = nil, nil, nil, false
		s.bulk.clear()
	} else if key == "" {
		s.closeLogs()
		s.closeDetail()
		s.bulk.clear()
	}
	if key == "" {
		return
	}
	s.target, s.watching = t, key
	if req := s.pending; req != nil && req.logs {
		s.pending = nil
		a.openDockerLogs(req.item.Name, []docker.LogTarget{{ID: req.item.ID}}, false)
	}
	revision := s.revision
	s.stop = a.WatchDockerContainers(t, func(items []docker.Container, err, statsErr error) {
		if s.revision != revision {
			return
		}
		s.err, s.statsErr = err, statsErr
		if err == nil {
			s.items, s.loaded = items, true
			s.applyPendingContainer()
			for id := range s.bulk.selected {
				if !slices.ContainsFunc(items, func(c docker.Container) bool { return c.ID == id }) {
					delete(s.bulk.selected, id)
				}
			}
		}
	})
}

func (s *dockerContainersState) applyPendingContainer() {
	req := s.pending
	if req == nil {
		return
	}
	s.pending = nil
	i := slices.IndexFunc(s.items, func(c docker.Container) bool {
		return req.item.ID != "" && (c.ID == req.item.ID || strings.HasPrefix(c.ID, req.item.ID))
	})
	if i < 0 && req.item.Name != "" {
		i = slices.IndexFunc(s.items, func(c docker.Container) bool { return c.Name == req.item.Name })
	}
	if i >= 0 {
		s.filter, s.query = dockerAll, strings.ToLower(s.items[i].Name)
		s.selectContainer(s.items[i].ID)
	}
}

const (
	dockerAll = iota
	dockerProblems
	dockerRunning
	dockerStopped
)

func showDockerContainer(item docker.Container, filter int, query string) bool {
	switch {
	case filter == dockerProblems && !item.Problem():
		return false
	case filter == dockerRunning && !item.Active():
		return false
	case filter == dockerStopped && item.Active():
		return false
	}
	return query == "" || strings.Contains(strings.ToLower(item.Name+" "+item.Image+" "+item.Ports), query) ||
		item.ConflictPort() > 0 && strings.Contains(strconv.Itoa(item.ConflictPort()), query)
}

type dockerGroup struct {
	key, name, kind, dir string
	compose              docker.ComposeProject
	items                []docker.Container
	running, problems    int
}

func (a *App) dockerGroups(items []docker.Container, filter int, query string) []dockerGroup {
	groups := make(map[string]*dockerGroup)
	for _, item := range items {
		g := dockerGroup{key: "standalone", name: locales.DockerNoProject()}
		switch {
		case item.Kind != "":
			g = dockerGroup{key: "kind:" + item.Kind, name: item.Kind, kind: "kind"}
		case item.Compose.Name != "":
			g = dockerGroup{key: "compose:" + item.Compose.Name + "\n" + item.Compose.Dir,
				name: item.Compose.Name, kind: "Compose", compose: item.Compose}
			for _, p := range a.Projects {
				if item.Compose.Dir != "" && filepath.Clean(item.Compose.Dir) == filepath.Clean(p.Dir) {
					g.dir = p.Dir
					break
				}
			}
		}
		if groups[g.key] == nil {
			groups[g.key] = &g
		}
		group := groups[g.key]
		if item.Problem() {
			group.problems++
		}
		if item.Active() {
			group.running++
		}
		if showDockerContainer(item, filter, query) {
			group.items = append(group.items, item)
		}
	}
	var result []dockerGroup
	for _, g := range groups {
		if len(g.items) > 0 {
			result = append(result, *g)
		}
	}
	if filter == dockerAll || filter == dockerStopped {
		for _, p := range a.Projects {
			project, ok := composeProject(p)
			if !ok || query != "" && !strings.Contains(strings.ToLower(p.Dir), query) {
				continue
			}
			if !slices.ContainsFunc(items, func(item docker.Container) bool {
				return item.Compose.Dir != "" && filepath.Clean(item.Compose.Dir) == filepath.Clean(p.Dir)
			}) {
				result = append(result, dockerGroup{key: "project:" + p.Dir, name: filepath.Base(p.Dir),
					kind: "Compose", dir: p.Dir, compose: project})
			}
		}
	}
	slices.SortFunc(result, func(a, b dockerGroup) int {
		order := func(g dockerGroup) int {
			switch {
			case g.problems > 0:
				return 0
			case g.kind == "Compose":
				return 1
			case g.kind == "kind":
				return 3
			default:
				return 2
			}
		}
		if n := cmp.Compare(order(a), order(b)); n != 0 {
			return n
		}
		return cmp.Compare(a.key, b.key)
	})
	return result
}

func composeProject(p projects.Project) (docker.ComposeProject, bool) {
	for _, f := range p.Files {
		switch f {
		case "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml":
			return docker.ComposeProject{Dir: p.Dir, Files: filepath.Join(p.Dir, f)}, true
		}
	}
	return docker.ComposeProject{}, false
}

func (s *dockerContainersState) groupOpen(g dockerGroup) bool {
	if open, ok := s.open[g.key]; ok {
		return open
	}
	return g.problems > 0 || g.kind == "" || s.query != "" || s.filter != dockerAll
}

func (s *dockerContainersState) setGroup(key string, open bool) {
	if s.open == nil {
		s.open = make(map[string]bool)
	}
	s.open[key] = open
}

func (a *App) dockerContainersView(c *ui.Context) {
	s := &a.engine.containers
	if !s.loaded {
		if s.err != nil {
			toolNote(c, s.err.Error())
		} else if a.WatchDockerContainers != nil {
			tableSkeleton(c, locales.DockerContainersReading())
		}
		return
	}
	if s.err != nil {
		toolNote(c, locales.KubeClusterStale(s.err.Error()))
	}
	if s.statsErr != nil {
		ui.Text(c, locales.DockerStatsUnavailable()).FontSize(11.5).TextColor(c.Theme().TextMuted).Tooltip(s.statsErr.Error())
	}
	var counts [4]int
	for _, item := range s.items {
		for tab := range counts {
			if showDockerContainer(item, tab, "") {
				counts[tab]++
			}
		}
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
		segments(c, "docker-filter", &s.filter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[dockerAll])},
			{locales.KubePodsTrouble(), strconv.Itoa(counts[dockerProblems])},
			{locales.DockerContainersRunning(), strconv.Itoa(counts[dockerRunning])},
			{locales.DockerContainersStopped(), strconv.Itoa(counts[dockerStopped])},
		})
		ui.Box(c).Grow(1)
		ui.SearchField(c.Key("docker-search"), &s.query).Label(locales.DockerContainerSearch()).Placeholder(locales.DockerContainerSearch()).Height(controlHeight).Width(270).MinWidth(110).Shrink(1)
	})
	groups := a.dockerGroups(s.items, s.filter, strings.ToLower(strings.TrimSpace(s.query)))
	for i := range groups {
		sortDockerContainers(s.sort, groups[i].items)
	}
	var ids []string
	running := 0
	for _, g := range groups {
		ids = append(ids, dockerContainerIDs(g.items)...)
		for _, item := range g.items {
			if item.Active() {
				running++
			}
		}
	}
	item, compact := s.selectedContainer()
	var detail func()
	tracks := dockerContainerTracks(c)
	columns := []sortColumn{
		{locales.KubeClusterName(), "name"},
		{locales.KubeClusterStatus(), "status"},
		{locales.PortsTitle(), "ports"},
		{locales.DockerUptime(), "uptime"},
		{"CPU", "cpu"},
		{locales.KubeMetricsMemory(), "memory"},
		{"", ""},
	}
	if compact {
		tracks = []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(112), ui.Fixed(54)}
		columns = []sortColumn{
			{locales.KubeClusterName(), "name"},
			{locales.KubeClusterStatus(), "status"},
			{"", ""},
		}
		detail = func() { a.dockerDetails(c, item) }
	}
	pal := Colors(c.Theme())
	hasBulk := len(s.bulk.selected) > 0
	workloadPanes(c.Key("docker-container-panes"), func() {
		sortHead(c, tracks, func() {
			s.bulk.checkbox(c, "docker-select-all", locales.KubeBulkSelectAll(), ids)
		}, &s.sort, columns...)
	}, func() {
		for _, g := range groups {
			a.dockerGroupRows(c.Key(g.key), g, tracks, compact)
		}
		if len(groups) == 0 {
			ui.Text(c, locales.DockerContainersEmpty()).FontSize(13).TextColor(c.Theme().TextMuted).Padding(12, 16)
		}
		ui.Row(c).Gap(8).Padding(8, 14).AlignItems(ui.Center).Background(pal.Subtle).Shrink(0).Children(func() {
			ui.Text(c, locales.DockerContainersCount(len(ids), running)).FontSize(11.5).TextColor(c.Theme().TextMuted).Wrap().Grow(1).MinWidth(0)
			expand := slices.ContainsFunc(groups, func(g dockerGroup) bool { return len(g.items) > 0 && !s.groupOpen(g) })
			label := locales.DockerCollapseAll()
			if expand {
				label = locales.DockerExpandAll()
			}
			if len(ids) > 0 {
				ui.Button(c.Key("expand-all"), label).Height(24).Shrink(0).OnClick(func() {
					for _, g := range groups {
						s.setGroup(g.key, expand)
					}
				})
			}
		})
		if hasBulk {
			ui.Box(c).Height(56).Shrink(0)
		}
	}, item.ID, detail)
	a.dockerBulkBar(c, s)
}

func dockerContainerTracks(c *ui.Context) []ui.Track {
	if narrow(c) {
		return []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(118), ui.Fixed(72), ui.Fixed(64), ui.Fixed(44), ui.Fixed(56), ui.Fixed(80)}
	}
	return []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(152), ui.Fixed(104), ui.Fixed(82), ui.Fixed(56), ui.Fixed(68), ui.Fixed(108)}
}

func sortDockerContainers(s tableSort, items []docker.Container) {
	statusRank := func(c docker.Container) int {
		switch {
		case c.Problem():
			return 0
		case c.State.Status == "restarting":
			return 1
		case c.State.Status == "running":
			return 2
		case c.State.Status == "paused":
			return 3
		case c.State.Status == "created":
			return 4
		default:
			return 5
		}
	}
	sortRows(s, items, map[string]func(x, y docker.Container) int{
		"name": func(x, y docker.Container) int {
			return cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		},
		"status": func(x, y docker.Container) int {
			if n := cmp.Compare(statusRank(x), statusRank(y)); n != 0 {
				return n
			}
			return cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		},
		"ports": func(x, y docker.Container) int {
			px, py := dockerPortLabel(x.Ports), dockerPortLabel(y.Ports)
			if (px == "") != (py == "") {
				if px == "" {
					return 1
				}
				return -1
			}
			return cmp.Compare(px, py)
		},
		"uptime": func(x, y docker.Container) int {
			ax, ay := x.Active() && !x.State.StartedAt.IsZero(), y.Active() && !y.State.StartedAt.IsZero()
			if ax != ay {
				if ax {
					return -1
				}
				return 1
			}
			if !ax {
				return 0
			}
			return x.State.StartedAt.Compare(y.State.StartedAt)
		},
		"cpu": func(x, y docker.Container) int {
			return cmp.Compare(parseDockerPercent(x.CPU), parseDockerPercent(y.CPU))
		},
		"memory": func(x, y docker.Container) int {
			return cmp.Compare(parseDockerSize(x.Memory), parseDockerSize(y.Memory))
		},
	})
}

func parseDockerPercent(raw string) float64 {
	s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(raw), "%"))
	if s == "" {
		return -1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return -1
	}
	return v
}

func parseDockerSize(raw string) float64 {
	s := strings.TrimSpace(raw)
	if s == "" {
		return -1
	}
	i := 0
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
		i++
	}
	if i == 0 {
		return -1
	}
	v, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		return -1
	}
	unit := strings.ToLower(strings.TrimSpace(s[i:]))
	switch unit {
	case "b":
		return v
	case "kb", "k":
		return v * 1e3
	case "kib":
		return v * 1024
	case "mb", "m":
		return v * 1e6
	case "mib":
		return v * 1024 * 1024
	case "gb", "g":
		return v * 1e9
	case "gib":
		return v * 1024 * 1024 * 1024
	case "tb", "t":
		return v * 1e12
	case "tib":
		return v * 1024 * 1024 * 1024 * 1024
	default:
		return v
	}
}

func dockerContainerIDs(items []docker.Container) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func (a *App) dockerGroupRows(c *ui.Context, g dockerGroup, tracks []ui.Track, compact bool) {
	t := c.Theme()
	pal := Colors(t)
	s := &a.engine.containers
	open := s.groupOpen(g)
	menu := func(m *ui.Menu) { a.dockerGroupMenu(m, g) }
	ui.Column(c).Children(func() {
		line := workloadSelectionRow(c.Key("group-row"), pal.Line)
		line.Children(func() {
			ui.Column(c).Width(18).Shrink(0).Justify(ui.Center).Children(func() {
				s.bulk.checkbox(c, "docker-select-group", locales.KubeBulkSelect(g.name), dockerContainerIDs(g.items))
			})
			row := ui.ButtonBase(c.Key("toggle")).Grow(1).MinWidth(0).MinHeight(40).Gap(8).AlignItems(ui.Center).Justify(ui.Start).
				Label(g.name).Disabled(len(g.items) == 0).OnClick(func() { s.setGroup(g.key, !open) })
			row.ContextMenu(menu)
			bg := pal.Subtle
			if row.Hovered() && len(g.items) > 0 {
				bg = pal.Muted
			}
			line.Background(line.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
			row.Children(func() {
				turn := float32(0)
				if open && len(g.items) > 0 {
					turn = 90
				}
				chevron := ui.Box(c).Size(14, 14).Shrink(0).Center()
				chevron.Rotate(chevron.AnimateTo("turn", turn, ui.Animation{Duration: 160 * time.Millisecond})).Children(func() {
					ui.Icon(c, Icon("chevron-right")).Size(14, 14).TextColor(pal.TextFaint)
				})
				if g.kind != "" {
					aiTag(c, g.kind, t.Background, t.TextMuted).Shrink(0).BorderWidth(1).BorderColor(pal.Line)
				}
				clipped(c, g.name).FontSize(13).FontWeight(600).MinWidth(0)
				switch {
				case g.problems > 0:
					aiTag(c, locales.DockerGroupProblems(g.problems), pal.DangerSoft, pal.DangerText).Shrink(0)
				case g.running > 0:
					aiTag(c, locales.DockerGroupRunning(g.running), pal.SuccessSoft, pal.SuccessText).Shrink(0)
				default:
					aiTag(c, locales.DockerContainersStopped(), pal.Muted, t.TextMuted).Shrink(0)
				}
			})
			ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
				if g.dir != "" && !narrow(c) && !compact {
					link := ui.ButtonBase(c.Key("project-link")).Height(24).MaxWidth(205).MinWidth(0).Padding(0, 6).
						AlignItems(ui.Center).Radius(5).Label(locales.OpenProject()).Tooltip(g.dir).
						OnClick(func() { a.router.Push(projectPath(g.dir)) })
					if link.Hovered() {
						link.Background(pal.Muted)
					}
					link.Children(func() {
						clipped(c, tilde(g.dir, a.Home)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).MinWidth(0)
					})
				}
				if g.compose.Files != "" && a.canActOnDockerContainers() {
					if g.compose.Name == "" {
						iconButton(c.Key("compose-up"), "play", ops.DockerUp.Label()+"…").
							OnClick(func() { a.planDockerCompose(g.compose, ops.DockerUp) })
					} else {
						if a.StreamDockerLogs != nil && len(g.items) > 0 {
							iconButton(c.Key("compose-logs"), "scroll-text", locales.DockerComposeLogs()).
								OnClick(func() { a.openDockerComposeLogs(g) })
						}
						if g.running > 0 {
							iconButton(c.Key("compose-stop"), "square", ops.DockerStop.Label()+"…").
								OnClick(func() { a.planDockerCompose(g.compose, ops.DockerStop) })
						} else {
							action := ops.DockerStart
							if g.dir != "" {
								action = ops.DockerUp
							}
							iconButton(c.Key("compose-start"), "play", action.Label()+"…").
								OnClick(func() { a.planDockerCompose(g.compose, action) })
						}
					}
				}
				if g.compose.Name != "" || g.dir != "" {
					iconButton(c.Key("group-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
				} else {
					ui.Box(c).Width(24)
				}
			})
		})
		if open && len(g.items) > 0 {
			ui.Column(c.Key("group-items")).Transition(rowMotion).Children(func() {
				for _, item := range g.items {
					a.dockerContainerRow(c.Key(item.ID), item, tracks, compact)
				}
			})
		}
	})
}

func (a *App) openDockerComposeLogs(g dockerGroup) {
	var targets []docker.LogTarget
	for _, item := range a.engine.containers.items {
		if item.Kind == "" && item.Compose.Name == g.compose.Name && item.Compose.Dir == g.compose.Dir {
			targets = append(targets, docker.LogTarget{ID: item.ID, Name: item.Name})
		}
	}
	a.openDockerLogs(g.name, targets, true)
}

func (a *App) openDockerShell(item docker.Container) {
	s := &a.engine.containers
	if a.OpenDockerShell == nil || item.State.Status != "running" || s.detail.shellRequest != nil || s.err != nil {
		return
	}
	request, target := new(byte), s.target
	s.detail.shellRequest = request
	a.OpenDockerShell(target, item.ID, func(err error) {
		if s.detail.shellRequest != request {
			return
		}
		s.detail.shellRequest = nil
		if err != nil && a.isWatchingDockerContainer(target) {
			a.planErr = err.Error()
		}
	})
}

func (a *App) dockerContainerRow(c *ui.Context, item docker.Container, tracks []ui.Track, compact bool) {
	t := c.Theme()
	pal := Colors(t)
	s := &a.engine.containers
	status, bg, fg := dockerContainerStatus(c, item)
	menu := func(m *ui.Menu) {
		if m.Item(locales.Details()).Chosen() {
			s.selectContainer(item.ID)
		}
		if a.StreamDockerLogs != nil && m.Item(locales.KubeLogsOpen()).Chosen() {
			s.selectContainer(item.ID)
			a.openDockerLogs(item.Name, []docker.LogTarget{{ID: item.ID}}, false)
		}
		if a.OpenDockerShell != nil && item.State.Status == "running" && m.Item(locales.KubeShellAction()).Chosen() {
			a.openDockerShell(item)
		}
		if m.Item(locales.KubeCopyName()).Chosen() {
			copyName(c, item.Name)
		}
		if a.canActOnDockerContainers() {
			m.Separator()
			for _, action := range dockerContainerActions {
				if action.Applies(item) && m.Item(action.Label()+"…").Chosen() {
					a.planDockerContainers([]docker.Container{item}, action)
				}
			}
		}
	}
	row := ui.Column(c).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).ContextMenu(menu)
	rowBg := t.Background.Alpha(0)
	switch {
	case item.ID == s.detail.id:
		rowBg = pal.AccentSoft
	case item.Problem():
		rowBg = bg.Alpha(0.10)
	case row.Hovered():
		rowBg = pal.Subtle
	}
	row.Background(row.AnimateColor("bg", rowBg, ui.Animation{Duration: 120 * time.Millisecond}))
	dot := pal.TextFaint.Alpha(0.45)
	switch {
	case item.Problem() && item.State.Status != "restarting":
		dot = t.Danger
	case item.State.Status == "restarting" || item.State.Status == "paused":
		dot = t.Warning
	case item.State.Status == "running":
		dot = t.Success
	}
	var issue *rowIssue
	if why := dockerContainerProblem(item); why != "" {
		sev := issueDanger
		if item.State.Status == "restarting" {
			sev = issueWarning
		}
		issue = &rowIssue{
			key:      "docker-container:" + item.ID,
			label:    locales.Details() + " · " + item.Name,
			severity: sev,
			title:    status,
			body:     why,
		}
		if port := item.ConflictPort(); port > 0 {
			issue.actionLabel = locales.DockerOpenPorts()
			issue.onAction = func() {
				a.portsFilter = strconv.Itoa(port)
				a.router.Push(portsPage)
			}
		}
	}
	row.Children(func() {
		resourceGrid(ui.Box(c), tracks).Padding(6, 16).MinHeight(44).Children(func() {
			s.bulk.checkbox(c, "docker-select-container", locales.KubeBulkSelect(item.Name), []string{item.ID})
			ui.ButtonBase(c.Key("docker-open-container")).Padding(0, 0, 0, 10).MinWidth(0).MinHeight(32).Gap(10).
				AlignItems(ui.Center).Justify(ui.Start).Label(item.Name).Tooltip(item.Name).
				OnClick(func() { s.selectContainer(item.ID) }).Children(func() {
				dockerTechTile(c, item.Image, 28, &dot)
				ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
					clipped(c, item.Name).FontSize(12.5).Font(FontMono)
					if !compact && item.Image != "" {
						clipped(c, item.Image).FontSize(10.5).TextColor(pal.TextFaint)
					}
				})
			})
			a.workloadStatusIssue(c, status, bg, fg, issue)
			if compact {
				ui.Row(c).Gap(2).AlignItems(ui.Center).Justify(ui.End).Children(func() {
					a.dockerContainerQuickPower(c, item)
					iconButton(c.Key("container-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
				})
				return
			}
			port := dockerPortLabel(item.Ports)
			if item.ConflictPort() > 0 {
				port = strconv.Itoa(item.ConflictPort())
			}
			uptime := ""
			if item.Active() && !item.State.StartedAt.IsZero() {
				uptime = age(c, item.State.StartedAt)
			}
			clipped(c, cmp.Or(port, "—")).Font(FontMono).FontSize(11.5).MinWidth(0).Tooltip(cmp.Or(item.Ports, port))
			for _, value := range []string{uptime, item.CPU, item.Memory} {
				clipped(c, cmp.Or(value, "—")).Font(FontMono).FontSize(11.5).MinWidth(0)
			}
			ui.Row(c).Gap(2).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				a.dockerContainerQuickPower(c, item)
				if a.StreamDockerLogs != nil {
					iconButton(c.Key("container-logs"), "scroll-text", locales.KubeLogsOpen()).OnClick(func() {
						s.selectContainer(item.ID)
						a.openDockerLogs(item.Name, []docker.LogTarget{{ID: item.ID}}, false)
					})
				}
				if !narrow(c) && a.OpenDockerShell != nil && item.State.Status == "running" {
					iconButton(c.Key("container-shell"), "terminal", locales.KubeShellAction()).
						Disabled(s.detail.shellRequest != nil || s.err != nil).
						OnClick(func() { a.openDockerShell(item) })
				}
				iconButton(c.Key("container-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
			})
		})
	})
}

func (a *App) dockerContainerQuickPower(c *ui.Context, item docker.Container) {
	action, icon := ops.DockerStart, "play"
	switch {
	case item.State.Status == "paused":
		action, icon = ops.DockerUnpause, "play"
	case item.Active():
		action, icon = ops.DockerStop, "square"
	}
	if !action.Applies(item) || !a.canActOnDockerContainers() {
		return
	}
	iconButton(c.Key("container-power"), icon, action.Label()+"…").
		OnClick(func() { a.planDockerContainers([]docker.Container{item}, action) })
}

// dockerPortLabel fits published mappings in the table. Docker may list
// the same mapping once for IPv4 and once for IPv6.
func dockerPortLabel(raw string) string {
	var mappings []string
	for item := range strings.SplitSeq(raw, ",") {
		host, container, ok := strings.Cut(strings.TrimSpace(item), "->")
		if !ok {
			continue
		}
		if i := strings.LastIndexByte(host, ':'); i >= 0 {
			host = host[i+1:]
		}
		mapping := host + " → " + strings.TrimSuffix(container, "/tcp")
		if !slices.Contains(mappings, mapping) {
			mappings = append(mappings, mapping)
		}
	}
	if len(mappings) == 0 {
		return raw
	}
	return strings.Join(mappings, ", ")
}

func dockerContainerStatus(c *ui.Context, item docker.Container) (string, ui.Color, ui.Color) {
	t := c.Theme()
	p := Colors(t)
	switch item.State.Status {
	case "restarting":
		return locales.DockerContainerRestarting(item.RestartCount), p.WarningSoft, p.WarningText
	case "running":
		if !item.Problem() {
			return locales.DockerContainerRunning(), p.SuccessSoft, p.SuccessText
		}
	case "paused":
		return locales.DockerContainerPaused(), p.WarningSoft, p.WarningText
	case "exited":
		if item.ExitedError() {
			return locales.DockerContainerExit(item.State.ExitCode), p.DangerSoft, p.DangerText
		}
	case "created":
		if !item.Problem() {
			return locales.DockerContainerCreated(), p.Muted, t.TextMuted
		}
	}
	if item.Problem() {
		return locales.DockerContainerIssue(), p.DangerSoft, p.DangerText
	}
	return locales.DockerContainerStopped(), p.Muted, t.TextMuted
}

func dockerContainerProblem(item docker.Container) string {
	var reasons []string
	switch {
	case item.ConflictPort() > 0:
		reasons = append(reasons, locales.DockerPortConflict(strconv.Itoa(item.ConflictPort())))
	case item.State.Error != "":
		reasons = append(reasons, item.State.Error)
	}
	switch {
	case item.State.OOMKilled:
		reasons = append(reasons, locales.DockerContainerOOM())
	case (item.State.Status == "exited" || item.State.Status == "restarting") && item.ExitedError():
		switch item.State.ExitCode {
		case 137:
			reasons = append(reasons, locales.DockerContainerExit137())
		case 127:
			reasons = append(reasons, locales.DockerContainerExit127())
		case 126:
			reasons = append(reasons, locales.DockerContainerExit126())
		case 1:
			reasons = append(reasons, locales.DockerContainerExit1())
		default:
			reasons = append(reasons, locales.DockerContainerExit(item.State.ExitCode))
		}
	}
	if item.State.Status == "restarting" {
		reasons = append(reasons, locales.DockerRestartLoop(item.RestartCount))
	}
	if item.State.Status == "running" && item.State.Health != nil && item.State.Health.Status == "unhealthy" {
		reasons = append(reasons, locales.DockerContainerUnhealthy())
	}
	return strings.Join(reasons, " · ")
}

var dockerContainerActions = []ops.DockerAction{ops.DockerStart, ops.DockerStop, ops.DockerPause, ops.DockerUnpause, ops.DockerRestart, ops.DockerRemove}

var dockerComposeActions = []ops.DockerAction{ops.DockerStart, ops.DockerStop, ops.DockerRestart, ops.DockerRemove}

func (a *App) canPlanDocker() bool { return a.BuildPlan != nil && !a.op.running() }

func (a *App) canActOnDockerContainers() bool {
	s := &a.engine.containers
	return a.canPlanDocker() && !s.planning && s.err == nil && a.engine.err == nil
}

func (a *App) dockerBulkBar(c *ui.Context, s *dockerContainersState) {
	var selected []docker.Container
	for _, item := range s.items {
		if s.bulk.selected[item.ID] {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return
	}
	floatingBulkBar(c, "docker-bulk-bar", locales.DockerSelected(len(selected)), s.bulk.clear, func(fg, dangerFg, dangerBg ui.Color) {
		type bulkSpec struct {
			action ops.DockerAction
			icon   string
		}
		specs := []bulkSpec{
			{ops.DockerStart, "play"},
			{ops.DockerStop, "square"},
		}
		if slices.ContainsFunc(selected, ops.DockerPause.Applies) {
			specs = append(specs, bulkSpec{ops.DockerPause, "pause"})
		}
		if slices.ContainsFunc(selected, ops.DockerUnpause.Applies) {
			specs = append(specs, bulkSpec{ops.DockerUnpause, "play"})
		}
		specs = append(specs, bulkSpec{ops.DockerRestart, "refresh-cw"}, bulkSpec{ops.DockerRemove, "trash-2"})
		for _, spec := range specs {
			action := spec.action
			disabled := !a.canActOnDockerContainers() || slices.ContainsFunc(selected, func(item docker.Container) bool { return !action.Applies(item) })
			afg, abg := fg, ui.Transparent
			if action == ops.DockerRemove {
				afg, abg = dangerFg, dangerBg
			}
			bulkAction(c.Key(string(action)), spec.icon, action.Label()+"…", afg, abg, disabled, func() {
				a.planDockerContainers(selected, action)
			})
		}
	})
}

func (a *App) planDockerContainers(items []docker.Container, action ops.DockerAction) {
	if !a.canActOnDockerContainers() {
		return
	}
	target, issue := a.engineTarget(a.router.Query("context"))
	if issue != "" {
		return
	}
	a.buildDockerPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerContainers(target, env, items, action)
	})
}

func (a *App) dockerGroupMenu(m *ui.Menu, g dockerGroup) {
	if a.StreamDockerLogs != nil && g.compose.Name != "" && len(g.items) > 0 && m.Item(locales.DockerComposeLogs()).Chosen() {
		a.openDockerComposeLogs(g)
	}
	if g.dir != "" && m.Item(locales.OpenProject()).Chosen() {
		a.router.Push(projectPath(g.dir))
	}
	if !a.canActOnDockerContainers() || g.compose.Files == "" {
		return
	}
	actions := dockerComposeActions
	if g.dir != "" {
		actions = append(slices.Clone(actions), ops.DockerUp)
		if g.compose.Name != "" {
			actions = append(actions, ops.DockerDown)
		} else {
			actions = []ops.DockerAction{ops.DockerUp}
		}
	}
	for _, action := range actions {
		if m.Item(action.Label() + "…").Chosen() {
			a.planDockerCompose(g.compose, action)
		}
	}
}

func (a *App) dockerComposeButton(c *ui.Context, project docker.ComposeProject, action ops.DockerAction) {
	ui.Button(c, action.Label()+"…").Height(24).OnClick(func() { a.planDockerCompose(project, action) })
}

func (a *App) planDockerCompose(project docker.ComposeProject, action ops.DockerAction) {
	if !a.canPlanDocker() || a.engine.containers.planning {
		return
	}
	name := a.router.Query("context")
	if a.router.Path() != dockerEnginePage {
		st, _ := a.toolState("docker")
		if st.Docker == nil || !a.dockerAnswers() {
			return
		}
		name = st.Docker.Selected
	}
	target, issue := a.engineTarget(name)
	if issue != "" {
		return
	}
	a.buildDockerPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerCompose(context.Background(), run.Exec{}, env, target, project, action)
	})
}

func (a *App) buildDockerPlan(build func(sysenv.Env, ops.Terminal) (ops.Plan, error)) {
	s := &a.engine.containers
	path, name, dir := a.router.Path(), a.router.Query("context"), a.router.Query("dir")
	s.planning = true
	revision := s.revision
	a.BuildPlan(build, func(p ops.Plan, err error) {
		if s.revision != revision {
			return
		}
		s.planning = false
		if a.router.Path() == path && a.router.Query("context") == name && a.router.Query("dir") == dir && a.canPlanDocker() {
			a.proposeOrSay(p, err)
		}
	})
}

func (a *App) projectDockerCard(c *ui.Context, p projects.Project) {
	project, ok := composeProject(p)
	if !ok || !a.canPlanDocker() || !a.dockerAnswers() {
		return
	}
	listCard(c, locales.DockerComposeTitle(), "", func() {
		ui.Row(c).Gap(8).Padding(10, 12).Wrap().Children(func() {
			a.dockerComposeButton(c.Key("project-compose-up"), project, ops.DockerUp)
			a.dockerComposeButton(c.Key("project-compose-down"), project, ops.DockerDown)
		})
	})
}
