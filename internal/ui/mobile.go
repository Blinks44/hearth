package ui

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// mobilePage is the page of iOS and Android development.
const mobilePage = "/tools/mobile"

// mobileCaches are the caches of mobile development, by disk's cache IDs.
var mobileCaches = []string{"xcode", "cocoapods", "gradle", "pub"}

// mobileRow is a row of the mobile table: a name, a line about it and
// the space it takes, when counted.
type mobileRow struct {
	name, about string
	path        string
	size        int64
	sized       bool
	xcode       *inventory.XcodeApp
}

// mobileGroup is a section of the table, a tab each.
type mobileGroup struct {
	title string
	rows  []mobileRow
}

// mobileState is what the mobile detector found; empty before a scan.
func (a *App) mobileState() inventory.MobileState {
	if st, _ := a.Snapshot.Tool("mobile"); st.Mobile != nil {
		return *st.Mobile
	}
	return inventory.MobileState{}
}

// mobileCount is the sidebar's number: the Xcodes, an Android SDK with a
// platform and Flutter found.
func (a *App) mobileCount() int {
	m := a.mobileState()
	n := len(m.Xcodes)
	if m.Android != nil && len(m.Android.Platforms) > 0 {
		n++
	}
	if m.Flutter != nil {
		n++
	}
	return n
}

// mobileFound reports whether the Mobile page has any detected installation,
// runtime, SDK, or CocoaPods to display (so it is not hidden in the sidebar).
func (a *App) mobileFound() bool {
	m := a.mobileState()
	return len(m.Xcodes) > 0 || len(m.Runtimes) > 0 || m.CocoaPods != nil || m.Android != nil || m.Flutter != nil
}

// pathSize is the space the last count found at path.
func (a *App) pathSize(path string) (int64, bool) {
	if a.Sizes == nil {
		return 0, false
	}
	u, ok := a.Sizes.Paths[path]
	return u.Bytes, ok
}

// cachePath is where a cache of the last count is, "" when it is not on
// disk.
func (a *App) cachePath(id string) string {
	if a.Sizes != nil {
		for _, c := range a.Sizes.Caches {
			if c.ID == id {
				return c.Path
			}
		}
	}
	return ""
}

// newestRuntimes are the newest runtime's version of each platform.
func newestRuntimes(rts []inventory.SimRuntime) map[string]string {
	newest := map[string]string{}
	for _, rt := range rts {
		if cur, ok := newest[rt.Platform]; !ok || compareVersions(rt.Version, cur) > 0 {
			newest[rt.Platform] = rt.Version
		}
	}
	return newest
}

// compareVersions compares dotted versions by their numbers: 18.2 > 17.5.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return cmp.Compare(x, y)
		}
	}
	return 0
}

// mobileGroups are the table's groups: iOS, Android and Flutter, each
// when something of it is found.
func (a *App) mobileGroups(m inventory.MobileState) []mobileGroup {
	row := func(name, about, path string) mobileRow {
		r := mobileRow{name: name, about: about, path: path}
		if path != "" {
			r.size, r.sized = a.pathSize(path)
		}
		return r
	}
	cacheRow := func(name, id string, parts ...string) (mobileRow, bool) {
		path := a.cachePath(id)
		if path == "" {
			return mobileRow{}, false
		}
		return row(name, strings.Join(append(parts, locales.MobileRestorable()), " · "), path), true
	}
	var groups []mobileGroup

	var ios []mobileRow
	for _, x := range m.Xcodes {
		app := x
		parts := []string{locales.SourceInstaller()}
		if app.AppStore {
			parts[0] = "App Store"
		}
		if app.Selected {
			parts = append(parts, locales.MobileSelected())
		}
		r := row("Xcode "+app.Version, strings.Join(parts, " · "), app.Path)
		r.xcode = &app
		ios = append(ios, r)
	}
	newest := newestRuntimes(m.Runtimes)
	runtimes := slices.Clone(m.Runtimes)
	slices.SortStableFunc(runtimes, func(a, b inventory.SimRuntime) int {
		return cmp.Or(strings.Compare(a.Platform, b.Platform), compareVersions(b.Version, a.Version))
	})
	for _, rt := range runtimes {
		about := locales.MobileNewest(rt.Platform)
		if rt.Version != newest[rt.Platform] {
			about = locales.MobileOlder(rt.Platform + " " + newest[rt.Platform])
		}
		ios = append(ios, row(locales.MobileSimulator(rt.Platform, rt.Version), about, rt.Path))
	}
	if r, ok := cacheRow("DerivedData", "xcode", locales.MobileBuildCache()); ok {
		ios = append(ios, r)
	}
	if pod := m.CocoaPods; pod != nil {
		r := row(strings.TrimSpace("CocoaPods "+pod.Version), sourceName(pod.Source)+" · "+locales.MobileSpecsCache(), a.cachePath("cocoapods"))
		ios = append(ios, r)
	}
	if len(ios) > 0 {
		groups = append(groups, mobileGroup{"iOS · Xcode", ios})
	}

	var android []mobileRow
	if sdk := m.Android; sdk != nil {
		from := "SDK"
		if sdk.Studio != "" {
			from = "Android Studio"
		}
		where := locales.MobileDefaultDir()
		if sdk.Var != "" {
			where = locales.MobileVarSet(sdk.Var)
		}
		for _, p := range sdk.Platforms {
			android = append(android, row(locales.MobilePlatform(p.API), from+" · "+where, p.Path))
		}
		if len(sdk.Platforms) == 0 {
			android = append(android, row("Android SDK", strings.Join([]string{from, locales.MobileNoPlatforms()}, " · "), ""))
		}
		for _, img := range sdk.Images {
			about := locales.MobileNoAVD()
			if len(img.AVDs) > 0 {
				about = "AVD " + strings.Join(img.AVDs, ", ")
			}
			android = append(android, row(locales.MobileImage(img.API), strings.Join(strings.Split(img.Name, "/")[1:], " · ")+" · "+about, img.Path))
		}
		if r, ok := cacheRow(locales.MobileGradleCache(), "gradle", tilde(a.cachePath("gradle"), a.Home)); ok {
			android = append(android, r)
		}
	}
	if len(android) > 0 {
		groups = append(groups, mobileGroup{"Android", android})
	}

	var flutter []mobileRow
	if f := m.Flutter; f != nil {
		parts := []string{sourceName(f.Source)}
		if f.Version == "" {
			parts = append(parts, locales.UnknownVersion())
		}
		if f.Channel != "" {
			parts = append(parts, locales.MobileChannel(f.Channel))
		}
		flutter = append(flutter, row(strings.TrimSpace("Flutter "+f.Version), strings.Join(parts, " · "), f.Root))
	}
	if r, ok := cacheRow(locales.MobilePubCache(), "pub", tilde(a.cachePath("pub"), a.Home)); ok && m.Flutter != nil {
		flutter = append(flutter, r)
	}
	if len(flutter) > 0 {
		groups = append(groups, mobileGroup{"Flutter", flutter})
	}
	return groups
}

func (a *App) mobileView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		a.header(c, p.title(), p.subtitle())
		t := c.Theme()
		pal := Colors(t)
		m := a.mobileState()
		groups := a.mobileGroups(m)
		var total int64
		totalKnown := a.Sizes != nil
		for _, g := range groups {
			for _, r := range g.rows {
				if r.path != "" && !r.sized {
					totalKnown = false
				}
				total += r.size
			}
		}
		var removable []cleanup.Candidate
		var frees int64
		freesKnown := a.Sizes != nil
		for _, cand := range a.candidates(c) {
			if cand.Kind == cleanup.Cache && slices.Contains(mobileCaches, cand.ID) {
				removable = append(removable, cand)
				if !cand.Known {
					freesKnown = false
				}
				frees += cand.Frees()
			}
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.Subtle).BorderWidth(1).BorderColor(pal.Line).Center().Children(func() {
						ui.Icon(c, Icon("smartphone")).Size(24, 24).TextColor(t.TextMuted)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, p.title()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.MobileLead()).FontSize(12.5).TextColor(t.TextMuted).SingleLine()
					})
				}, func() {
					if len(m.Xcodes) > 0 {
						x := m.Xcodes[0]
						caption := locales.MobileNotSelected()
						dot := t.Warning
						for _, o := range m.Xcodes {
							if o.Selected {
								x, caption, dot = o, "xcode-select", t.Success
							}
						}
						stateCard(c, dot, "Xcode", x.Version, caption)
					}
					switch sdk := m.Android; {
					case sdk == nil:
						stateCard(c, pal.TextFaint, "Android SDK", locales.MobileNotFound(), "")
					case len(sdk.Platforms) == 0:
						stateCard(c, pal.TextFaint, "Android SDK", "—", locales.MobileNoPlatforms())
					default:
						caption := sdk.Var
						if caption == "" {
							caption = locales.MobileDefaultDir()
						}
						stateCard(c, t.Success, "Android SDK", "API "+strconv.Itoa(sdk.Platforms[0].API), caption)
					}
					if m.Flutter != nil {
						ver := m.Flutter.Version
						if ver == "" {
							ver = "—"
						}
						caption := m.Flutter.Channel
						if caption == "" {
							caption = "flutter"
						}
						stateCard(c, t.Success, "Flutter", ver, caption)
					}
					totalText := "…"
					if totalKnown {
						totalText = formatBytes(total)
					}
					freesText := locales.MobileCounting()
					spaceDot := pal.TextFaint
					if freesKnown {
						freesText = locales.MobileToCleanup(formatBytes(frees))
						if frees > 0 {
							spaceDot = t.Warning
						}
					}
					stateCard(c, spaceDot, locales.MobileTake(), totalText, freesText)
				})
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					n := 0
					segs := []segment{{locales.TabAll(), ""}}
					for _, g := range groups {
						n += len(g.rows)
						segs = append(segs, segment{strings.Split(g.title, " · ")[0], strconv.Itoa(len(g.rows))})
					}
					segs[0].count = strconv.Itoa(n)
					if a.mobileTab > len(groups) {
						a.mobileTab = 0
					}
					segments(c, "mobile-tabs", &a.mobileTab, segs)
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("mobile-filter"), &a.mobileFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Height(controlHeight).Width(200)
				})
				columns(c, 12, func() {
					switch {
					case a.Snapshot == nil:
						if a.Scanning {
							toolNote(c, locales.Checking())
						} else {
							toolNote(c, locales.NotChecked())
						}
					case len(groups) == 0:
						toolNote(c, locales.MobileNone())
					default:
						a.mobileTable(c, groups)
					}
				}, func() {
					if a.Snapshot != nil {
						mobileInstall(c, m)
					}
					a.mobileEnv(c, m)
					a.mobileRemovable(c, m, removable)
				})
			})
		})
	})
}

// mobileTable is the groups of the chosen tab, their rows filtered.
func (a *App) mobileTable(c *ui.Context, groups []mobileGroup) {
	pal := Colors(c.Theme())
	filter := strings.ToLower(strings.TrimSpace(a.mobileFilter))
	shown := 0
	ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		for i, g := range groups {
			if a.mobileTab > 0 && a.mobileTab-1 != i {
				continue
			}
			rows := slices.DeleteFunc(slices.Clone(g.rows), func(r mobileRow) bool {
				return filter != "" && !strings.Contains(strings.ToLower(r.name+" "+r.about+" "+r.path), filter)
			})
			if len(rows) == 0 {
				continue
			}
			var size int64
			sized := true
			hasPath := false
			for _, r := range rows {
				if r.path != "" {
					hasPath = true
					if !r.sized {
						sized = false
					}
				}
				size += r.size
			}
			groupSize := "—"
			switch {
			case hasPath && !sized:
				groupSize = "…"
			case hasPath:
				groupSize = formatBytes(size)
			}
			ui.Row(c).Gap(12).Padding(6, 16).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
				ui.Text(c, strings.ToUpper(g.title)).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
				ui.Text(c, groupSize).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
				if a.Shell != nil {
					ui.Box(c).Width(50).Shrink(0)
				}
			})
			for _, r := range rows {
				size := "—" // nothing on disk to count
				switch {
				case r.sized:
					size = formatBytes(r.size)
				case r.path != "":
					size = "…"
				}
				rowKey := g.title + ":" + r.name + ":" + r.path
				ui.Row(c.Key(rowKey)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
						clipped(c, r.name).FontSize(13)
						clipped(c, r.about).FontSize(10.5).TextColor(pal.TextFaint)
					})
					if r.xcode != nil && !r.xcode.Selected {
						if plan, ok := ops.SwitchXcodePlan(*r.xcode, a.detect("mobile")); ok {
							ui.Button(c.Key("xcode-use:"+r.xcode.Path), locales.MobileXcodeUse()).Shrink(0).OnClick(func() {
								a.Propose(plan)
							})
						}
					}
					ui.Text(c, size).Font(FontMono).FontSize(12).Shrink(0).SingleLine()
					a.pathActions(c, rowKey, r.path)
				})
			}
			shown += len(rows)
		}
		if shown == 0 {
			cardNote(c, locales.NothingMatches())
		}
	})
}

// mobileInstall links the official install pages of the tools not found:
// their installers are large and ask along the way, so Hearth does not
// run them.
func mobileInstall(c *ui.Context, m inventory.MobileState) {
	type link struct{ name, about, url string }
	var links []link
	if xcodeHere && len(m.Xcodes) == 0 {
		links = append(links, link{"Xcode", locales.MobileInstallXcode(), "https://apps.apple.com/app/xcode/id497799835"})
	}
	if m.Android == nil {
		links = append(links, link{"Android Studio", locales.MobileInstallStudio(), "https://developer.android.com/studio"})
	}
	if m.Flutter == nil {
		links = append(links, link{"Flutter", locales.MobileInstallFlutter(), "https://docs.flutter.dev/get-started/install"})
	}
	if len(links) == 0 {
		return
	}
	listCard(c, locales.MobileInstall(), "", func() {
		for _, l := range links {
			listRow(c, l.name, false, l.about, func() {
				ui.Link(c.Key("install:"+l.name), locales.MobileInstallOpen()+" ↗", l.url).FontSize(12.5).Shrink(0)
			})
		}
		cardNote(c, locales.MobileInstallNote())
	})
}

// mobileEnv is what decides which SDK and JDK the builds take.
func (a *App) mobileEnv(c *ui.Context, m inventory.MobileState) {
	listCard(c, locales.MobileEnv(), "", func() {
		if sdk := m.Android; sdk != nil {
			state := locales.MobileDirExists()
			if sdk.Missing {
				state = locales.MobileDirMissing()
			}
			root := sdk.Root
			if sdk.Var != "" {
				listRow(c, sdk.Var, true, tilde(root, a.Home)+" · "+state, func() {
					if root != "" {
						a.pathActions(c, "mobile-android", root)
					}
				})
			} else {
				listRow(c, "ANDROID_HOME", true, locales.MobileUnset()+" · "+tilde(root, a.Home), func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						if !sdk.Missing && root != "" && a.BuildPlan != nil {
							ui.Button(c.Key("mobile-android-set"), locales.EnvVarSetButton()).Shrink(0).OnClick(func() {
								a.setEnvVar("ANDROID_HOME", root)
							})
						}
						if root != "" {
							a.pathActions(c, "mobile-android", root)
						}
					})
				})
			}
		}
		javaHome := locales.MobileUnset()
		javaRaw := ""
		if a.Snapshot != nil {
			for _, v := range a.Snapshot.Variables {
				if v.Name == "JAVA_HOME" && v.Terminal != "" {
					javaRaw = v.Terminal
					javaHome = tilde(v.Terminal, a.Home)
				}
			}
		}
		activeJava := a.activeJavaHome()
		listRow(c, locales.MobileJavaHome(), false, javaHome, func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				if activeJava != "" && a.BuildPlan != nil {
					if javaRaw == "" {
						ui.Button(c.Key("mobile-javahome-set"), locales.EnvVarSetButton()).Shrink(0).OnClick(func() {
							a.setEnvVar("JAVA_HOME", activeJava)
						})
					} else if activeJava != a.javaHome() {
						ui.Button(c.Key("mobile-javahome-fix"), locales.EnvVarFixButton()).Shrink(0).OnClick(func() {
							a.setEnvVar("JAVA_HOME", activeJava)
						})
					}
				}
				if javaRaw != "" {
					a.pathActions(c, "mobile-javahome", javaRaw)
				}
			})
		})
		devDir := m.DeveloperDir
		if devDir == "" && a.Snapshot != nil {
			if st, _ := a.Snapshot.Tool("cpp"); st.Cpp != nil && st.Cpp.Base != nil && len(m.Xcodes) > 0 {
				devDir = st.Cpp.Base.Path
			}
		}
		if devDir != "" {
			listRow(c, "xcode-select", true, devDir, func() {
				a.pathActions(c, "mobile-xcode-select", devDir)
			})
		}
	})
}

// mobileRemovable is what Cleanup offers of mobile development, the older
// simulators, unused Android system images, and the way to Cleanup.
func (a *App) mobileRemovable(c *ui.Context, m inventory.MobileState, caches []cleanup.Candidate) {
	t := c.Theme()
	listCard(c, locales.MobileRemovable(), "", func() {
		newest := newestRuntimes(m.Runtimes)
		shown := 0
		for _, rt := range m.Runtimes {
			if rt.Version == newest[rt.Platform] {
				continue
			}
			runtime := rt
			caption := rt.Platform + " " + rt.Version
			if size, ok := a.pathSize(rt.Path); ok {
				caption = formatBytes(size) + " · " + caption
			}
			listRow(c, locales.MobileOldSimulator(), false, caption, func() {
				if a.BuildPlan != nil {
					ui.Button(c.Key("remove-sim:"+runtime.ID+":"+runtime.Path+":"+runtime.Platform+":"+runtime.Version), locales.RemoveEllipsis()).Shrink(0).
						OnClick(func() { a.removeSimRuntime(runtime) })
				}
			})
			shown++
		}
		if sdk := m.Android; sdk != nil {
			for _, img := range sdk.Images {
				if len(img.AVDs) > 0 {
					continue
				}
				part := img
				caption := img.Name
				if size, ok := a.pathSize(img.Path); ok {
					caption = formatBytes(size) + " · " + caption
				}
				listRow(c, locales.MobileUnusedImage(), false, caption, func() {
					if a.BuildPlan != nil {
						ui.Button(c.Key("remove-img:"+part.Path), locales.RemoveEllipsis()).Shrink(0).
							OnClick(func() { a.removeAndroidImage(sdk.Root, part) })
					}
				})
				shown++
			}
		}
		for _, cand := range caches {
			listRow(c, cacheName(cand.ID), false, formatBytes(cand.Frees())+" · "+locales.MobileRestorable(), func() {})
			shown++
		}
		if shown == 0 {
			cardNote(c, locales.MobileNothing())
			return
		}
		if len(caches) == 0 {
			return
		}
		b := ui.ButtonBase(c.Key("mobile-cleanup")).Column().AlignItems(ui.Start).Gap(1).Padding(9, 12, 9, 14).BorderWidth(1, 0, 0, 0).BorderColor(Colors(t).Line).
			Label(locales.MobileOpenCleanup()).OnClick(func() {
			a.cleanupTab = slices.Index(cleanupGroups, cleanup.Caches) + 1
			a.router.Push(cleanupPage)
		})
		if b.Hovered() {
			b.Background(Colors(t).Subtle)
		}
		b.Children(func() {
			ui.Text(c, locales.MobileOpenCleanup()).FontSize(13).TextColor(Accent).SingleLine()
			ui.Text(c, locales.MobileOpenCleanupCaption()).FontSize(11.5).TextColor(Colors(t).TextFaint).SingleLine()
		})
	})
}

// removeSimRuntime proposes removing an older simulator runtime via simctl.
func (a *App) removeSimRuntime(rt inventory.SimRuntime) {
	if a.BuildPlan == nil {
		return
	}
	detect := a.detect("mobile")
	runner := a.Runner
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.RemoveSimRuntimePlan(context.Background(), runner, rt, env, detect)
	}, a.proposeOrSay)
}

// removeAndroidImage proposes removing an unused Android system image.
func (a *App) removeAndroidImage(sdkRoot string, img inventory.AndroidPart) {
	if a.BuildPlan == nil {
		return
	}
	detect := a.detect("mobile")
	a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		return ops.RemoveAndroidImagePlan(sdkRoot, img, env, detect)
	}, a.proposeOrSay)
}
