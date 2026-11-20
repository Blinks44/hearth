package ui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type dockerResourcesState struct {
	target           docker.Target
	kind             docker.ResourceKind
	watching         string
	revision         int
	stop             func()
	items            []docker.Resource
	err, sizeErr     error
	loaded, planning bool
	filter           [2]int
	query            [2]string
	collapsed        [2]bool
}

func (s *dockerResourcesState) stopWatch() {
	if s.stop != nil {
		s.stop()
	}
	s.watching, s.stop = "", nil
	s.revision++
	s.planning = false
}

func (a *App) syncDockerResources(target docker.Target, key string) {
	s := &a.engine.resources
	var kind docker.ResourceKind
	switch a.engine.view {
	case dockerVolumes:
		kind = docker.VolumeResource
	case dockerNetworks:
		kind = docker.NetworkResource
	}
	if kind == "" || a.WatchDockerResources == nil {
		key = ""
	}
	if key == s.watching && (key == "" || kind == s.kind) {
		return
	}
	s.stopWatch()
	if key == "" || s.target != target || s.kind != kind {
		s.items, s.err, s.sizeErr, s.loaded = nil, nil, nil, false
	}
	if key == "" {
		return
	}
	s.target, s.kind, s.watching = target, kind, key
	revision := s.revision
	s.stop = a.WatchDockerResources(target, kind, func(items []docker.Resource, err, sizeErr error) {
		if s.revision != revision {
			return
		}
		s.err, s.sizeErr = err, sizeErr
		if err == nil {
			s.items, s.loaded = items, true
		}
	})
}

const (
	dockerResourceAll = iota
	dockerResourceUsed
	dockerResourceUnused
)

func dockerResourceStatus(kind docker.ResourceKind, item docker.Resource) int {
	if kind == docker.NetworkResource && item.SystemNetwork() {
		return dockerResourceAll
	}
	if len(item.Users) > 0 {
		return dockerResourceUsed
	}
	return dockerResourceUnused
}

func matchesDockerResource(item docker.Resource, query string) bool {
	return strings.Contains(strings.ToLower(item.Name+" "+item.Project+" "+strings.Join(item.Subnets, " ")), query) ||
		slices.ContainsFunc(item.Users, func(user docker.ResourceUser) bool {
			return strings.Contains(strings.ToLower(user.Name+" "+user.Project), query)
		})
}

func (a *App) dockerResourcesView(c *ui.Context) {
	s := &a.engine.resources
	index := a.engine.view - dockerVolumes
	kind := s.kind
	reading, empty := locales.DockerVolumesReading(), locales.DockerVolumesEmpty()
	search, name, unused := locales.DockerVolumeSearch(), locales.DockerVolumesLocal(), locales.DockerImagesDangling()
	column, hint := locales.DockerImageSize(), locales.DockerVolumesHint()
	if index == 1 {
		reading, empty = locales.DockerNetworksReading(), locales.DockerNetworksEmpty()
		search, name, unused = locales.DockerNetworkSearch(), locales.DockerViewNetworks(), locales.DockerNetworksUnused()
		column, hint = locales.DockerNetworkDriver(), locales.DockerNetworksHint()
	}
	if !s.loaded {
		if s.err != nil {
			toolNote(c, s.err.Error())
		} else {
			tableSkeleton(c, reading)
		}
		return
	}
	if s.err != nil {
		toolNote(c, locales.KubeClusterStale(s.err.Error()))
	}
	if s.sizeErr != nil {
		toolNote(c, locales.DockerVolumeSizeUnavailable(s.sizeErr.Error()))
	}
	var counts [3]int
	var items []docker.Resource
	var total int64
	allSizes := true
	query := strings.ToLower(strings.TrimSpace(s.query[index]))
	for _, item := range s.items {
		status := dockerResourceStatus(kind, item)
		if status != dockerResourceAll {
			counts[status]++
		}
		if (s.filter[index] == dockerResourceAll || status == s.filter[index]) && matchesDockerResource(item, query) {
			items = append(items, item)
			if item.Size >= 0 {
				total += item.Size
			} else {
				allSizes = false
			}
		}
	}
	counts[dockerResourceAll] = len(s.items)
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Shrink(0).Children(func() {
		segments(c, "docker-resource-filter", &s.filter[index], []segment{
			{locales.TabAll(), strconv.Itoa(counts[dockerResourceAll])},
			{locales.DockerImagesUsed(), strconv.Itoa(counts[dockerResourceUsed])},
			{unused, strconv.Itoa(counts[dockerResourceUnused])},
		})
		ui.Box(c).Grow(1)
		width := float32(270)
		if narrow(c) {
			width = 220
		}
		ui.SearchField(c.Key("docker-resource-search"), &s.query[index]).Label(search).Placeholder(search).
			Width(width).MinWidth(110).Shrink(1)
	})
	tracks := []ui.Track{ui.Fr(1), ui.Fixed(162), ui.Fixed(226), ui.Fixed(63), ui.Fixed(89), ui.Fixed(24)}
	if narrow(c) {
		tracks = []ui.Track{ui.Fr(1), ui.Fixed(116), ui.Fixed(130), ui.Fixed(64), ui.Fixed(54), ui.Fixed(24)}
	}
	summary := strconv.Itoa(len(items))
	if kind == docker.VolumeResource && allSizes && len(items) > 0 {
		summary += " · " + formatBytes(total)
	}
	pal := Colors(c.Theme())
	workloadPanes(c.Key("docker-resource-panes-"+string(kind)), func() {
		resourceHead(c, tracks, locales.KubeClusterName(), locales.KubeClusterStatus(), locales.DockerImageUsers(),
			column, locales.DockerContainerCreated(), "")
	}, func() {
		if len(items) == 0 {
			ui.Text(c, empty).FontSize(13).TextColor(c.Theme().TextMuted).Padding(12, 16).Wrap()
			return
		}
		open := !s.collapsed[index] || query != ""
		ui.Row(c).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).
			AlignItems(ui.Center).Gap(8).Padding(0, 16).Children(func() {
			ui.ButtonBase(c.Key("docker-resource-group")).MinHeight(40).Grow(1).MinWidth(0).Gap(8).
				AlignItems(ui.Center).Justify(ui.Start).Label(name).Disabled(query != "").
				OnClick(func() { s.collapsed[index] = open }).Children(func() {
				icon := "chevron-right"
				if open {
					icon = "chevron-down"
				}
				ui.Icon(c, Icon(icon)).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
				clipped(c, name).FontSize(13).FontWeight(600).MinWidth(0)
				aiTag(c, summary, pal.Muted, c.Theme().TextMuted).Shrink(0)
			})
			if !narrow(c) {
				clipped(c, hint).FontSize(11).TextColor(pal.TextFaint).Tooltip(hint)
			}
		})
		if open {
			for _, item := range items {
				a.dockerResourceRow(c.Key(item.ID), item, tracks)
			}
		}
	}, "", nil)
}

func (a *App) dockerResourceRow(c *ui.Context, item docker.Resource, tracks []ui.Track) {
	kind := a.engine.resources.kind
	pal, theme := Colors(c.Theme()), c.Theme()
	var users []string
	for _, user := range item.Users {
		label := user.Name
		if user.Project != "" && user.Project != item.Project {
			label += " · " + user.Project
		}
		users = append(users, label)
	}
	names := strings.Join(users, ", ")
	if names == "" {
		names = "–"
	}
	status, bg, fg := locales.KubeStorageUnused(), pal.Muted, theme.TextMuted
	system := kind == docker.NetworkResource && item.SystemNetwork()
	switch {
	case system:
		status = locales.DockerNetworkSystem()
	case slices.ContainsFunc(item.Users, func(user docker.ResourceUser) bool { return user.Active }):
		status, bg, fg = locales.DockerImagesUsed(), pal.SuccessSoft, pal.SuccessText
	case len(item.Users) > 0:
		status = locales.DockerImageStoppedUser()
	}
	metric := item.Driver
	if kind == docker.VolumeResource {
		metric = "–"
		if item.Size >= 0 {
			metric = formatBytes(item.Size)
		}
	}
	detail := strings.Join(item.Subnets, ", ")
	if item.Project != "" {
		detail = strings.Trim(strings.Join([]string{item.Project, detail}, " · "), " ·")
	}
	remove := locales.DockerContainerRemove() + "…"
	menu := func(m *ui.Menu) {
		if m.Item(locales.KubeCopyName()).Chosen() {
			copyName(c, item.Name)
		}
		if !system && a.canActOnDockerResources() && m.Item(remove).Chosen() {
			a.planDockerResourceRemoval(item)
		}
	}
	ui.Column(c).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).ContextMenu(menu).Children(func() {
		resourceGrid(ui.Box(c), tracks).Padding(6, 16).MinHeight(39).Children(func() {
			ui.Column(c).Gap(1).MinWidth(0).Children(func() {
				clipped(c, item.Name).Font(FontMono).FontSize(12.5).Tooltip(item.Name)
				if detail != "" {
					clipped(c, detail).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).Tooltip(detail)
				}
			})
			workloadStatus(c, status, bg, fg)
			clipped(c, names).Font(FontMono).FontSize(11.5).MinWidth(0).Tooltip(names)
			clipped(c, metric).FontSize(11.5).Tooltip(metric)
			created, date := "–", ""
			if !item.Created.IsZero() {
				created, date = age(c, item.Created), item.Created.Local().Format(time.DateTime)
			}
			clipped(c, created).FontSize(11.5).Tooltip(date)
			iconButton(c.Key("docker-resource-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
		})
		if !system && len(item.Users) == 0 {
			hint := locales.DockerVolumeUnusedHint()
			if kind == docker.NetworkResource {
				hint = locales.DockerNetworkUnusedHint()
			}
			ui.Row(c).Gap(7).AlignItems(ui.Center).Padding(0, 16, 8, 16).Children(func() {
				ui.Icon(c, Icon("info")).Size(13, 13).Shrink(0).TextColor(theme.TextMuted)
				ui.Text(c, hint).FontSize(11.5).TextColor(theme.TextMuted).Wrap().MinWidth(0)
				ui.Button(c.Key("docker-resource-remove"), remove).Height(24).Shrink(0).
					Disabled(!a.canActOnDockerResources()).OnClick(func() { a.planDockerResourceRemoval(item) })
			})
		}
	})
}

func (a *App) canActOnDockerResources() bool {
	s := &a.engine.resources
	return a.canPlanDocker() && s.watching != "" && !s.planning && s.err == nil && a.engine.err == nil
}

func (a *App) planDockerResourceRemoval(item docker.Resource) {
	if !a.canActOnDockerResources() {
		return
	}
	s := &a.engine.resources
	target, kind, revision, view := s.target, s.kind, s.revision, a.engine.view
	s.planning = true
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerResourceRemoval(context.Background(), run.Exec{}, target, env, kind, item)
	}, func(p ops.Plan, err error) {
		if revision != s.revision {
			return
		}
		s.planning = false
		if a.canActOnDockerResources() && a.router.Path() == dockerEnginePage &&
			a.router.Query("context") == target.Context && a.engine.view == view {
			a.proposeOrSay(p, err)
		}
	})
}
