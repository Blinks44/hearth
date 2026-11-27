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

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// cleanupPage is the Cleanup page's path.
const cleanupPage = "/cleanup"

// cleanupAgeThresholds maps a.cleanupAge tab index to minimum age.
var cleanupAgeThresholds = []time.Duration{
	0,
	30 * 24 * time.Hour,
	90 * 24 * time.Hour,
	180 * 24 * time.Hour,
}

// caches name the tools' caches by ID.
var caches = map[string]string{
	"npm":             "npm",
	"pnpm":            "pnpm",
	"yarn":            "Yarn",
	"yarn-berry":      "Yarn Berry",
	"bun":             "Bun",
	"uv":              "uv",
	"pip":             "pip",
	"brew":            "Homebrew",
	"go-mod":          "Go modules",
	"go-build":        "Go build",
	"cargo":           "Cargo",
	"rustup":          "rustup",
	"gradle":          "Gradle",
	"maven":           "Maven",
	"nuget":           "NuGet",
	"composer":        "Composer",
	"ccache":          "ccache",
	"xcode":           "Xcode DerivedData",
	"cocoapods":       "CocoaPods",
	"pub":             "Flutter pub",
	"playwright":      "Playwright",
	"claude-cache":    "Claude Code",
	"codex-cache":     "Codex",
	"codex-worktrees": "Codex",
	"gemini-cache":    "Gemini CLI",
	"copilot-cache":   "Copilot CLI",
	"opencode-cache":  "OpenCode",
	"cursor-cache":    "Cursor",
	"jetbrains":       "JetBrains",
}

// cleanupGroups are the Cleanup page's groups, a tab each after All.
var cleanupGroups = []cleanup.Group{cleanup.Caches, cleanup.Copies, cleanup.Docker, cleanup.Projects, cleanup.Models, cleanup.History}

// candidates are what the last scan and count offer to clean up; nil
// before the first scan.
func (a *App) candidates(c *ui.Context) []cleanup.Candidate {
	var lines map[int]versions.Line
	if a.NodeVersions != nil {
		lines = a.NodeVersions.Lines
	}
	var opts cleanup.Options
	if a.CleanupStaleDays > 0 {
		opts.StaleAfter = time.Duration(a.CleanupStaleDays) * 24 * time.Hour
	}
	if a.CleanupMinMB > 0 {
		opts.MinFrees = int64(a.CleanupMinMB) * 1_000_000
	}
	return cleanup.FindWithOptions(a.Snapshot, a.Sizes, a.Projects, lines, c.Now(), opts)
}

// candidateKey tells a candidate apart across scans, for the user's
// choice of it.
func candidateKey(cand cleanup.Candidate) string {
	return string(cand.Kind) + "\x00" + cand.ID + "\x00" + strings.Join(cand.Paths, "\x00")
}

// chosen reports whether the candidate is chosen: as the user set it, or
// as Hearth offers it. One the plan cannot remove, as a copy from winget,
// is never chosen.
func (a *App) chosen(cand cleanup.Candidate) bool {
	if !ops.Cleans(cand, a.Snapshot) {
		return false
	}
	if v, ok := a.cleanupChoice[candidateKey(cand)]; ok {
		return v
	}
	return cand.Selected
}

// freeable is what the candidates free, all of them.
func freeable(cands []cleanup.Candidate) int64 {
	var n int64
	for _, cand := range cands {
		n += cand.Frees()
	}
	return n
}

// cleanupAmount formats the total space candidates free.
func cleanupAmount(cands []cleanup.Candidate) string {
	return formatBytes(freeable(cands))
}

type cleanupGroupRows struct {
	group cleanup.Group
	rows  []cleanup.Candidate
}

func (a *App) visibleCleanupGroups(cands []cleanup.Candidate, now time.Time) (groups []cleanupGroupRows, visible []cleanup.Candidate) {
	filter := strings.ToLower(strings.TrimSpace(a.cleanupFilter))
	var minAge time.Duration
	if a.cleanupAge > 0 && a.cleanupAge < len(cleanupAgeThresholds) {
		minAge = cleanupAgeThresholds[a.cleanupAge]
	}
	for _, g := range cleanupGroups {
		if a.cleanupTab > 0 && cleanupGroups[a.cleanupTab-1] != g {
			continue
		}
		var rows []cleanup.Candidate
		for _, cand := range cands {
			if cand.Kind.Group() != g {
				continue
			}
			if minAge > 0 && (cand.Changed.IsZero() || now.Sub(cand.Changed) < minAge) {
				continue
			}
			if filter != "" {
				haystack := strings.ToLower(a.candidateTitle(cand) + " " + a.candidateCaption(cand) + " " + candidateRoleLabel(cand))
				if !strings.Contains(haystack, filter) {
					continue
				}
			}
			rows = append(rows, cand)
		}
		if len(rows) > 0 {
			sortCleanupRows(rows, a.cleanupSort == 1)
			groups = append(groups, cleanupGroupRows{group: g, rows: rows})
			visible = append(visible, rows...)
		}
	}
	return groups, visible
}

func sortCleanupRows(rows []cleanup.Candidate, oldestFirst bool) {
	if !oldestFirst {
		slices.SortStableFunc(rows, func(a, b cleanup.Candidate) int {
			return cmp.Compare(b.Frees(), a.Frees())
		})
		return
	}
	slices.SortStableFunc(rows, func(a, b cleanup.Candidate) int {
		aKnown, bKnown := !a.Changed.IsZero(), !b.Changed.IsZero()
		if aKnown != bKnown {
			if aKnown {
				return -1
			}
			return 1
		}
		if aKnown && bKnown {
			if c := a.Changed.Compare(b.Changed); c != 0 {
				return c
			}
		}
		return cmp.Compare(b.Frees(), a.Frees())
	})
}

type cleanupProjectBucket struct {
	project string
	rows    []cleanup.Candidate
	total   int64
	changed time.Time
}

func groupProjectCandidates(rows []cleanup.Candidate, oldestFirst bool) []cleanupProjectBucket {
	index := map[string]int{}
	var buckets []cleanupProjectBucket
	for _, cand := range rows {
		i, ok := index[cand.Project]
		if !ok {
			i = len(buckets)
			index[cand.Project] = i
			buckets = append(buckets, cleanupProjectBucket{project: cand.Project})
		}
		b := &buckets[i]
		b.rows = append(b.rows, cand)
		b.total += cand.Frees()
		if cand.Changed.After(b.changed) {
			b.changed = cand.Changed
		}
	}
	for i := range buckets {
		sortCleanupRows(buckets[i].rows, false)
	}
	slices.SortStableFunc(buckets, func(a, b cleanupProjectBucket) int {
		if oldestFirst {
			aKnown, bKnown := !a.changed.IsZero(), !b.changed.IsZero()
			if aKnown != bKnown {
				if aKnown {
					return -1
				}
				return 1
			}
			if aKnown && bKnown {
				if c := a.changed.Compare(b.changed); c != 0 {
					return c
				}
			}
		}
		return cmp.Compare(b.total, a.total)
	})
	return buckets
}

func maxCandidateFrees(cands []cleanup.Candidate) int64 {
	var best int64
	for _, cand := range cands {
		if cand.Known && cand.Frees() > best {
			best = cand.Frees()
		}
	}
	return best
}

func candidateRole(cand cleanup.Candidate) string {
	if cand.Kind != cleanup.ProjectInstalls {
		return ""
	}
	if cand.Role != "" {
		return cand.Role
	}
	return projects.InstallRole(filepath.Base(cand.ID))
}

func candidateRoleLabel(cand cleanup.Candidate) string {
	switch candidateRole(cand) {
	case inventory.InstallRoleDeps:
		return locales.CleanupRoleDeps()
	case inventory.InstallRoleBuild:
		return locales.CleanupRoleBuild()
	}
	return ""
}

func (a *App) cleanupView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		t := c.Theme()
		pal := Colors(t)
		now := c.Now()
		cands := a.candidates(c)
		visibleGroups, visibleCands := a.visibleCleanupGroups(cands, now)
		maxBytes := maxCandidateFrees(visibleCands)
		var selected []cleanup.Candidate
		for _, cand := range visibleCands {
			if a.chosen(cand) {
				selected = append(selected, cand)
			}
		}
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			a.recountSizesButton(c)
			a.cleanupButton(c, selected)
		}})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(16).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.AccentSoft).Center().Children(func() {
						ui.Icon(c, Icon("brush-cleaning")).Size(26, 26).TextColor(Accent)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, locales.CleanupTitle()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.CleanupLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					found, chosen := "—", "—"
					if a.Sizes != nil {
						found, chosen = cleanupAmount(cands), cleanupAmount(selected)
					}
					stateCard(c, pal.TextFaint, locales.CleanupFound(), found, locales.CleanupCandidates(len(cands)))
					stateCard(c, Accent, locales.CleanupSelected(), chosen, locales.CleanupCandidates(len(selected)))
					counted, caption := "—", locales.CleanupCounting()
					if a.Sizes != nil && !a.Sizes.Taken.IsZero() {
						counted, caption = ago(c, a.Sizes.Taken), locales.CleanupCountedCaption()
					}
					if a.Counting {
						stateCardBusy(c, locales.CleanupCounted(), counted, locales.CleanupCounting())
					} else {
						stateCard(c, t.Success, locales.CleanupCounted(), counted, caption)
					}
				})
				if a.Sizes != nil && freeable(cands) > 0 {
					a.cleanupBreakdownBar(c, cands)
				}
				count := func(g cleanup.Group) string {
					n := 0
					for _, cand := range cands {
						if cand.Kind.Group() == g {
							n++
						}
					}
					return strconv.Itoa(n)
				}
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "cleanup-tabs", &a.cleanupTab, []segment{
						{locales.TabAll(), strconv.Itoa(len(cands))},
						{locales.CleanupCaches(), count(cleanup.Caches)},
						{locales.CleanupCopies(), count(cleanup.Copies)},
						{"Docker", count(cleanup.Docker)},
						{locales.ProjectsTitle(), count(cleanup.Projects)},
						{locales.CleanupModels(), count(cleanup.Models)},
						{locales.CleanupGroupHistory(), count(cleanup.History)},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("cleanup-filter"), &a.cleanupFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Height(controlHeight).Width(200)
				})
				if a.Sizes != nil && len(cands) > 0 {
					ageCount := func(d time.Duration) string {
						n := 0
						for _, cand := range cands {
							if !cand.Changed.IsZero() && now.Sub(cand.Changed) >= d {
								n++
							}
						}
						return strconv.Itoa(n)
					}
					ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Children(func() {
						segments(c, "cleanup-age", &a.cleanupAge, []segment{
							{locales.CleanupAgeAll(), ""},
							{locales.CleanupAge30(), ageCount(30 * 24 * time.Hour)},
							{locales.CleanupAge90(), ageCount(90 * 24 * time.Hour)},
							{locales.CleanupAge180(), ageCount(180 * 24 * time.Hour)},
						})
						ui.Box(c).Grow(1).Height(1)
						segments(c, "cleanup-sort", &a.cleanupSort, []segment{
							{locales.CleanupSortLargest(), ""},
							{locales.CleanupSortOldest(), ""},
						})
					})
				}
				columns(c, 12, func() {
					if a.Sizes != nil && a.Sizes.DockerErr != "" {
						warningRow(c, locales.CleanupDockerErr(a.Sizes.DockerErr))
					}
					switch {
					case a.Snapshot == nil || a.Sizes == nil || a.Counting && len(cands) == 0:
						toolNote(c, locales.CleanupWaiting())
						return
					case len(cands) == 0:
						toolNote(c, locales.CleanupEmpty())
						return
					}
					ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
						shown := 0
						for _, g := range visibleGroups {
							ui.Row(c.Key(cleanupGroupTitle(g.group))).Transition(rowMotion).Gap(12).Padding(6, 16).AlignItems(ui.Center).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
								a.groupCheckbox(c, cleanupGroupTitle(g.group), g.rows)
								ui.Text(c, strings.ToUpper(cleanupGroupTitle(g.group))).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
								ui.Text(c, cleanupAmount(g.rows)).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
								if a.Shell != nil {
									ui.Box(c).Size(24, 24).Shrink(0)
								}
							})
							if g.group == cleanup.Projects {
								for _, bucket := range groupProjectCandidates(g.rows, a.cleanupSort == 1) {
									open := a.cleanupProjectExpanded(bucket)
									a.projectGroupHeader(c, bucket, open)
									shown += len(bucket.rows)
									if !open {
										continue
									}
									for _, cand := range bucket.rows {
										a.candidateRow(c, cand, maxBytes, true)
									}
								}
								continue
							}
							for _, cand := range g.rows {
								shown++
								a.candidateRow(c, cand, maxBytes, false)
							}
						}
						if shown == 0 {
							cardNote(c, locales.CleanupNone())
						}
					})
					toolNote(c, locales.CleanupReadOnly())
				}, func() {
					var restorable, staleProjects []cleanup.Candidate
					for _, cand := range cands {
						if !cand.Selected || !ops.Cleans(cand, a.Snapshot) {
							continue
						}
						if cand.Kind.Group() == cleanup.Projects {
							staleProjects = append(staleProjects, cand)
						} else {
							restorable = append(restorable, cand)
						}
					}
					atOnceAside := ""
					if a.Sizes != nil {
						atOnceAside = formatBytes(freeable(restorable) + freeable(staleProjects))
					}
					listCard(c, locales.CleanupAtOnce(), atOnceAside, func() {
						size := "—"
						if a.Sizes != nil {
							size = cleanupAmount(restorable)
						}
						listRowWrapped(c, locales.CleanupAtOnceRow(), locales.CleanupAtOnceCaption(size), func() {})
						if len(staleProjects) > 0 {
							listRowWrapped(c, locales.CleanupAtOnceProjectsRow(), locales.CleanupAtOnceProjectsCaption(cleanupAmount(staleProjects)), func() {})
						}
					})
					listCard(c, locales.CleanupYourChoice(), "", func() {
						listRowWrapped(c, locales.CleanupCopiesRow(), locales.CleanupCopiesCaption(), func() {})
						listRowWrapped(c, locales.CleanupDockerRow(), locales.CleanupDockerCaption(), func() {})
						listRowWrapped(c, locales.CleanupGroupProjects(), locales.CleanupProjectsCaption(), func() {})
						listRowWrapped(c, locales.CleanupModelsRow(), locales.CleanupModelsCaption(), func() {})
						listRowWrapped(c, locales.CleanupHistoryRow(), locales.ReasonForgotten(), func() {})
					})
					listCard(c, locales.CleanupNotOffered(), "", func() {
						listRowWrapped(c, locales.CleanupVolumes(), locales.CleanupVolumesCaption(), func() {})
						listRowWrapped(c, locales.CleanupSystem(), locales.CleanupSystemCaption(), func() {})
						listRowWrapped(c, locales.CleanupProjectVersions(), locales.CleanupProjectVersionsCaption(), func() {})
					})
				})
			})
		})
	})
}

// cleanupBreakdownBar renders a stacked horizontal breakdown of reclaimable
// disk space across Caches, Dependencies, Build artifacts, Docker, and Other.
func (a *App) cleanupBreakdownBar(c *ui.Context, cands []cleanup.Candidate) {
	t := c.Theme()
	pal := Colors(t)
	var cachesBytes, depsBytes, buildBytes, dockerBytes, otherBytes int64
	for _, cand := range cands {
		n := cand.Frees()
		if n <= 0 {
			continue
		}
		switch cand.Kind.Group() {
		case cleanup.Caches:
			cachesBytes += n
		case cleanup.Docker:
			dockerBytes += n
		case cleanup.Projects:
			if candidateRole(cand) == inventory.InstallRoleBuild {
				buildBytes += n
			} else {
				depsBytes += n
			}
		default:
			otherBytes += n
		}
	}
	type segmentBar struct {
		key   string
		label string
		bytes int64
		color ui.Color
	}
	all := []segmentBar{
		{"caches", locales.CleanupCaches(), cachesBytes, Accent},
		{"deps", locales.CleanupRoleDeps(), depsBytes, pal.Info},
		{"build", locales.CleanupRoleBuild(), buildBytes, t.Warning},
		{"docker", "Docker", dockerBytes, t.Success},
		{"other", locales.CleanupBreakdownOther(), otherBytes, pal.TextFaint},
	}
	var active []segmentBar
	for _, seg := range all {
		if seg.bytes > 0 {
			active = append(active, seg)
		}
	}
	if len(active) == 0 {
		return
	}
	ui.Column(c.Key("cleanup-breakdown")).Gap(8).Padding(10, 14).Radius(10).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Children(func() {
		ui.Row(c).Gap(2).Height(8).Radius(4).Clip().Background(pal.Muted).Children(func() {
			for _, seg := range active {
				ui.Box(c.Key(seg.key)).Height(8).Grow(float32(seg.bytes)).MinWidth(6).Background(seg.color)
			}
		})
		ui.Row(c).Gap(14).Wrap().AlignItems(ui.Center).Children(func() {
			for _, seg := range active {
				ui.Row(c.Key("legend-" + seg.key)).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(seg.color).Shrink(0)
					ui.Text(c, seg.label).FontSize(11).TextColor(t.TextMuted).SingleLine()
					ui.Text(c, formatBytes(seg.bytes)).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
				})
			}
		})
	})
}

// cleanupProjectExpanded reports whether a project bucket in Cleanup shows its
// child dependency/build rows: matches during search always expand, explicit
// user toggles win, and otherwise 1-2 folders start open while 3+ start
// collapsed so large monorepos stay scannable.
func (a *App) cleanupProjectExpanded(bucket cleanupProjectBucket) bool {
	if strings.TrimSpace(a.cleanupFilter) != "" {
		return true
	}
	if a.cleanupProjectOpen != nil {
		if open, ok := a.cleanupProjectOpen[bucket.project]; ok {
			return open
		}
	}
	return len(bucket.rows) <= 2
}

func (a *App) toggleCleanupProject(project string, open bool) {
	if a.cleanupProjectOpen == nil {
		a.cleanupProjectOpen = map[string]bool{}
	}
	a.cleanupProjectOpen[project] = !open
}

// projectGroupHeader renders a collapsible project sub-header in Cleanup.
func (a *App) projectGroupHeader(c *ui.Context, bucket cleanupProjectBucket, open bool) {
	t := c.Theme()
	pal := Colors(t)
	name := filepath.Base(bucket.project)
	caption := tilde(bucket.project, a.Home)
	if !bucket.changed.IsZero() {
		caption += " · " + locales.CleanupLastActive(bucket.changed)
	}
	chev := "chevron-down"
	if !open {
		chev = "chevron-right"
	}
	ui.Row(c.Key("proj-"+bucket.project)).Transition(rowMotion).Gap(12).Padding(6, 16).MinHeight(40).AlignItems(ui.Center).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		a.groupCheckbox(c, name, bucket.rows)
		ui.ButtonBase(c.Key("toggle")).Row().Grow(1).MinWidth(0).Gap(8).Justify(ui.Start).AlignItems(ui.Center).Label(name).OnClick(func() {
			a.toggleCleanupProject(bucket.project, open)
		}).Children(func() {
			checkSize := t.Space(4)
			ui.Box(c).Size(checkSize, checkSize).Center().Shrink(0).Children(func() {
				ui.Icon(c, Icon(chev)).Size(13, 13).TextColor(pal.TextFaint)
			})
			ui.Icon(c, Icon("folder-git-2")).Size(15, 15).TextColor(pal.TextFaint).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					clipped(c, name).FontSize(12.5).FontWeight(600).TextColor(t.Text)
					if len(bucket.rows) > 1 {
						ui.Text(c, locales.EditorsFolders(len(bucket.rows))).FontSize(10).FontWeight(500).TextColor(t.TextMuted).SingleLine().Shrink(0).Padding(1, 6).Radius(4).Background(pal.Muted)
					}
				})
				clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint)
			})
			ui.Text(c, cleanupAmount(bucket.rows)).Font(FontMono).FontSize(11.5).TextColor(t.TextMuted).Shrink(0).SingleLine()
		})
		if a.Shell != nil {
			proj := bucket.project
			iconButton(c, "folder-open", showInFolder()).OnClick(func() { a.Shell.ShowItemInFolder(proj) })
		}
	})
}

// recountSizesButton triggers a fresh disk-usage count right now.
func (a *App) recountSizesButton(c *ui.Context) {
	ui.Button(c.Key("cleanup-recount"), locales.CleanupRecount()).
		Height(controlHeight).
		Disabled(a.RecountSizes == nil || a.Counting || a.Snapshot == nil).
		OnClick(func() {
			if a.RecountSizes != nil {
				a.RecountSizes()
			}
		})
}

// cleanupButton is the header's button that proposes removing the
// selected candidates the plan removes; nothing when there are none.
func (a *App) cleanupButton(c *ui.Context, selected []cleanup.Candidate) {
	if a.BuildPlan == nil || a.Sizes == nil {
		return
	}
	var cands []cleanup.Candidate
	var titles []string
	for _, cand := range selected {
		if ops.Cleans(cand, a.Snapshot) {
			cands, titles = append(cands, cand), append(titles, a.candidateTitle(cand))
		}
	}
	if len(cands) == 0 {
		return
	}
	ui.PrimaryButton(c.Key("cleanup-run"), locales.CleanupRemoveSelected(cleanupAmount(cands))).
		Height(controlHeight).
		Disabled(a.Counting).
		OnClick(func() {
			target, snap, count := a.dockerTarget(), a.Snapshot, a.CountDocker
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				var docker ops.CountDocker
				if count != nil {
					docker = func(ctx context.Context) ([]inventory.DockerUsage, error) { return count(ctx, env, snap) }
				}
				return ops.CleanupPlan(cands, titles, snap, env, target, docker)
			}, a.proposeOrSay)
		})
}

// dockerTarget is the terminal's Docker CLI and its selected context,
// which Docker's candidates were counted in.
func (a *App) dockerTarget() ops.DockerTarget {
	st, _ := a.Snapshot.Tool("docker")
	i := inventory.TerminalCopy(st.Installations)
	if i < 0 || st.Docker == nil {
		return ops.DockerTarget{}
	}
	endpoint := ""
	for _, k := range st.Docker.Contexts {
		if k.Name == st.Docker.Selected {
			endpoint = k.Endpoint
			break
		}
	}
	return ops.DockerTarget{CLI: st.Installations[i].Path, Context: st.Docker.Selected, Endpoint: endpoint}
}

// groupCheckbox is a group's check box: checked when every row shown the
// plan removes is chosen, mixed when some are; a click chooses them all,
// or none when all were.
func (a *App) groupCheckbox(c *ui.Context, title string, rows []cleanup.Candidate) {
	var keys []string
	on := 0
	for _, cand := range rows {
		if ops.Cleans(cand, a.Snapshot) {
			keys = append(keys, candidateKey(cand))
			if a.chosen(cand) {
				on++
			}
		}
	}
	all := ui.CheckOff
	switch {
	case on > 0 && on == len(keys):
		all = ui.CheckOn
	case on > 0:
		all = ui.CheckMixed
	}
	if ui.CheckboxState(c.Key("all"), &all, "").Label(locales.CleanupSelectGroup(title)).Disabled(len(keys) == 0).Changed() {
		if a.cleanupChoice == nil {
			a.cleanupChoice = map[string]bool{}
		}
		for _, key := range keys {
			a.cleanupChoice[key] = all == ui.CheckOn
		}
	}
}

func candidateBarColor(t *ui.Theme, pal Palette, cand cleanup.Candidate) ui.Color {
	switch cand.Kind.Group() {
	case cleanup.Caches:
		return Accent
	case cleanup.Docker:
		return t.Success
	case cleanup.Projects:
		if candidateRole(cand) == inventory.InstallRoleBuild {
			return t.Warning
		}
		return pal.Info
	}
	return pal.TextFaint
}

// candidateRow is a candidate in the table: its check box, what it is and
// why Hearth offers it, and the space it frees with a relative bar.
func (a *App) candidateRow(c *ui.Context, cand cleanup.Candidate, maxBytes int64, grouped bool) {
	t := c.Theme()
	pal := Colors(t)
	key := candidateKey(cand)
	leftPad := float32(16)
	if grouped {
		leftPad = 16 + t.Space(4) + 12
	}
	ui.Row(c.Key(key)).Transition(rowMotion).Gap(12).Padding(6, 16, 6, leftPad).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		on := a.chosen(cand)
		title := a.candidateTitle(cand)
		displayTitle := title
		if grouped && cand.Kind == cleanup.ProjectInstalls {
			displayTitle = cand.ID
		}
		if ui.Checkbox(c.Key("check"), &on, "").Label(title).Disabled(!ops.Cleans(cand, a.Snapshot)).Changed() {
			if a.cleanupChoice == nil {
				a.cleanupChoice = map[string]bool{}
			}
			a.cleanupChoice[key] = on
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			if role := candidateRole(cand); role != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					clipped(c, displayTitle).FontSize(13)
					bg, fg := pal.InfoSoft, pal.InfoText
					label := locales.CleanupRoleDeps()
					if role == inventory.InstallRoleBuild {
						bg, fg, label = pal.WarningSoft, pal.WarningText, locales.CleanupRoleBuild()
					}
					ui.Text(c, label).FontSize(10).FontWeight(500).TextColor(fg).SingleLine().Shrink(0).Padding(1, 6).Radius(4).Background(bg)
				})
			} else {
				clipped(c, displayTitle).FontSize(13)
			}
			caption := a.candidateCaption(cand)
			if grouped && cand.Kind == cleanup.ProjectInstalls {
				caption = a.groupedInstallCaption(cand)
			}
			if caption != "" {
				clipped(c, caption).FontSize(10.5).TextColor(pal.TextFaint)
			}
		})
		size := "—"
		if cand.Known {
			size = formatUsage(inventory.Usage{Bytes: cand.Frees(), Partial: cand.Usage.Partial})
		}
		ui.Column(c).AlignItems(ui.End).Gap(4).Shrink(0).Children(func() {
			ui.Text(c, size).Font(FontMono).FontSize(12).SingleLine()
			if cand.Known && cand.Frees() > 0 && maxBytes > 0 {
				const barW = float32(56)
				fill := max(3, float32(cand.Frees())/float32(maxBytes)*barW)
				ui.Box(c).Size(barW, 4).Radius(2).Clip().Background(pal.Muted).Children(func() {
					ui.Box(c).Size(fill, 4).Radius(2).Background(candidateBarColor(t, pal, cand))
				})
			}
		})
		if a.Shell != nil {
			switch len(cand.Paths) {
			case 0:
				ui.Box(c).Size(24, 24).Shrink(0)
			case 1:
				path := cand.Paths[0]
				iconButton(c, "folder-open", showInFolder()).OnClick(func() { a.Shell.ShowItemInFolder(path) })
			default:
				paths := cand.Paths
				iconButton(c, "folder-open", showInFolder()).Menu(func(m *ui.Menu) {
					limit := min(len(paths), 12)
					for _, p := range paths[:limit] {
						path := p
						if m.Item(tilde(path, a.Home)).Chosen() {
							a.Shell.ShowItemInFolder(path)
						}
					}
					if len(paths) > limit {
						m.Separator()
						parent := filepath.Dir(paths[0])
						if m.Item(locales.CleanupRevealMore(len(paths)-limit, tilde(parent, a.Home))).Chosen() {
							a.Shell.ShowItemInFolder(parent)
						}
					}
				})
			}
		}
	})
}

func (a *App) groupedInstallCaption(cand cleanup.Candidate) string {
	var parts []string
	for _, r := range cand.Reasons {
		switch r {
		case cleanup.Unchanged:
			parts = append(parts, locales.ReasonUnchanged(cand.Changed))
		case cleanup.Reinstalled:
			parts = append(parts, locales.ReasonReinstalled())
		}
	}
	return strings.Join(parts, " · ")
}

// candidateTitle says what a candidate is: "Bun cache", "Node.js 23.6.0 ·
// brew", "old-landing: node_modules".
func (a *App) candidateTitle(cand cleanup.Candidate) string {
	switch cand.Kind {
	case cleanup.Cache:
		if cand.ID == "codex-worktrees" {
			return locales.CleanupCodexWorktrees()
		}
		return locales.CleanupCache(cacheName(cand.ID))
	case cleanup.ExtraCopy:
		in := cand.Copies[0]
		return strings.TrimSpace(toolName(cand.ID)+" "+in.Version) + " · " + sourceName(in.Source)
	case cleanup.Leftover:
		return cand.ID
	case cleanup.UnneededPackage:
		return cand.ID + " · " + cand.Manager
	case cleanup.BuildCache:
		return locales.DockerImageBuildCache()
	case cleanup.UnusedImages:
		return locales.CleanupImages()
	case cleanup.StoppedContainers:
		return locales.CleanupContainers()
	case cleanup.ProjectInstalls:
		return filepath.Base(cand.Project) + ": " + cand.ID
	case cleanup.Model:
		return cand.ID + " · " + cand.Manager
	case cleanup.OldExtensions:
		return locales.EditorsOldTitle(cand.ID)
	case cleanup.AgentHistory:
		return locales.CleanupHistoryTitle(agentTitle(cand.ID))
	}
	return cand.ID
}

// candidateCaption says why Hearth offers a candidate, or for a cache the
// command its tool clears it with.
func (a *App) candidateCaption(cand cleanup.Candidate) string {
	var parts []string
	switch cand.Kind {
	case cleanup.Cache:
		if argv := ops.CacheCommand(cand.ID); argv != nil {
			return ops.Command(argv)
		}
		if len(cand.Paths) > 0 {
			parts = append(parts, tilde(cand.Paths[0], a.Home))
			if len(cand.Paths) > 1 {
				parts = append(parts, locales.CleanupMorePaths(len(cand.Paths)-1))
			}
		}
	case cleanup.BuildCache:
		parts = append(parts, "docker builder prune")
	case cleanup.UnusedImages:
		parts = append(parts, locales.CleanupImageCount(cand.Count))
	case cleanup.StoppedContainers:
		parts = append(parts, locales.CleanupContainerCount(cand.Count))
	case cleanup.ProjectInstalls:
		parts = append(parts, tilde(cand.Project, a.Home))
		if !cand.Changed.IsZero() && !slices.Contains(cand.Reasons, cleanup.Unchanged) {
			parts = append(parts, locales.CleanupLastActive(cand.Changed))
		}
	case cleanup.OldExtensions:
		parts = append(parts, locales.EditorsFolders(cand.Count))
	case cleanup.AgentHistory:
		parts = append(parts, locales.UsageSessionCount(cand.Count), locales.CleanupHistoryOld(int(cleanup.HistoryAfter.Hours()/24)))
	case cleanup.Model:
		if !cand.Changed.IsZero() {
			parts = append(parts, locales.ModelsDownloaded(locales.AgoOn(cand.Changed)))
		}
	}
	for _, r := range cand.Reasons {
		switch r {
		case cleanup.NotDefault:
			parts = append(parts, locales.ReasonNotDefault())
		case cleanup.Shadowed:
			parts = append(parts, locales.ReasonShadowed(a.runsInstead(cand.ID)))
		case cleanup.NotOnPath:
			parts = append(parts, locales.ReasonNotOnPath())
		case cleanup.NoProject:
			parts = append(parts, locales.ReasonNoProject())
		case cleanup.NotSetUp:
			parts = append(parts, locales.ReasonNotSetUp(cand.ID), locales.CleanupVersions(len(cand.Paths), toolName(cand.Copies[0].Tool)))
		case cleanup.NothingNeeds:
			parts = append(parts, locales.PkgNotNeeded())
		case cleanup.Rebuilt:
			parts = append(parts, locales.ReasonRebuilt())
		case cleanup.Pulled:
			parts = append(parts, locales.ReasonPulled())
		case cleanup.Stopped:
			parts = append(parts, locales.ReasonStopped())
		case cleanup.Unchanged:
			parts = append(parts, locales.ReasonUnchanged(cand.Changed))
		case cleanup.Reinstalled:
			parts = append(parts, locales.ReasonReinstalled())
		case cleanup.NoAgent:
			parts = append(parts, locales.ReasonNoAgent())
		case cleanup.Downloaded:
			parts = append(parts, locales.ReasonDownloaded())
		case cleanup.Superseded:
			parts = append(parts, locales.ReasonSuperseded())
		case cleanup.Forgotten:
			parts = append(parts, locales.ReasonForgotten())
		case cleanup.AbandonedWorktrees:
			parts = append(parts, locales.ReasonAbandonedWorktrees())
		}
	}
	return strings.Join(parts, " · ")
}

// runsInstead names the source of the tool's copy the terminal runs, as
// "fnm".
func (a *App) runsInstead(tool string) string {
	ts, _ := a.Snapshot.Tool(tool)
	if i := inventory.TerminalCopy(ts.Installations); i >= 0 {
		return sourceName(ts.Installations[i].Source)
	}
	return "PATH"
}

// toolName is a tool's name as the sidebar says it: "Node.js".
func toolName(id string) string {
	for _, l := range languages {
		if l.id == id {
			return l.title()
		}
	}
	return toolTitle(id)
}

func cacheName(id string) string {
	if name, ok := caches[id]; ok {
		return name
	}
	return id
}

func cleanupGroupTitle(g cleanup.Group) string {
	switch g {
	case cleanup.Caches:
		return locales.CleanupGroupCaches()
	case cleanup.Copies:
		return locales.CleanupGroupCopies()
	case cleanup.Docker:
		return "Docker"
	case cleanup.Models:
		return locales.CleanupGroupModels()
	case cleanup.History:
		return locales.CleanupGroupHistory()
	}
	return locales.CleanupGroupProjects()
}

// formatBytes is formatUsage of a whole count.
func formatBytes(n int64) string { return formatUsage(inventory.Usage{Bytes: n}) }

// ago says how long ago t was: "just now", "5 min ago", or the date.
func ago(c *ui.Context, t time.Time) string {
	since := c.Now().Sub(t)
	switch {
	case since < time.Minute:
		c.After(time.Minute - since)
		return locales.AgoJustNow()
	case since < time.Hour:
		c.After(time.Minute - since%time.Minute)
		return locales.AgoMinutes(int(since / time.Minute))
	case since < 24*time.Hour:
		c.After(time.Hour - since%time.Hour)
		return locales.AgoHours(int(since / time.Hour))
	}
	return locales.AgoOn(t)
}

// cleanupEnd is the Cleanup item's end in the sidebar: what can be freed.
func (a *App) cleanupEnd(c *ui.Context) func() {
	if a.Sizes == nil {
		return nil
	}
	cands := a.candidates(c)
	if freeable(cands) == 0 {
		return nil
	}
	return func() {
		ui.Text(c, cleanupAmount(cands)).Font(FontMono).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
	}
}
