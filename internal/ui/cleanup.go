package ui

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// cleanupPage is the Cleanup page's path.
const cleanupPage = "/cleanup"

// caches name the tools' caches by ID, and the command each tool clears
// its own with, where it has one.
var caches = map[string]struct{ name, command string }{
	"npm":        {"npm", "npm cache clean --force"},
	"pnpm":       {"pnpm", "pnpm store prune"},
	"yarn":       {"Yarn", "yarn cache clean"},
	"yarn-berry": {"Yarn Berry", "yarn cache clean --mirror"},
	"bun":        {"Bun", "bun pm cache rm"},
	"uv":         {"uv", "uv cache clean"},
	"pip":        {"pip", "pip cache purge"},
	"brew":       {"Homebrew", "brew cleanup --prune=all"},
	"go-mod":     {"Go modules", "go clean -modcache"},
	"go-build":   {"Go build", "go clean -cache"},
	"cargo":      {"Cargo", ""},
	"gradle":     {"Gradle", ""},
	"maven":      {"Maven", ""},
	"composer":   {"Composer", "composer clear-cache"},
	"ccache":     {"ccache", "ccache -C"},
	"xcode":      {"Xcode DerivedData", ""},
	"cocoapods":  {"CocoaPods", "pod cache clean --all"},
	"pub":        {"Flutter pub", "flutter pub cache clean"},
	"jetbrains":  {"JetBrains", ""},
}

// cleanupGroups are the Cleanup page's groups, a tab each after All.
var cleanupGroups = []cleanup.Group{cleanup.Caches, cleanup.Copies, cleanup.Docker, cleanup.Projects, cleanup.Models}

// candidates are what the last scan and count offer to clean up; nil
// before the first scan.
func (a *App) candidates(c *ui.Context) []cleanup.Candidate {
	var lines map[int]versions.Line
	if a.NodeVersions != nil {
		lines = a.NodeVersions.Lines
	}
	return cleanup.Find(a.Snapshot, a.Sizes, a.Projects, lines, c.Now())
}

// candidateKey tells a candidate apart across scans, for the user's
// choice of it.
func candidateKey(cand cleanup.Candidate) string {
	return string(cand.Kind) + "\x00" + cand.ID + "\x00" + strings.Join(cand.Paths, "\x00")
}

// chosen reports whether the candidate is chosen: as the user set it, or
// as Hearth offers it.
func (a *App) chosen(cand cleanup.Candidate) bool {
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

func (a *App) cleanupView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		t := c.Theme()
		pal := Colors(t)
		cands := a.candidates(c)
		var selected []cleanup.Candidate
		for _, cand := range cands {
			if a.chosen(cand) {
				selected = append(selected, cand)
			}
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
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
						found, chosen = formatBytes(freeable(cands)), formatBytes(freeable(selected))
					}
					stateCard(c, pal.TextFaint, locales.CleanupFound(), found, locales.CleanupCandidates(len(cands)))
					stateCard(c, Accent, locales.CleanupSelected(), chosen, locales.CleanupCandidates(len(selected)))
					counted, caption := "—", locales.CleanupCounting()
					if a.Sizes != nil && !a.Sizes.Taken.IsZero() {
						counted, caption = ago(c, a.Sizes.Taken), locales.CleanupCountedCaption()
					}
					if a.Counting {
						caption = locales.CleanupCounting()
					}
					stateCard(c, t.Success, locales.CleanupCounted(), counted, caption)
				})
				groups := cleanupGroups
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
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("cleanup-filter"), &a.cleanupFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Width(200)
				})
				columns(c, 12, func() {
					switch {
					case a.Snapshot == nil || a.Sizes == nil:
						toolNote(c, locales.CleanupWaiting())
						return
					case len(cands) == 0:
						toolNote(c, locales.CleanupEmpty())
						return
					}
					filter := strings.ToLower(a.cleanupFilter)
					ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
						shown := 0
						for _, g := range groups {
							if a.cleanupTab > 0 && groups[a.cleanupTab-1] != g {
								continue
							}
							var rows []cleanup.Candidate
							var total int64
							for _, cand := range cands {
								if cand.Kind.Group() == g && strings.Contains(strings.ToLower(a.candidateTitle(cand)+" "+a.candidateCaption(cand)), filter) {
									rows = append(rows, cand)
									total += cand.Frees()
								}
							}
							if len(rows) == 0 {
								continue
							}
							ui.Row(c).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
								ui.Text(c, strings.ToUpper(cleanupGroupTitle(g))).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
								ui.Text(c, formatBytes(total)).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
							})
							for _, cand := range rows {
								shown++
								a.candidateRow(c, cand)
							}
						}
						if shown == 0 {
							cardNote(c, locales.CleanupNone())
						}
					})
					toolNote(c, locales.CleanupReadOnly())
				}, func() {
					var restorable int64
					for _, cand := range cands {
						if cand.Selected {
							restorable += cand.Frees()
						}
					}
					listCard(c, locales.CleanupAtOnce(), "", func() {
						size := "—"
						if a.Sizes != nil {
							size = formatBytes(restorable)
						}
						listRow(c, locales.CleanupAtOnceRow(), false, locales.CleanupAtOnceCaption(size), func() {})
					})
					listCard(c, locales.CleanupYourChoice(), "", func() {
						listRow(c, locales.CleanupCopiesRow(), false, locales.CleanupCopiesCaption(), func() {})
						listRow(c, locales.CleanupDockerRow(), false, locales.CleanupDockerCaption(), func() {})
						listRow(c, locales.CleanupGroupProjects(), false, locales.CleanupProjectsCaption(), func() {})
						listRow(c, locales.CleanupModelsRow(), false, locales.CleanupModelsCaption(), func() {})
					})
					listCard(c, locales.CleanupNotOffered(), "", func() {
						listRow(c, locales.CleanupVolumes(), false, locales.CleanupVolumesCaption(), func() {})
						listRow(c, locales.CleanupSystem(), false, locales.CleanupSystemCaption(), func() {})
						listRow(c, locales.CleanupProjectVersions(), false, locales.CleanupProjectVersionsCaption(), func() {})
					})
				})
			})
		})
	})
}

// candidateRow is a candidate in the table: its check box, what it is and
// why Hearth offers it, and the space it frees.
func (a *App) candidateRow(c *ui.Context, cand cleanup.Candidate) {
	pal := Colors(c.Theme())
	key := candidateKey(cand)
	ui.Row(c.Key(key)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		on := a.chosen(cand)
		title := a.candidateTitle(cand)
		if ui.Checkbox(c.Key("check"), &on, "").Label(title).Changed() {
			if a.cleanupChoice == nil {
				a.cleanupChoice = map[string]bool{}
			}
			a.cleanupChoice[key] = on
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			clipped(c, title).FontSize(13)
			clipped(c, a.candidateCaption(cand)).FontSize(10.5).TextColor(pal.TextFaint)
		})
		size := "—"
		if cand.Known {
			size = formatUsage(inventory.Usage{Bytes: cand.Frees(), Partial: cand.Usage.Partial})
		}
		ui.Text(c, size).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
	})
}

// candidateTitle says what a candidate is: "Bun cache", "Node.js 23.6.0 ·
// brew", "old-landing: node_modules".
func (a *App) candidateTitle(cand cleanup.Candidate) string {
	switch cand.Kind {
	case cleanup.Cache:
		return locales.CleanupCache(cacheName(cand.ID))
	case cleanup.ExtraCopy:
		in := cand.Copies[0]
		return strings.TrimSpace(toolName(cand.ID)+" "+in.Version) + " · " + sourceName(in.Source)
	case cleanup.Leftover:
		return cand.ID
	case cleanup.UnneededPackage:
		return cand.ID + " · " + cand.Manager
	case cleanup.BuildCache:
		return "Build Cache"
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
	}
	return cand.ID
}

// candidateCaption says why Hearth offers a candidate, or for a cache the
// command its tool clears it with.
func (a *App) candidateCaption(cand cleanup.Candidate) string {
	var parts []string
	switch cand.Kind {
	case cleanup.Cache:
		if cmd := caches[cand.ID].command; cmd != "" {
			return cmd
		}
		return tilde(cand.Paths[0], a.Home)
	case cleanup.BuildCache:
		parts = append(parts, "docker builder prune")
	case cleanup.UnusedImages:
		parts = append(parts, locales.CleanupImageCount(cand.Count))
	case cleanup.StoppedContainers:
		parts = append(parts, locales.CleanupContainerCount(cand.Count))
	case cleanup.ProjectInstalls:
		parts = append(parts, tilde(cand.Project, a.Home))
	case cleanup.OldExtensions:
		parts = append(parts, locales.EditorsFolders(cand.Count))
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
			parts = append(parts, locales.ReasonNotSetUp(cand.ID), locales.CleanupVersions(len(cand.Copies), toolName(cand.Copies[0].Tool)))
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
	if c, ok := caches[id]; ok {
		return c.name
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
	n := freeable(a.candidates(c))
	if n == 0 {
		return nil
	}
	return func() {
		ui.Text(c, formatBytes(n)).Font(FontMono).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
	}
}
