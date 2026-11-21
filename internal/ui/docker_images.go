package ui

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type dockerImagesState struct {
	target   docker.Target
	watching string
	revision int
	stop     func()
	items    []docker.Image
	err      error
	loaded   bool
	filter   int
	query    string
	sort     tableSort
	groups   [2]*bool
	bulk     resourceSelection
	pullOpen bool
	pullRef  string
	planning bool
}

func (s *dockerImagesState) stopWatch() {
	if s.stop != nil {
		s.stop()
	}
	s.stop, s.watching = nil, ""
	s.revision++
	s.planning = false
}

func (a *App) syncDockerImages(t docker.Target, key string) {
	s := &a.engine.images
	if a.engine.view != dockerImages || a.WatchDockerImages == nil {
		key = ""
	}
	if key == s.watching {
		return
	}
	s.stopWatch()
	if t != (docker.Target{}) && s.target != t {
		s.items, s.err, s.loaded = nil, nil, false
		s.pullOpen, s.pullRef = false, ""
		s.bulk.clear()
	} else if key == "" {
		s.pullOpen, s.pullRef = false, ""
		s.bulk.clear()
	}
	if key == "" {
		return
	}
	s.target, s.watching = t, key
	revision := s.revision
	s.stop = a.WatchDockerImages(t, func(items []docker.Image, err error) {
		if s.revision != revision {
			return
		}
		s.err = err
		if err != nil {
			return
		}
		s.items, s.loaded = items, true
		for id := range s.bulk.selected {
			if !slices.ContainsFunc(items, func(item docker.Image) bool { return item.ID == id }) {
				delete(s.bulk.selected, id)
			}
		}
	})
}

const (
	dockerImagesAll = iota
	dockerImagesUsed
	dockerImagesDangling
	dockerImagesEmulated
)

func showDockerImage(item docker.Image, filter int, query, arch string) bool {
	switch {
	case filter == dockerImagesUsed && len(item.Users) == 0:
		return false
	case filter == dockerImagesDangling && !item.Dangling():
		return false
	case filter == dockerImagesEmulated && !item.Emulated(arch):
		return false
	}
	return query == "" || strings.Contains(item.ID, query) ||
		slices.ContainsFunc(item.RepoTags, func(tag string) bool { return strings.Contains(strings.ToLower(tag), query) }) ||
		slices.ContainsFunc(item.RepoDigests, func(digest string) bool { return strings.Contains(strings.ToLower(digest), query) })
}

func (a *App) dockerImagesView(c *ui.Context) {
	s := &a.engine.images
	if !s.loaded {
		if s.err != nil {
			toolNote(c, s.err.Error())
		} else if a.WatchDockerImages != nil {
			tableSkeleton(c, locales.DockerImagesReading())
		}
		return
	}
	if s.err != nil {
		toolNote(c, locales.KubeClusterStale(s.err.Error()))
	}
	arch := a.engine.data.Arch
	var counts [4]int
	for _, item := range s.items {
		for tab := range counts {
			if showDockerImage(item, tab, "", arch) {
				counts[tab]++
			}
		}
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Shrink(0).Children(func() {
		segments(c, "docker-image-filter", &s.filter, []segment{
			{locales.TabAll(), strconv.Itoa(counts[dockerImagesAll])},
			{locales.DockerImagesUsed(), strconv.Itoa(counts[dockerImagesUsed])},
			{locales.DockerImagesDangling(), strconv.Itoa(counts[dockerImagesDangling])},
			{locales.DockerImagesEmulation(), strconv.Itoa(counts[dockerImagesEmulated])},
		})
		ui.Box(c).Grow(1)
		pull := ui.Button(c.Key("docker-pull"), locales.DockerImageDownload()+"…").Height(controlHeight).Shrink(0).
			Disabled(!a.canActOnDockerImages()).OnClick(func() { s.pullOpen = !s.pullOpen })
		ui.Popover(c, pull, &s.pullOpen, func() { a.dockerPullForm(c) })
		width := float32(270)
		if narrow(c) {
			width = 160
		}
		ui.SearchField(c.Key("docker-image-search"), &s.query).Label(locales.DockerImageSearch()).
			Placeholder(locales.DockerImageSearch()).Height(controlHeight).Width(width).MinWidth(110).Shrink(1)
	})
	var groups [2][]docker.Image
	var ids []string
	var total int64
	query := strings.ToLower(strings.TrimSpace(s.query))
	for _, item := range s.items {
		if !showDockerImage(item, s.filter, query, arch) {
			continue
		}
		group := 0
		if item.Dangling() {
			group = 1
		}
		groups[group] = append(groups[group], item)
		ids = append(ids, item.ID)
		total += item.Size
	}
	for i := range groups {
		sortDockerImages(s.sort, groups[i], arch)
	}
	tracks := dockerImageTracks(c)
	hasBulk := len(s.bulk.selected) > 0
	workloadPanes(c.Key("docker-image-panes"), func() {
		sortHead(c, tracks, func() {
			s.bulk.checkbox(c, "image-select-all", locales.KubeBulkSelectAll(), ids)
		}, &s.sort,
			sortColumn{locales.KubeClusterName(), "name"},
			sortColumn{locales.KubeClusterStatus(), "status"},
			sortColumn{locales.DockerImageUsers(), "users"},
			sortColumn{locales.DockerImageSize(), "size"},
			sortColumn{locales.DockerContainerCreated(), "created"},
			sortColumn{"", ""},
		)
	}, func() {
		expand := false
		for i, items := range groups {
			if len(items) > 0 {
				open := s.groupOpen(i)
				expand = expand || !open
				a.dockerImageGroup(c.Key(strconv.Itoa(i)), i, items, tracks, arch, open)
			}
		}
		if len(ids) == 0 {
			ui.Text(c, locales.DockerImagesEmpty()).FontSize(13).TextColor(c.Theme().TextMuted).Padding(12, 16).Wrap()
		}
		pal := Colors(c.Theme())
		if cacheSize := a.dockerImageCacheSize(); s.filter == dockerImagesAll && query == "" && cacheSize > 0 {
			workloadSelectionRow(c, pal.Line).MinHeight(40).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Width(18).Shrink(0)
				ui.Text(c, locales.DockerImageBuildCache()).FontSize(13).FontWeight(600)
				aiTag(c, formatBytes(cacheSize), pal.Muted, c.Theme().TextMuted)
				ui.Box(c).Grow(1)
				a.dockerImageCleanup(c)
			})
		}
		ui.Row(c).Gap(8).Padding(8, 14).AlignItems(ui.Center).Background(pal.Subtle).Shrink(0).Children(func() {
			ui.Text(c, locales.CleanupImageCount(len(ids))+" · "+formatBytes(total)).FontSize(11.5).TextColor(c.Theme().TextMuted).Grow(1).Wrap()
			if len(ids) > 0 {
				label := locales.DockerCollapseAll()
				if expand {
					label = locales.DockerExpandAll()
				}
				ui.Button(c.Key("image-expand-all"), label).Height(24).Shrink(0).OnClick(func() {
					for i := range groups {
						s.groups[i] = new(expand)
					}
				})
			}
		})
		if hasBulk {
			ui.Box(c).Height(56).Shrink(0)
		}
	}, "", nil)
	a.dockerImageBulkBar(c)
}

func dockerImageTracks(c *ui.Context) []ui.Track {
	if narrow(c) {
		return []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(128), ui.Fixed(104), ui.Fixed(62), ui.Fixed(52), ui.Fixed(54)}
	}
	return []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(172), ui.Fixed(186), ui.Fixed(70), ui.Fixed(80), ui.Fixed(54)}
}

func sortDockerImages(s tableSort, items []docker.Image, arch string) {
	statusRank := func(item docker.Image) int {
		switch {
		case item.Emulated(arch):
			return 0
		case slices.ContainsFunc(item.Users, func(user docker.ImageUser) bool { return user.Active }):
			return 1
		case len(item.Users) > 0:
			return 2
		default:
			return 3
		}
	}
	sortRows(s, items, map[string]func(x, y docker.Image) int{
		"name": func(x, y docker.Image) int {
			return cmp.Compare(strings.ToLower(x.Name()), strings.ToLower(y.Name()))
		},
		"status": func(x, y docker.Image) int {
			if n := cmp.Compare(statusRank(x), statusRank(y)); n != 0 {
				return n
			}
			return cmp.Compare(strings.ToLower(x.Name()), strings.ToLower(y.Name()))
		},
		"users": func(x, y docker.Image) int {
			return cmp.Compare(len(x.Users), len(y.Users))
		},
		"size": func(x, y docker.Image) int {
			return cmp.Compare(x.Size, y.Size)
		},
		"created": func(x, y docker.Image) int {
			return x.Created.Compare(y.Created)
		},
	})
}

func (s *dockerImagesState) groupOpen(i int) bool {
	if open := s.groups[i]; open != nil {
		return *open
	}
	return i == 0 || s.filter != dockerImagesAll || strings.TrimSpace(s.query) != ""
}

func (a *App) dockerImageGroup(c *ui.Context, group int, items []docker.Image, tracks []ui.Track, arch string, open bool) {
	s := &a.engine.images
	pal := Colors(c.Theme())
	name := locales.DockerImagesTagged()
	if group == 1 {
		name = locales.DockerImagesDanglingGroup()
	}
	var ids []string
	var size int64
	for _, item := range items {
		ids = append(ids, item.ID)
		size += item.Size
	}
	ui.Column(c).Children(func() {
		line := workloadSelectionRow(c.Key("image-group-row"), pal.Line).AlignItems(ui.Center)
		line.Children(func() {
			ui.Column(c).Width(18).Shrink(0).Children(func() {
				s.bulk.checkbox(c, "image-select-group", locales.KubeBulkSelect(name), ids)
			})
			btn := ui.ButtonBase(c.Key("image-group")).MinHeight(40).Grow(1).MinWidth(0).Gap(8).
				AlignItems(ui.Center).Justify(ui.Start).Label(name).OnClick(func() { s.groups[group] = new(!open) })
			bg := pal.Subtle
			if btn.Hovered() {
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
				aiTag(c, strconv.Itoa(len(items))+" · "+formatBytes(size), pal.Muted, c.Theme().TextMuted).Shrink(0)
			})
			if group == 1 {
				a.dockerImageCleanup(c)
			}
			ui.Box(c).Width(24)
		})
		if open {
			ui.Column(c.Key("image-group-items")).Transition(rowMotion).Children(func() {
				for _, item := range items {
					a.dockerImageRow(c.Key(item.ID), item, tracks, arch)
				}
			})
		}
	})
}

func (a *App) dockerImageCacheSize() int64 {
	if a.Sizes != nil && a.dockerTarget().Context == a.engine.images.target.Context {
		for _, row := range a.Sizes.Docker {
			if row.Type == "Build Cache" {
				return row.Bytes
			}
		}
	}
	return 0
}

func (a *App) dockerImageCleanup(c *ui.Context) {
	ui.Button(c.Key("docker-image-cleanup"), locales.ChangesOpenCleanup()).Height(24).Shrink(0).OnClick(func() {
		a.cleanupTab = slices.Index(cleanupGroups, cleanup.Docker) + 1
		a.cleanupFilter = ""
		a.router.Push(cleanupPage)
	})
}

func (a *App) dockerImageRow(c *ui.Context, item docker.Image, tracks []ui.Track, arch string) {
	t := c.Theme()
	pal := Colors(t)
	var users []string
	for _, user := range item.Users {
		users = append(users, user.Name)
	}
	names := strings.Join(users, ", ")
	id := strings.TrimPrefix(item.ID, "sha256:")
	platform := strings.Trim(strings.Join([]string{item.OS, item.Architecture}, "/"), "/")
	tags := strings.Join(append(slices.Clone(item.RepoTags), item.RepoDigests...), "\n")
	created, createdAt := "—", ""
	if !item.Created.IsZero() {
		created, createdAt = age(c, item.Created), item.Created.Local().Format(time.DateTime)
	}
	status, bg, fg := locales.KubeStorageUnused(), pal.Muted, t.TextMuted
	switch {
	case slices.ContainsFunc(item.Users, func(user docker.ImageUser) bool { return user.Active }):
		status, bg, fg = locales.DockerImagesUsed(), pal.SuccessSoft, pal.SuccessText
	case len(users) > 0:
		status = locales.DockerImageStoppedUser()
	}
	var issue *rowIssue
	if item.Emulated(arch) {
		issue = &rowIssue{
			key:      "docker-image:" + item.ID,
			label:    locales.Details() + " · " + item.Name(),
			severity: issueWarning,
			title:    item.Architecture + " · " + locales.DockerImagesEmulation(),
			body:     locales.DockerImageEmulationHint(item.Architecture, arch),
		}
	}
	menu := func(m *ui.Menu) {
		if m.Item(locales.KubeCopyName()).Chosen() {
			copyName(c, item.Name())
		}
		if m.Item(locales.DockerImageCopyID()).Chosen() {
			copyName(c, item.ID)
		}
		if a.canActOnDockerImages() && m.Item(locales.DockerContainerRemove()+"…").Chosen() {
			a.planDockerImageRemoval([]docker.Image{item})
		}
	}
	row := ui.Column(c).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).ContextMenu(menu)
	rowBg := t.Background.Alpha(0)
	switch {
	case item.Emulated(arch):
		rowBg = pal.WarningSoft.Alpha(0.22)
	case row.Hovered():
		rowBg = pal.Subtle
	}
	row.Background(row.AnimateColor("bg", rowBg, ui.Animation{Duration: 120 * time.Millisecond}))
	row.Children(func() {
		resourceGrid(ui.Box(c), tracks).Padding(6, 16).MinHeight(44).Children(func() {
			a.engine.images.bulk.checkbox(c, "docker-select-image", locales.KubeBulkSelect(item.Name()), []string{item.ID})
			ui.Row(c).Gap(10).Padding(0, 0, 0, 10).AlignItems(ui.Center).MinWidth(0).Children(func() {
				dockerTechTile(c, item.Name(), 26, nil)
				ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
					clipped(c, item.Name()).Font(FontMono).FontSize(12.5).Tooltip(strings.TrimSpace(item.Name() + "\n" + tags))
					clipped(c, strings.TrimSpace(platform+"  "+id[:min(12, len(id))])).
						Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).Tooltip(platform + " · " + item.ID)
				})
			})
			a.workloadStatusIssue(c, status, bg, fg, issue)
			if names == "" {
				names = "—"
			}
			clipped(c, names).Font(FontMono).FontSize(11.5).MinWidth(0).Tooltip(names)
			clipped(c, formatBytes(item.Size)).FontSize(11.5)
			clipped(c, created).FontSize(11.5).Tooltip(createdAt)
			ui.Row(c).Gap(2).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				if len(item.Users) == 0 && a.canActOnDockerImages() {
					iconButton(c.Key("image-remove"), "trash-2", locales.DockerContainerRemove()+"…").
						OnClick(func() { a.planDockerImageRemoval([]docker.Image{item}) })
				}
				iconButton(c.Key("image-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
			})
		})
	})
}

func (a *App) canActOnDockerImages() bool {
	s := &a.engine.images
	return a.canPlanDocker() && !s.planning && s.err == nil && a.engine.err == nil
}

func (a *App) dockerPullForm(c *ui.Context) {
	s := &a.engine.images
	ui.Column(c).Width(340).Padding(12).Gap(8).Children(func() {
		label := locales.DockerImageSearch()
		submit := func() {
			ref := strings.TrimSpace(s.pullRef)
			if !a.canActOnDockerImages() || !ops.ValidDockerImageRef(ref) {
				return
			}
			s.pullOpen = false
			target := s.target
			a.buildDockerImagePlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.PlanDockerPull(target, env, ref)
			})
		}
		ui.Text(c, locales.DockerImageDownload()).FontSize(14).FontWeight(600)
		ui.TextInput(c.Key("docker-image-reference"), &s.pullRef).Label(label).Placeholder("alpine:3.23").AutoFocus().OnChange(c.Invalidate).OnSubmit(submit)
		if strings.TrimSpace(s.pullRef) != "" && !ops.ValidDockerImageRef(strings.TrimSpace(s.pullRef)) {
			ui.Text(c, locales.DockerImageReferenceInvalid()).FontSize(11.5).TextColor(c.Theme().Warning).Wrap()
		}
		ui.Button(c.Key("docker-image-review"), locales.DockerImageReview()).AlignSelf(ui.End).
			Disabled(!a.canActOnDockerImages() || !ops.ValidDockerImageRef(strings.TrimSpace(s.pullRef))).OnClick(submit)
	})
}

func (a *App) dockerImageBulkBar(c *ui.Context) {
	s := &a.engine.images
	var selected []docker.Image
	for _, item := range s.items {
		if s.bulk.selected[item.ID] {
			selected = append(selected, item)
		}
	}
	if len(selected) == 0 {
		return
	}
	floatingBulkBar(c, "docker-image-bulk", locales.DockerSelected(len(selected)), s.bulk.clear, func(_, dangerFg, dangerBg ui.Color) {
		bulkAction(c.Key("remove"), "trash-2", locales.DockerContainerRemove()+"…", dangerFg, dangerBg, !a.canActOnDockerImages(), func() {
			a.planDockerImageRemoval(selected)
		})
	})
}

func (a *App) planDockerImageRemoval(items []docker.Image) {
	target := a.engine.images.target
	a.buildDockerImagePlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.PlanDockerImageRemoval(context.Background(), run.Exec{}, target, env, items)
	})
}

func (a *App) buildDockerImagePlan(build func(sysenv.Env, ops.Terminal) (ops.Plan, error)) {
	if !a.canActOnDockerImages() {
		return
	}
	s := &a.engine.images
	revision := s.revision
	s.planning = true
	a.BuildPlan(build, func(p ops.Plan, err error) {
		if s.revision != revision {
			return
		}
		s.planning = false
		if a.canActOnDockerImages() && a.router.Path() == dockerEnginePage &&
			a.router.Query("context") == s.target.Context && a.engine.view == dockerImages {
			a.proposeOrSay(p, err)
		}
	})
}
