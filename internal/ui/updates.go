package ui

import (
	"cmp"
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/updates"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// updatesPage is the Updates page's path.
const updatesPage = "/updates"

// updateRow is a row of the Updates page: a newer version of a tool, or a
// package a manager can update.
type updateRow struct {
	key, title, caption, from, to string
	// version is the tool's update; nil for a package.
	version *updates.Update
	m       inventory.PackageManager
	pkg     inventory.SystemPackage
	// choosable reports that Hearth updates it; offered that it is
	// checked until the user says otherwise.
	choosable, offered bool
}

// updateGroup is a titled group of the Updates table.
type updateGroup struct {
	title    string
	versions bool
	rows     []updateRow
}

// updateLists are the version lists the updates of tools come from, as
// they last came.
func (a *App) updateLists() updates.Lists {
	return updates.Lists{
		Node: a.NodeVersions, JS: a.JSVersions, Go: a.GoVersions, Python: a.PythonVersions, PHP: a.PHPVersions, Ruby: a.RubyVersions,
		Java: a.JavaVersions, Rust: a.RustVersions, Dotnet: a.DotnetVersions, Bun: a.BunLatest, Agents: a.AgentVersions, Mise: a.MiseOutdated,
	}
}

// managersChecked are the package managers of the last scan, with what
// the check of their updates found over it.
func (a *App) managersChecked() []inventory.PackageManager {
	st, ok := a.Snapshot.Tool("packages")
	if !ok {
		return nil
	}
	out := make([]inventory.PackageManager, len(st.PackageManagers))
	for i, m := range st.PackageManagers {
		if checked, ok := a.PackageUpdates[m.Path]; ok {
			m = checked
		}
		out[i] = m
	}
	return out
}

// updateGroups are what can be updated: the versions of tools, fixes then
// new lines, then the packages by manager, Homebrew's formulas apart from
// its apps.
func (a *App) updateGroups(c *ui.Context) []updateGroup {
	patches := updateGroup{title: locales.UpdatesGroupPatches(), versions: true}
	lines := updateGroup{title: locales.UpdatesGroupLines(), versions: true}
	managers := a.managersChecked()
	for _, u := range updates.Find(a.Snapshot, a.Projects, a.updateLists(), c.Now()) {
		prefix, title := "version", toolName(u.Tool)
		if u.Agent {
			prefix, title = "agent", agentTitle(u.Tool)
		}
		r := updateRow{key: prefix + "\x00" + u.Tool + "\x00" + u.Copy.Path + "\x00" + u.Version, version: &u, to: u.Version}
		_, r.choosable = a.updatePlan(u)
		if u.Kind == updates.Patch {
			// Its package's row updates it, to the version its manager has.
			if updates.Held(u.Copy, managers) {
				continue
			}
			r.title, r.from, r.offered = title, u.Copy.Version, r.choosable
			r.caption = sourceName(u.Copy.Source) + " · " + locales.UpdatesPatchCaption()
			switch {
			case r.choosable:
			case u.Copy.Source.Manager == "mise" && a.MiseOutdated == nil:
				// mise has not said yet whether the release is in range.
			case u.Copy.Source.Manager == "mise":
				// mise outdated sees no release in the config's range.
				r.caption = sourceName(u.Copy.Source) + " · " + locales.UpdatesMiseHeld()
			default:
				r.caption = sourceName(u.Copy.Source) + " · " + locales.UpdatesElsewhere()
			}
			patches.rows = append(patches.rows, r)
			continue
		}
		r.title = title
		if u.LTS != "" {
			r.title += " LTS"
		}
		switch {
		case !r.choosable:
			r.caption = locales.UpdatesElsewhere()
		case a.newMise(u):
			r.caption = locales.UpdatesLineMise()
		default:
			r.caption = locales.UpdatesLineCaption()
		}
		lines.rows = append(lines.rows, r)
	}
	groups := []updateGroup{patches, lines}
	for _, m := range managers {
		formulas := updateGroup{title: m.Name}
		apps := updateGroup{title: locales.UpdatesGroupApps(m.Name)}
		if m.ID == "brew" {
			formulas.title = locales.UpdatesGroupFormulas(m.Name)
		}
		for _, pkg := range m.Packages {
			if pkg.Available == "" {
				continue
			}
			r := updateRow{key: m.Path + "\x00" + packageKey(pkg), title: pkg.Name, from: flatpakBuild(m.ID, pkg.Version), to: flatpakBuild(m.ID, pkg.Available), m: m, pkg: pkg}
			r.choosable = ops.PackageManaged(m.ID)
			// Only what the user asked for: others need the dependencies at
			// the versions they have.
			r.offered = r.choosable && pkg.Kind != "cask" && m.ReasonsKnown && !pkg.Dependency
			switch {
			case !r.choosable:
				r.caption = locales.UpdatesNotThrough(m.Name)
			case pkg.Kind == "cask":
				r.caption = locales.UpdatesCaskCaption()
			case m.ID == "pacman":
				r.caption = locales.PkgUpgradeSystem()
			case m.ReasonsKnown && pkg.Dependency:
				r.caption = locales.UpdatesDependency()
			case m.ReasonsKnown:
				r.caption = locales.UpdatesAsked()
			}
			if pkg.Kind == "cask" {
				apps.rows = append(apps.rows, r)
			} else {
				formulas.rows = append(formulas.rows, r)
			}
		}
		groups = append(groups, formulas, apps)
	}
	out := groups[:0]
	for _, g := range groups {
		if len(g.rows) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// updateChosen reports that r is checked: as the user set it, or as
// Hearth offers it.
func (a *App) updateChosen(r updateRow) bool {
	if !r.choosable {
		return false
	}
	if v, ok := a.updateChoice[r.key]; ok {
		return v
	}
	return r.offered
}

// updateCounts are how many versions and packages the groups hold.
func updateCounts(groups []updateGroup) (versions, packages int) {
	for _, g := range groups {
		if g.versions {
			versions += len(g.rows)
		} else {
			packages += len(g.rows)
		}
	}
	return versions, packages
}

func (a *App) visibleUpdateGroups(groups []updateGroup) []updateGroup {
	filter := strings.ToLower(strings.TrimSpace(a.updatesFilter))
	var out []updateGroup
	for _, g := range groups {
		if a.updatesTab == 1 && !g.versions || a.updatesTab == 2 && g.versions {
			continue
		}
		var rows []updateRow
		for _, r := range g.rows {
			if updateRowMatches(g, r, filter) {
				rows = append(rows, r)
			}
		}
		if len(rows) > 0 {
			out = append(out, updateGroup{title: g.title, versions: g.versions, rows: rows})
		}
	}
	return out
}

func updateRowMatches(g updateGroup, r updateRow, filter string) bool {
	if filter == "" {
		return true
	}
	parts := []string{r.title, r.caption, r.from, r.to, g.title, r.m.ID, r.m.Name}
	if r.version != nil {
		parts = append(parts, r.version.Tool, r.version.Copy.Source.Manager)
	}
	return strings.Contains(strings.ToLower(strings.Join(parts, " ")), filter)
}

func (a *App) updatesView(c *ui.Context, p page) {
	ui.Column(c).Fill().Children(func() {
		t := c.Theme()
		pal := Colors(t)
		groups := a.updateGroups(c)
		visibleGroups := a.visibleUpdateGroups(groups)
		var chosen []updateRow
		for _, g := range visibleGroups {
			for _, r := range g.rows {
				if a.updateChosen(r) {
					chosen = append(chosen, r)
				}
			}
		}
		nv, np := updateCounts(groups)
		a.headerWith(c, p.title(), p.subtitle(), headerExtras{after: func() {
			a.checkUpdatesButton(c)
			a.updateButton(c, chosen)
		}})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Gap(18).Padding(22, 28, 24, 28).Children(func() {
				headRow(c, func() {
					ui.Box(c).Size(48, 48).Radius(12).Background(pal.InfoSoft).Center().Children(func() {
						ui.Icon(c, Icon("circle-arrow-up")).Size(26, 26).TextColor(pal.Info)
					})
					ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
						ui.Text(c, locales.UpdatesTitle()).FontSize(22).FontWeight(600)
						ui.Text(c, locales.UpdatesLead()).FontSize(12).TextColor(t.TextMuted)
					})
				}, func() {
					found, selected := "—", "—"
					if a.Snapshot != nil {
						found, selected = strconv.Itoa(nv+np), strconv.Itoa(len(chosen))
					}
					stateCard(c, pal.TextFaint, locales.CleanupFound(), found, locales.UpdatesFoundCaption(nv, np))
					stateCard(c, Accent, locales.CleanupSelected(), selected, locales.UpdatesSelectedCaption())
					checked, caption := "—", locales.ProblemsCheckedCaption()
					if when := a.updatesChecked(); !when.IsZero() {
						checked = ago(c, when)
					}
					checking := a.updatesChecking()
					if checking {
						stateCardBusy(c, locales.ProblemsChecked(), checked, locales.Checking())
					} else {
						stateCard(c, t.Success, locales.ProblemsChecked(), checked, caption)
					}
				})
				if a.AppUpdateAvailable != "" && a.CheckAppUpdate != nil {
					ui.Row(c.Key("hearth-app-update-banner")).Gap(12).Padding(12, 16).Radius(12).
						Background(pal.AccentSoft).BorderWidth(1).BorderColor(Accent.Alpha(0.28)).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(32, 32).Shrink(0).Radius(8).Background(Accent.Alpha(0.14)).Center().Children(func() {
							ui.Icon(c, Icon("circle-arrow-up")).Size(18, 18).TextColor(Accent)
						})
						ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, locales.AppUpdateBannerTitle(a.AppUpdateAvailable)).FontSize(13.5).FontWeight(600).TextColor(t.Text)
							ui.Text(c, locales.AppUpdateBannerHint()).FontSize(12).TextColor(t.TextMuted)
						})
						ui.PrimaryButton(c.Key("hearth-app-update-btn"), locales.AppUpdateInstallButton()).OnClick(a.CheckAppUpdate)
					})
				}
				toolbar := ui.Row(c).Gap(10).AlignItems(ui.Center)
				if narrow(c) {
					toolbar.Wrap()
				}
				toolbar.Children(func() {
					segments(c, "updates-tabs", &a.updatesTab, []segment{
						{locales.TabAll(), strconv.Itoa(nv + np)},
						{locales.UpdatesVersions(), strconv.Itoa(nv)},
						{locales.UpdatesPackages(), strconv.Itoa(np)},
					})
					ui.Box(c).Grow(1).Height(1)
					ui.SearchField(c.Key("updates-filter"), &a.updatesFilter).Bind(&a.filterField).Placeholder(locales.CleanupFilter()).Height(controlHeight).Width(200)
				})
				checking := a.updatesChecking()
				errs := a.updateErrors()
				columns(c, 12, func() {
					if a.Snapshot == nil {
						toolNote(c, locales.UpdatesWaiting())
						return
					}
					if checking {
						toolNote(c, locales.UpdatesChecking())
					}
					if !a.VersionLists {
						toolNote(c, locales.UpdatesOffline())
					}
					for _, msg := range errs {
						warningRow(c, msg)
					}
					if nv+np == 0 {
						switch {
						case checking || !a.VersionLists:
							return
						case len(errs) > 0:
							toolNote(c, locales.UpdatesCheckFailed())
							return
						default:
							toolNote(c, locales.UpdatesEmpty())
							return
						}
					}
					ui.Column(c).Radius(12).Clip().BorderWidth(1).BorderColor(pal.Divider).Children(func() {
						shown := 0
						for _, g := range visibleGroups {
							ui.Row(c.Key("group:"+g.title)).Gap(12).Padding(6, 16).AlignItems(ui.Center).Background(pal.Subtle).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
								a.updateGroupCheckbox(c, g.title, g.rows)
								ui.Text(c, strings.ToUpper(g.title)).Grow(1).FontSize(10.5).FontWeight(500).TextColor(pal.TextFaint).SingleLine()
								ui.Text(c, strconv.Itoa(len(g.rows))).Font(FontMono).FontSize(10.5).TextColor(pal.TextFaint)
							})
							for _, r := range g.rows {
								shown++
								a.updateRowView(c, r)
							}
						}
						if shown == 0 {
							cardNote(c, locales.UpdatesNone())
						}
					})
				}, func() {
					listCard(c, locales.CleanupAtOnce(), "", func() {
						listRowWrapped(c, locales.UpdatesAtOnceRow(), locales.UpdatesAtOnceCaption(), func() {})
					})
					listCard(c, locales.CleanupYourChoice(), "", func() {
						listRowWrapped(c, locales.UpdatesLinesRow(), locales.UpdatesLineCaption(), func() {})
						listRowWrapped(c, locales.UpdatesAppsRow(), locales.UpdatesAppsCaption(), func() {})
						listRowWrapped(c, locales.UpdatesDepsRow(), locales.UpdatesDepsCaption(), func() {})
					})
					listCard(c, locales.CleanupNotOffered(), "", func() {
						listRowWrapped(c, locales.UpdatesPinnedRow(), locales.UpdatesPinnedCaption(), func() {})
						listRowWrapped(c, locales.UpdatesSystemRow(), locales.UpdatesSystemCaption(), func() {})
						listRowWrapped(c, locales.UpdatesManualRow(), locales.UpdatesManualCaption(), func() {})
					})
				})
			})
		})
	})
}

// updateGroupCheckbox changes only the visible, supported rows of a group.
func (a *App) updateGroupCheckbox(c *ui.Context, title string, rows []updateRow) {
	var keys []string
	selected := 0
	for _, r := range rows {
		if r.choosable {
			keys = append(keys, r.key)
			if a.updateChosen(r) {
				selected++
			}
		}
	}
	state := ui.CheckOff
	if selected > 0 && selected == len(keys) {
		state = ui.CheckOn
	} else if selected > 0 {
		state = ui.CheckMixed
	}
	if ui.CheckboxState(c.Key("all"), &state, "").Label(locales.UpdatesSelectGroup(title)).Disabled(len(keys) == 0).Changed() {
		if a.updateChoice == nil {
			a.updateChoice = map[string]bool{}
		}
		for _, key := range keys {
			a.updateChoice[key] = state == ui.CheckOn
		}
	}
}

// updatesChecked is when updates were last checked, the oldest of the
// package managers' checks, or of the loaded version lists when no package
// manager has a recorded check time; zero when none was.
func (a *App) updatesChecked() time.Time {
	var oldest time.Time
	add := func(t time.Time) {
		if !t.IsZero() && (oldest.IsZero() || t.Before(oldest)) {
			oldest = t
		}
	}
	for _, m := range a.managersChecked() {
		add(m.UpdatesFetched)
	}
	if !oldest.IsZero() {
		return oldest
	}
	if a.NodeVersions != nil {
		add(a.NodeVersions.Fetched)
	}
	if a.JSVersions != nil {
		add(a.JSVersions.Npm.Fetched)
		add(a.JSVersions.Pnpm.Fetched)
		add(a.JSVersions.Yarn.Fetched)
	}
	if a.BunLatest != nil {
		add(a.BunLatest.Fetched)
	}
	if a.GoVersions != nil {
		add(a.GoVersions.Fetched)
	}
	if a.PythonVersions != nil {
		add(a.PythonVersions.Fetched)
	}
	if a.PHPVersions != nil {
		add(a.PHPVersions.Fetched)
	}
	if a.RustVersions != nil {
		add(a.RustVersions.Fetched)
	}
	if a.DotnetVersions != nil {
		add(a.DotnetVersions.Fetched)
	}
	if a.RubyVersions != nil {
		add(a.RubyVersions.Fetched)
	}
	if a.JavaVersions != nil {
		add(a.JavaVersions.Fetched)
	}
	if a.AgentVersions != nil {
		add(a.AgentVersions.Fetched)
	}
	if a.MiseOutdated != nil {
		add(a.MiseOutdated.Fetched)
	}
	return oldest
}

// updatesChecking reports whether any package manager, version list, agent,
// or mise outdated check is currently running.
func (a *App) updatesChecking() bool {
	if a.MiseChecking || a.AgentChecking {
		return true
	}
	for _, busy := range a.PackageChecks {
		if busy {
			return true
		}
	}
	for _, busy := range a.VersionChecks {
		if busy {
			return true
		}
	}
	return false
}

// updateErrors lists update-check failures for installed tools, agents, mise,
// and package managers. Offline notices are omitted when VersionLists
// is turned off, because the page already shows a dedicated offline note.
func (a *App) updateErrors() []string {
	if a.Snapshot == nil {
		return nil
	}
	offline := locales.VersionsOffline()
	var out []string
	add := func(source, msg string) {
		if msg == "" || (!a.VersionLists && msg == offline) {
			return
		}
		out = append(out, source+": "+msg)
	}
	versionErrs := []struct {
		tool, title, err string
	}{
		{"node", "Node.js", a.NodeVersionsErr},
		{"bun", "Bun", a.BunLatestErr},
		{"go", "Go", a.GoVersionsErr},
		{"python", "Python", a.PythonVersionsErr},
		{"php", "PHP", a.PHPVersionsErr},
		{"rust", "Rust", a.RustVersionsErr},
		{"dotnet", ".NET", a.DotnetVersionsErr},
		{"ruby", "Ruby", a.RubyVersionsErr},
		{"java", "Java", a.JavaVersionsErr},
	}
	for _, v := range versionErrs {
		if st, ok := a.Snapshot.Tool(v.tool); ok && len(listed(st.Installations)) > 0 {
			add(v.title, v.err)
		}
	}
	if a.JSVersionsErr != "" {
		for _, tool := range []string{"npm", "pnpm", "yarn"} {
			if st, ok := a.Snapshot.Tool(tool); ok && len(listed(st.Installations)) > 0 {
				add("npm / pnpm / Yarn", a.JSVersionsErr)
				break
			}
		}
	}
	if a.AgentVersionsErr != "" {
		for _, ag := range a.Snapshot.Agents {
			if len(listed(ag.Installations)) > 0 {
				add(locales.AgentsTitle(), a.AgentVersionsErr)
				break
			}
		}
	}
	if a.MiseOutdatedErr != "" {
		add("mise", a.MiseOutdatedErr)
	}
	for _, m := range a.managersChecked() {
		if m.UpdateError != "" {
			add(m.Name, m.UpdateError)
		}
	}
	return out
}

// updateRowAction returns the navigation callback when clicking an update row,
// or nil if the row has no dedicated destination.
func (a *App) updateRowAction(r updateRow) func() {
	if r.version != nil {
		u := *r.version
		if u.Agent {
			return func() { a.router.Push(agentPath(u.Tool)) }
		}
		switch u.Tool {
		case "npm", "pnpm", "yarn":
			return func() { a.router.Push("/tools/jspm") }
		case "kubectl", "helm", "kind":
			return func() { a.router.Push(kubePage) }
		case "node", "bun", "python", "php", "rust", "go", "java", "ruby", "dotnet":
			return func() { a.router.Push("/tools/" + u.Tool) }
		default:
			return func() {
				a.cliTab = 0
				a.cliFilter = u.Tool
				a.router.Push(cliPage)
			}
		}
	}
	if r.m.Path != "" {
		mgrPath, pkgName := r.m.Path, r.pkg.Name
		return func() {
			a.packageManager = mgrPath
			a.packageTab = 0
			a.packageFilter = pkgName
			a.router.Push("/system/packages")
		}
	}
	return nil
}

// updateRowView is an update in the table: its check box when Hearth
// updates it, what it is and how it goes, and from what version to what.
func (a *App) updateRowView(c *ui.Context, r updateRow) {
	t := c.Theme()
	pal := Colors(t)
	ui.Row(c.Key(r.key)).Gap(12).Padding(6, 16).MinHeight(44).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).Children(func() {
		if r.choosable {
			on := a.updateChosen(r)
			if ui.Checkbox(c.Key("check"), &on, "").Label(r.title).Changed() {
				if a.updateChoice == nil {
					a.updateChoice = map[string]bool{}
				}
				a.updateChoice[r.key] = on
			}
		} else {
			// Keeps the titles in line with the checked rows'.
			ui.Box(c).Size(16, 16).Shrink(0)
		}
		// A cask's version adds its download's hash, as 3.29.9,306d153d…:
		// the tooltip has it whole.
		short := func(v string) string { v, _, _ = strings.Cut(v, ","); return v }
		versions, whole := short(r.to), r.to
		if r.from != "" {
			versions, whole = short(r.from)+" → "+versions, r.from+" → "+r.to
		}
		body := func(chevronColor ui.Color) {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				clipped(c, r.title).FontSize(13)
				if r.caption != "" {
					clipped(c, r.caption).FontSize(10.5).TextColor(pal.TextFaint)
				}
			})
			ui.Text(c, versions).Font(FontMono).FontSize(12).Shrink(0).MaxWidth(280).SingleLine().Tooltip(whole)
			if chevronColor.A > 0 {
				ui.Icon(c, Icon("chevron-right")).Size(14, 14).TextColor(chevronColor)
			}
		}
		if open := a.updateRowAction(r); open != nil {
			b := ui.ButtonBase(c.Key("open")).Grow(1).MinWidth(0).Gap(12).Padding(2, 4).Radius(6).AlignItems(ui.Center).Label(locales.UpdatesOpenTool(r.title))
			chev := pal.TextFaint
			if b.Hovered() {
				b.Background(pal.Subtle)
				chev = t.TextMuted
			}
			b.Children(func() { body(chev) })
			if b.Clicked() {
				open()
			}
		} else {
			ui.Row(c).Grow(1).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() { body(ui.Color{}) })
		}
	})
}

// checkUpdatesButton revalidates version lists, agent releases, and
// package-manager updates right now, bypassing the daily cache.
func (a *App) checkUpdatesButton(c *ui.Context) {
	ui.Button(c.Key("updates-check"), locales.UpdatesCheckNow()).
		Height(controlHeight).
		Disabled(a.CheckUpdates == nil || a.updatesChecking() || !a.VersionLists || a.Snapshot == nil).
		OnClick(func() {
			if a.CheckUpdates != nil {
				a.CheckUpdates()
			}
		})
}

// updateButton is the header's button that updates the chosen rows, each
// through the manager that keeps it, in one plan: the versions first, then
// the packages by manager; nothing when none is chosen.
func (a *App) updateButton(c *ui.Context, chosen []updateRow) {
	if len(chosen) == 0 {
		return
	}
	ui.PrimaryButton(c.Key("updates-run"), locales.UpdatesRunChosen(len(chosen))).Height(controlHeight).OnClick(func() {
		var plans []ops.Plan
		var managers []inventory.PackageManager
		byManager := map[string][]inventory.SystemPackage{}
		mise := false
		for _, r := range chosen {
			if r.version != nil {
				mise = mise || a.newMise(*r.version)
				if p, ok := a.updatePlan(*r.version); ok {
					plans = append(plans, p)
				}
				continue
			}
			if _, seen := byManager[r.m.Path]; !seen {
				managers = append(managers, r.m)
			}
			byManager[r.m.Path] = append(byManager[r.m.Path], r.pkg)
		}
		for _, m := range managers {
			if p, ok := a.packagePlan(ops.PackageUpgrade, m, byManager[m.Path]); ok {
				plans = append(plans, p)
			}
		}
		// The choice stays: what was updated leaves the table with the
		// next scan, and a plan cancelled keeps what was checked.
		p, ok := ops.UpdatePlan(plans)
		switch {
		case !ok:
		case mise:
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				s, err := ops.SetUpMise(env)
				return ops.WithMise(p, s), err
			}, a.proposeOrSay)
		default:
			a.Propose(p)
		}
	})
}

// updateCount is how many versions and packages can be updated, as last
// checked; "—" before any is known.
func (a *App) updateCount(c *ui.Context) string {
	known := false
	for _, m := range a.managersChecked() {
		known = known || m.UpdatesKnown
	}
	nv, np := updateCounts(a.updateGroups(c))
	if !known && nv+np == 0 {
		return "—"
	}
	return strconv.Itoa(nv + np)
}

// updatesEnd is the Updates item's end in the sidebar: how many there
// are.
func (a *App) updatesEnd(c *ui.Context) func() {
	nv, np := updateCounts(a.updateGroups(c))
	if nv+np == 0 {
		return nil
	}
	return func() {
		ui.Text(c, strconv.Itoa(nv+np)).FontSize(11).TextColor(Colors(c.Theme()).TextFaint).SingleLine()
	}
}

// updatePlan is the plan that installs u: a fix through the manager of the
// copy it replaces, a new line beside the copies through the one new
// versions install through, or mise when there is none (newMise says it
// is set up first); false when Hearth cannot.
func (a *App) updatePlan(u updates.Update) (ops.Plan, bool) {
	if u.Agent {
		return ops.AgentNpmUpdatePlan(u.Tool, agentTitle(u.Tool), u.Copy, a.findAgent(u.Tool))
	}
	if u.Mise != "" {
		detect := a.detectMiseTool(u.Tool)
		if u.Kind == updates.NewLine {
			return ops.VersionPlan(ops.VersionInstall, ops.VersionTarget{Tool: u.Mise, Title: u.Tool, Manager: "mise", Version: u.Version}, detect)
		}
		from := ops.VersionTarget{Tool: u.Tool, Title: u.Tool, Manager: "mise", Version: u.Copy.Version, Root: u.Copy.Source.Root}
		return ops.VersionUpdatePlan(ops.VersionUpdate{From: from, To: ops.VersionTarget{Version: u.Version}, Mise: u.Mise}, detect)
	}
	if u.Kind == updates.NewLine {
		if u.Tool == "npm" {
			return ops.Plan{}, false
		}
		st, _ := a.toolState(u.Tool)
		manager := a.installManager(st)
		version, label := u.Version, versionLabel(u.Tool, u.Version)
		if u.Tool == "java" {
			// Its managers install the newest Temurin of a line.
			version, _, _ = strings.Cut(version, ".")
			label = "Temurin " + version
		}
		if manager == "" {
			if a.BuildPlan == nil {
				return ops.Plan{}, false
			}
			manager = "mise"
		}
		return a.versionPlan(ops.VersionInstall, u.Tool, manager, version, label, "")
	}
	in := u.Copy
	if in.Source.Kind != inventory.KindVersionManager || in.Source.Shim {
		return ops.Plan{}, false
	}
	manager := in.Source.Manager
	if manager == "mise" && in.Default {
		// mise use -g would pin the config's range, as "22" or "latest",
		// to the release: mise upgrade moves the default within it, when
		// mise sees the release in it.
		name := filepath.Base(filepath.Dir(in.Source.Root))
		if t, ok := a.miseOutdated(name); !ok || t.Current != in.Version || t.InRange != u.Version {
			return ops.Plan{}, false
		}
		u.Mise, u.Tool = name, toolTitle(u.Tool)
		from := ops.VersionTarget{Tool: name, Title: u.Tool, Manager: "mise", Version: in.Version, Label: versionLabel(name, in.Version), Root: in.Source.Root}
		return ops.VersionUpdatePlan(ops.VersionUpdate{From: from, To: ops.VersionTarget{Version: u.Version, Label: versionLabel(name, u.Version)}, Mise: name}, a.detect(name))
	}
	vu := ops.VersionUpdate{
		From:    a.versionTarget(u.Tool, manager, cmp.Or(in.Source.Version, in.Version), versionLabel(u.Tool, in.Version), in.Source.Root),
		To:      a.versionTarget(u.Tool, manager, u.Version, versionLabel(u.Tool, u.Version), ""),
		Default: in.Default,
		Globals: globals(in),
	}
	if in.Rust != nil {
		vu.Toolchain = in.Rust.Name
	}
	return ops.VersionUpdatePlan(vu, a.detect(u.Tool))
}

// newMise reports that u's plan needs mise set up first: a new line of a
// tool no version manager keeps yet.
func (a *App) newMise(u updates.Update) bool {
	if u.Agent || u.Kind != updates.NewLine || u.Mise != "" || u.Tool == "npm" || u.Tool == "yarn" {
		return false
	}
	st, _ := a.toolState(u.Tool)
	return a.installManager(st) == ""
}

// miseOutdated is what mise outdated said of its tool name; false when it
// said nothing.
func (a *App) miseOutdated(name string) (versions.MiseTool, bool) {
	if a.MiseOutdated == nil {
		return versions.MiseTool{}, false
	}
	t, ok := a.MiseOutdated.Tools[name]
	return t, ok
}

// detectMiseTool finds mise's tool of the folder name again, for a plan's
// check: its versions as the tool's copies.
func (a *App) detectMiseTool(name string) ops.Detect {
	d := a.detect("mise")
	if d == nil {
		return nil
	}
	return func(ctx context.Context) (inventory.ToolState, error) {
		st, err := d(ctx)
		if err != nil {
			return inventory.ToolState{}, err
		}
		out := inventory.ToolState{Tool: name}
		for _, mt := range st.MiseTools {
			if mt.Name == name {
				out.Installations = mt.Installations
			}
		}
		return out, nil
	}
}

// globals reports packages installed globally into a copy, which go with
// it: Node's besides the npm and Corepack it comes with; any Python or
// Ruby, whose pip and gem Hearth does not tell from those they come with.
func globals(in inventory.Installation) bool {
	switch in.Tool {
	case "node":
		return slices.ContainsFunc(in.Packages, func(p inventory.Package) bool { return p.Name != "npm" && p.Name != "corepack" })
	case "python", "ruby":
		return true
	}
	return false
}

// flatpakBuild names a Flatpak commit, which stands for a version a
// package or its update has none of, as "build 40a4a730925d".
func flatpakBuild(manager, v string) string {
	if manager == "flatpak" && commitID.MatchString(v) {
		return locales.UpdatesBuild(v)
	}
	return v
}

// commitID is the short commit Flatpak shows, 12 hex digits.
var commitID = regexp.MustCompile(`^[0-9a-f]{12}$`)
