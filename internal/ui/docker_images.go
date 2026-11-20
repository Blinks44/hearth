package ui

import (
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
	if key == "" || s.target != t {
		s.items, s.err, s.loaded = nil, nil, false
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
		pull := ui.Button(c.Key("docker-pull"), locales.DockerImageDownload()+"…").Height(24).Shrink(0).
			Disabled(!a.canActOnDockerImages()).OnClick(func() { s.pullOpen = !s.pullOpen })
		ui.Popover(c, pull, &s.pullOpen, func() { a.dockerPullForm(c) })
		width := float32(270)
		if narrow(c) {
			width = 160
		}
		ui.SearchField(c.Key("docker-image-search"), &s.query).Label(locales.DockerImageSearch()).
			Placeholder(locales.DockerImageSearch()).Width(width).MinWidth(110).Shrink(1)
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
	tracks := dockerImageTracks(c)
	workloadPanes(c.Key("docker-image-panes"), func() {
		workloadHead(c, tracks, func() {
			s.bulk.checkbox(c, "image-select-all", locales.KubeBulkSelectAll(), ids)
		}, locales.KubeClusterName(), locales.KubeClusterStatus(), locales.DockerImageUsers(), locales.DockerImageSize(), locales.DockerContainerCreated(), "")
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
	}, "", nil)
	a.dockerImageBulkBar(c)
}

func dockerImageTracks(c *ui.Context) []ui.Track {
	if narrow(c) {
		return []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(116), ui.Fixed(130), ui.Fixed(64), ui.Fixed(54), ui.Fixed(24)}
	}
	return []ui.Track{ui.Fixed(18), ui.Fr(1), ui.Fixed(162), ui.Fixed(226), ui.Fixed(63), ui.Fixed(89), ui.Fixed(24)}
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
	workloadSelectionRow(c, pal.Line).Background(pal.Subtle).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Width(18).Shrink(0).Children(func() {
			s.bulk.checkbox(c, "image-select-group", locales.KubeBulkSelect(name), ids)
		})
		ui.ButtonBase(c.Key("image-group")).MinHeight(40).Grow(1).MinWidth(0).Gap(8).
			AlignItems(ui.Center).Justify(ui.Start).Label(name).OnClick(func() { s.groups[group] = new(!open) }).Children(func() {
			icon := "chevron-right"
			if open {
				icon = "chevron-down"
			}
			ui.Icon(c, Icon(icon)).Size(14, 14).Shrink(0).TextColor(pal.TextFaint)
			clipped(c, name).FontSize(13).FontWeight(600).MinWidth(0)
			aiTag(c, strconv.Itoa(len(items))+" · "+formatBytes(size), pal.Muted, c.Theme().TextMuted).Shrink(0)
		})
		if group == 1 {
			a.dockerImageCleanup(c)
		}
		ui.Box(c).Width(24)
	})
	if open {
		for _, item := range items {
			a.dockerImageRow(c.Key(item.ID), item, tracks, arch)
		}
	}
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
	pal := Colors(c.Theme())
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
	status, bg, fg := locales.KubeStorageUnused(), pal.Muted, c.Theme().TextMuted
	switch {
	case item.Emulated(arch):
		status, bg, fg = item.Architecture+" · "+locales.DockerImagesEmulation(), pal.WarningSoft, pal.WarningText
	case slices.ContainsFunc(item.Users, func(user docker.ImageUser) bool { return user.Active }):
		status, bg, fg = locales.DockerImagesUsed(), pal.SuccessSoft, pal.SuccessText
	case len(users) > 0:
		status = locales.DockerImageStoppedUser()
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
	if item.Emulated(arch) {
		row.Background(bg.Alpha(0.24))
	}
	row.Children(func() {
		resourceGrid(ui.Box(c), tracks).Padding(6, 16).MinHeight(39).Children(func() {
			a.engine.images.bulk.checkbox(c, "docker-select-image", locales.KubeBulkSelect(item.Name()), []string{item.ID})
			ui.Column(c).Gap(1).Padding(0, 0, 0, 23).MinWidth(0).Children(func() {
				clipped(c, item.Name()).Font(FontMono).FontSize(12.5).Tooltip(strings.TrimSpace(item.Name() + "\n" + tags))
				clipped(c, strings.TrimSpace(platform+"  "+id[:min(12, len(id))])).
					Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint).Tooltip(platform + " · " + item.ID)
			})
			workloadStatus(c, status, bg, fg)
			if names == "" {
				names = "—"
			}
			clipped(c, names).Font(FontMono).FontSize(11.5).MinWidth(0).Tooltip(names)
			clipped(c, formatBytes(item.Size)).FontSize(11.5)
			clipped(c, created).FontSize(11.5).Tooltip(createdAt)
			iconButton(c.Key("image-menu"), "ellipsis", locales.MoreActions()).Menu(menu)
		})
		if item.Emulated(arch) {
			ui.Row(c).AlignItems(ui.Center).Gap(7).Padding(0, 16, 8, 67).Children(func() {
				ui.Icon(c, Icon("triangle-alert")).Size(13, 13).Shrink(0).TextColor(fg)
				ui.Text(c, locales.DockerImageEmulationHint(item.Architecture, arch)).FontSize(11.5).TextColor(fg).Wrap().Grow(1).MinWidth(0)
			})
		}
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
	fg, bg := ui.RGB(250, 249, 247), ui.RGB(31, 29, 26)
	ui.Row(c.Key("docker-image-bulk")).Gap(5).Padding(6, 10).Radius(10).AlignItems(ui.Center).Background(bg).Shrink(0).Children(func() {
		ui.Text(c, locales.DockerSelected(len(selected))).FontSize(12).TextColor(fg)
		bulkAction(c.Key("remove"), "trash-2", locales.DockerContainerRemove()+"…", fg, ui.Transparent, !a.canActOnDockerImages(), func() {
			a.planDockerImageRemoval(selected)
		})
		bulkAction(c.Key("clear"), "", locales.KubeBulkClear(), fg, ui.Transparent, false, s.bulk.clear)
	})
	c.OnShortcut(0, ui.KeyEscape, s.bulk.clear)
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
