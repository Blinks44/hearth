package ui

import (
	"cmp"
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

type dockerResourceSlot struct {
	items        []docker.Resource
	err, sizeErr error
	loaded       bool
	filter       int
	query        string
	collapsed    bool
	sort         tableSort
	bulk         resourceSelection
}

type dockerResourcesState struct {
	target   docker.Target
	kind     docker.ResourceKind
	watching string
	revision int
	stop     func()
	planning bool
	slots    [2]dockerResourceSlot
}

func dockerResourceSlotIndex(kind docker.ResourceKind) int {
	if kind == docker.NetworkResource {
		return 1
	}
	return 0
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
	if target != (docker.Target{}) && s.target != target {
		for i := range s.slots {
			s.slots[i].items, s.slots[i].err, s.slots[i].sizeErr, s.slots[i].loaded = nil, nil, nil, false
			s.slots[i].bulk.clear()
		}
	} else if key == "" || s.kind != kind {
		s.slots[0].bulk.clear()
		s.slots[1].bulk.clear()
	}
	if key == "" {
		return
	}
	s.target, s.kind, s.watching = target, kind, key
	index := dockerResourceSlotIndex(kind)
	revision := s.revision
	s.stop = a.WatchDockerResources(target, kind, func(items []docker.Resource, err, sizeErr error) {
		if s.revision != revision {
			return
		}
		slot := &s.slots[index]
		slot.err, slot.sizeErr = err, sizeErr
		if err == nil {
			slot.items, slot.loaded = items, true
			for id := range slot.bulk.selected {
				if !slices.ContainsFunc(items, func(item docker.Resource) bool { return item.ID == id }) {
					delete(slot.bulk.selected, id)
				}
			}
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
	return strings.Contains(strings.ToLower(item.Name+" "+item.Project+" "+item.Driver+" "+strings.Join(item.Subnets, " ")), query) ||
		slices.ContainsFunc(item.Users, func(user docker.ResourceUser) bool {
			return strings.Contains(strings.ToLower(user.Name+" "+user.Project), query)
		})
}

func (a *App) dockerResourcesView(c *ui.Context) {
	s := &a.engine.resources
	index := a.engine.view - dockerVolumes
	slot := &s.slots[index]
	kind := docker.VolumeResource
	if index == 1 {
		kind = docker.NetworkResource
	}
	reading, empty := locales.DockerVolumesReading(), locales.DockerVolumesEmpty()
	search, name, unused := locales.DockerVolumeSearch(), locales.DockerVolumesLocal(), locales.DockerNetworksUnused()
	column, hint := locales.DockerImageSize(), locales.DockerVolumesHint()
	if index == 1 {
		reading, empty = locales.DockerNetworksReading(), locales.DockerNetworksEmpty()
		search, name, unused = locales.DockerNetworkSearch(), locales.DockerViewNetworks(), locales.DockerNetworksUnused()
		column, hint = locales.DockerNetworkDriver(), locales.DockerNetworksHint()
	}
	if !slot.loaded {
		if slot.err != nil {
			toolNote(c, slot.err.Error())
		} else {
			tableSkeleton(c, reading)
		}
		return
	}
	if slot.err != nil {
		toolNote(c, locales.KubeClusterStale(slot.err.Error()))
	}
	if slot.sizeErr != nil {
		toolNote(c, locales.DockerVolumeSizeUnavailable(slot.sizeErr.Error()))
	}
	var counts [3]int
	var items []docker.Resource
	var selectableIDs []string
	var total int64
	allSizes := true
	query := strings.ToLower(strings.TrimSpace(slot.query))
	for _, item := range slot.items {
		status := dockerResourceStatus(kind, item)
		if status != dockerResourceAll {
			counts[status]++
		}
		if (slot.filter == dockerResourceAll || status == slot.filter) && matchesDockerResource(item, query) {
			items = append(items, item)
			if kind != docker.NetworkResource || !item.SystemNetwork() {
				selectableIDs = append(selectableIDs, item.ID)
			}
			if item.Size >= 0 {
				total += item.Size
			} else {
				allSizes = false
			}
		}
	}
	sortDockerResources(slot.sort, items, kind)
	counts[dockerResourceAll] = len(slot.items)
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Shrink(0).Children(func() {
		segments(c, "docker-resource-filter", &slot.filter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[dockerResourceAll])},
			{locales.DockerImagesUsed(), strconv.Itoa(counts[dockerResourceUsed])},
			{unused, strconv.Itoa(counts[dockerResourceUnused])},
		})
		ui.Box(c).Grow(1)
		width := float32(270)
		if narrow(c) {
			width = 220
		}
		ui.SearchField(c.Key("docker-resource-search"), &slot.query).Label(search).Placeholder(search).
			Height(controlHeight).Width(width).MinWidth(110).Shrink(1)
	})
	tracks := []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(152), ui.Fixed(200), ui.Fixed(70), ui.Fixed(86), ui.Fixed(54)}
	if narrow(c) {
		tracks = []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(132), ui.Fixed(94), ui.Fixed(64), ui.Fixed(54), ui.Fixed(54)}
	}
	summary := strconv.Itoa(len(items))
	if kind == docker.VolumeResource && allSizes && len(items) > 0 {
		summary += " · " + formatBytes(total)
	}
	pal := Colors(c.Theme())
	hasBulk := len(slot.bulk.selected) > 0
	workloadPanes(c.Key("docker-resource-panes-"+string(kind)), func() {
		sortHead(c, tracks, func() {
			slot.bulk.checkbox(c, "resource-select-all", locales.KubeBulkSelectAll(), selectableIDs)
		}, &slot.sort,
			sortColumn{locales.KubeClusterName(), "name"},
			sortColumn{locales.KubeClusterStatus(), "status"},
			sortColumn{locales.DockerImageUsers(), "users"},
			sortColumn{column, "metric"},
			sortColumn{locales.DockerContainerCreated(), "created"},
			sortColumn{"", ""},
		)
	}, func() {
		if len(items) == 0 {
			ui.Text(c, empty).FontSize(13).TextColor(c.Theme().TextMuted).Padding(12, 16).Wrap()
			return
		}
		open := !slot.collapsed || query != ""
		line := workloadSelectionRow(c.Key("docker-resource-group-row"), pal.Line).AlignItems(ui.Center)
		line.Children(func() {
			ui.Column(c).Width(18).Shrink(0).Children(func() {
				slot.bulk.checkbox(c, "resource-select-group", locales.KubeBulkSelect(name), selectableIDs)
			})
			btn := ui.ButtonBase(c.Key("docker-resource-group")).MinHeight(40).Grow(1).MinWidth(0).Gap(8).
				AlignItems(ui.Center).Justify(ui.Start).Label(name).Disabled(query != "").
				OnClick(func() { slot.collapsed = open })
			bg := pal.Subtle
			if btn.Hovered() && query == "" {
				bg = pal.Muted
			}
			line.Background(line.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
			btn.Children(func() {
				turn := float32(0)
				if open {
					turn = 90
				}
				chevron := ui.Box(c).Size(14, 14).Shrink(0).Center()
				chevron.Rotate(chevron.AnimateTo("turn", turn, ui.Animation{Duration: 160 * time.Millisecond})).Children(func() {
					ui.Icon(c, Icon("chevron-right")).Size(14, 14).TextColor(pal.TextFaint)
				})
				clipped(c, name).FontSize(13).FontWeight(600).MinWidth(0)
				aiTag(c, summary, pal.Muted, c.Theme().TextMuted).Shrink(0)
			})
			if !narrow(c) {
				clipped(c, hint).FontSize(11).TextColor(pal.TextFaint).Tooltip(hint)
			}
		})
		if open {
			ui.Column(c.Key("docker-resource-items")).Transition(rowMotion).Children(func() {
				for _, item := range items {
					a.dockerResourceRow(c.Key(item.ID), kind, slot, item, tracks)
				}
			})
		}
		if hasBulk {
			ui.Box(c).Height(56).Shrink(0)
		}
	}, "", nil)
	a.dockerResourceBulkBar(c, slot)
}

func sortDockerResources(s tableSort, items []docker.Resource, kind docker.ResourceKind) {
	statusRank := func(item docker.Resource) int {
		switch {
		case kind == docker.NetworkResource && item.SystemNetwork():
			return 0
		case slices.ContainsFunc(item.Users, func(user docker.ResourceUser) bool { return user.Active }):
			return 1
		case len(item.Users) > 0:
			return 2
		default:
			return 3
		}
	}
	sortRows(s, items, map[string]func(x, y docker.Resource) int{
		"name": func(x, y docker.Resource) int {
			return cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		},
		"status": func(x, y docker.Resource) int {
			if n := cmp.Compare(statusRank(x), statusRank(y)); n != 0 {
				return n
			}
			return cmp.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
		},
		"users": func(x, y docker.Resource) int {
			return cmp.Compare(len(x.Users), len(y.Users))
		},
		"metric": func(x, y docker.Resource) int {
			if kind == docker.VolumeResource {
				return cmp.Compare(x.Size, y.Size)
			}
			return cmp.Compare(strings.ToLower(x.Driver), strings.ToLower(y.Driver))
		},
		"created": func(x, y docker.Resource) int {
			return x.Created.Compare(y.Created)
		},
	})
}

func (a *App) dockerResourceRow(c *ui.Context, kind docker.ResourceKind, slot *dockerResourceSlot, item docker.Resource, tracks []ui.Track) {
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
		names = "—"
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
		metric = "—"
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
			a.planDockerResourcesRemoval([]docker.Resource{item})
		}
	}
	row := ui.Column(c).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).ContextMenu(menu)
	rowBg := theme.Background.Alpha(0)
	if row.Hovered() {
		rowBg = pal.Subtle
	}
	row.Background(row.AnimateColor("bg", rowBg, ui.Animation{Duration: 120 * time.Millisecond}))
	row.Children(func() {
		resourceGrid(ui.Box(c), tracks).Padding(6, 16).MinHeight(44).Children(func() {
			var boxIDs []string
			if !system {
				boxIDs = []string{item.ID}
			}
			slot.bulk.checkbox(c, "docker-select-resource", locales.KubeBulkSelect(item.Name), boxIDs)
			ui.Column(c).Gap(1).MinWidth(0).Children(func() {
				clipped(c, item.Name).Font(FontMono).FontSize(12.5).Tooltip(item.Name)
				if detail != "" {
					clipped(c, detail).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).Tooltip(detail)
				}
			})
			workloadStatus(c, status, bg, fg)
			a.dockerResourceUsedByCell(c, a.engine.resources.target.Context, item.Project, item.Users, names)
			clipped(c, metric).FontSize(11.5).Tooltip(metric)
			created, date := "—", ""
			if !item.Created.IsZero() {
				created, date = age(c, item.Created), item.Created.Local().Format(time.DateTime)
			}
			clipped(c, created).FontSize(11.5).Tooltip(date)
			ui.Row(c).Gap(2).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				if !system && len(item.Users) == 0 && a.canActOnDockerResources() {
					iconButton(c.Key("docker-resource-remove"), "trash-2", remove).
						OnClick(func() { a.planDockerResourcesRemoval([]docker.Resource{item}) })
				}
				iconButton(c.Key("docker-resource-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
			})
		})
		if !system && len(item.Users) == 0 {
			hint := locales.DockerVolumeUnusedHint()
			if kind == docker.NetworkResource {
				hint = locales.DockerNetworkUnusedHint()
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(0, 16, 7, 44).Children(func() {
				ui.Icon(c, Icon("info")).Size(12, 12).Shrink(0).TextColor(pal.TextFaint)
				ui.Text(c, hint).FontSize(11).TextColor(pal.TextFaint).Wrap().Grow(1).MinWidth(0)
			})
		}
	})
}

func (a *App) dockerResourceUsedByCell(c *ui.Context, contextName, itemProject string, users []docker.ResourceUser, tooltip string) {
	pal := Colors(c.Theme())
	if len(users) == 0 {
		clipped(c, "—").Font(FontMono).FontSize(11.5).MinWidth(0)
		return
	}
	if contextName == "" {
		clipped(c, tooltip).Font(FontMono).FontSize(11.5).MinWidth(0).Tooltip(tooltip)
		return
	}
	ui.Row(c).Gap(4).AlignItems(ui.Center).MinWidth(0).Clip().Tooltip(tooltip).Children(func() {
		for i, u := range users {
			user := u
			if i > 0 {
				ui.Text(c, ",").Font(FontMono).FontSize(11.5).TextColor(pal.TextFaint).SingleLine()
			}
			label := user.Name
			if user.Project != "" && user.Project != itemProject {
				label += " · " + user.Project
			}
			link := ui.ButtonBase(c.Key("res-user-"+strconv.Itoa(i)+"-"+user.Name)).Radius(4).Padding(1, 3).
				Label(label).Tooltip(label).
				OnClick(func() { a.requestDockerContainer(contextName, user.ID, user.Name, true, false) })
			if link.Hovered() {
				link.Background(pal.Subtle)
			}
			link.Children(func() {
				clipped(c, label).Font(FontMono).FontSize(11.5).TextColor(pal.AccentInk)
			})
		}
	})
}

func (a *App) dockerResourceBulkBar(c *ui.Context, slot *dockerResourceSlot) {
	var selected []docker.Resource
	for _, item := range slot.items {
		if slot.bulk.selected[item.ID] {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return
	}
	floatingBulkBar(c, "docker-resource-bulk", locales.DockerSelected(len(selected)), slot.bulk.clear, func(_, dangerFg, dangerBg ui.Color) {
		bulkAction(c.Key("remove"), "trash-2", locales.DockerContainerRemove()+"…", dangerFg, dangerBg, !a.canActOnDockerResources(), func() {
			a.planDockerResourcesRemoval(selected)
		})
	})
}

func (a *App) canActOnDockerResources() bool {
	s := &a.engine.resources
	slot := &s.slots[dockerResourceSlotIndex(s.kind)]
	return a.canPlanDocker() && s.watching != "" && !s.planning && slot.err == nil && a.engine.err == nil
}

func (a *App) planDockerResourcesRemoval(items []docker.Resource) {
	if !a.canActOnDockerResources() || len(items) == 0 {
		return
	}
	s := &a.engine.resources
	target, kind, revision, view := s.target, s.kind, s.revision, a.engine.view
	runner := cmp.Or[run.Runner](a.Runner, run.Exec{})
	s.planning = true
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerResourcesRemoval(context.Background(), runner, target, env, kind, items)
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
