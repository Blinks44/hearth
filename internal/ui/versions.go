package ui

import (
	"cmp"
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// installManager is the version manager a tool's new versions install
// through: the one of the copy the terminal runs, else the first found
// that installs it, else mise when it is there; "" when none is, and mise
// is to be set up first.
func (a *App) installManager(st inventory.ToolState) string {
	can := func(manager string) bool {
		_, ok := ops.VersionPlan(ops.VersionInstall, ops.VersionTarget{Tool: st.Tool, Manager: manager, Version: "0"}, nil)
		return ok
	}
	if active := inventory.TerminalCopy(st.Installations); active >= 0 {
		if s := st.Installations[active].Source; s.Kind == inventory.KindVersionManager && can(s.Manager) {
			return s.Manager
		}
	}
	for _, m := range st.Managers {
		if can(m.Name) {
			return m.Name
		}
	}
	if _, ok := a.miseManager(); ok {
		return "mise"
	}
	return ""
}

// versionPlan is the plan that does action to version of tool through
// manager, as ops.VersionPlan, checked by finding the tool again; label
// is the version as the page shows it.
func (a *App) versionPlan(action ops.VersionAction, tool, manager, version, label, root string) (ops.Plan, bool) {
	return ops.VersionPlan(action, a.versionTarget(tool, manager, version, label, root), a.detect(tool))
}

// versionTarget is version of tool through manager, with how the manager
// is set up: whether it needs the administrator, and asdf's version.
func (a *App) versionTarget(tool, manager, version, label, root string) ops.VersionTarget {
	st, _ := a.toolState(tool)
	t := ops.VersionTarget{Tool: tool, Title: toolTitle(tool), Manager: manager, Version: version, Label: label, Root: root, AsdfSet: true}
	for _, m := range st.Managers {
		switch {
		case m.Name != manager:
		case m.Name == "asdf":
			t.AsdfSet = ops.AsdfSet(m.Version)
		default:
			t.Admin = m.Admin
		}
	}
	return t
}

// detect finds tool again, for a plan's check; nil when the app cannot.
func (a *App) detect(tool string) ops.Detect {
	d := a.DetectTool
	if d == nil {
		return nil
	}
	return func(ctx context.Context) (inventory.ToolState, error) { return d(ctx, tool) }
}

// canInstall reports that version of tool can be offered to install:
// through the manager in use, or mise set up first, and not there
// already from any source.
func (a *App) canInstall(st inventory.ToolState, version string) bool {
	manager := a.installManager(st)
	if manager == "" && a.BuildPlan == nil {
		return false
	}
	_, ok := a.versionPlan(ops.VersionInstall, st.Tool, cmp.Or(manager, "mise"), version, "", "")
	return ok && !hasVersion(st.Installations, "", version)
}

// installVersion offers to install version of tool through the manager in
// use; with none, through mise, set up first.
func (a *App) installVersion(tool, version string) {
	st, _ := a.toolState(tool)
	if st.Tool == "" {
		st.Tool = tool
	}
	manager := a.installManager(st)
	label := versionLabel(tool, version)
	if tool == "java" && len(version) > 0 && version[0] >= '0' && version[0] <= '9' && !strings.Contains(version, "-") {
		label = "Temurin " + version
	}
	p, ok := a.versionPlan(ops.VersionInstall, tool, cmp.Or(manager, "mise"), version, label, "")
	needsMiseHook := manager == ""
	if manager == "mise" {
		if m, found := a.miseManager(); !found || (!m.Activated && !m.Shims) {
			needsMiseHook = true
		}
	}
	switch {
	case !ok:
	case !needsMiseHook:
		a.Propose(p)
	case a.BuildPlan != nil:
		a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			s, err := ops.SetUpMise(env)
			return ops.WithMise(p, s), err
		}, a.proposeOrSay)
	default:
		a.Propose(p)
	}
}

// installButton is the download button of an available version, shown
// when it can be installed.
func (a *App) installButton(c *ui.Context, st inventory.ToolState, version, label string) {
	if a.canInstall(st, version) {
		iconButton(c.Key("install:"+version), "download", locales.InstallNamed(label)).
			OnClick(func() { a.installVersion(st.Tool, version) })
	}
}

// hasVersion reports that manager, or any source when it is "", keeps
// version, or a release of it when it is a line, as Java's 25.
func hasVersion(ins []inventory.Installation, manager, version string) bool {
	line := !strings.Contains(version, ".")
	for _, in := range ins {
		if (manager == "" || in.Source.Manager == manager) && !in.Source.Shim && (in.Version == version || in.Source.Version == version || line && strings.HasPrefix(in.Version, version+".")) {
			return true
		}
	}
	return false
}

// versionMenu adds a copy's operations to its menu: make it the default,
// remove it, through the manager that keeps it; false when there are
// none.
func (a *App) versionMenu(m *ui.Menu, in inventory.Installation) bool {
	manager, version, label := in.Source.Manager, cmp.Or(in.Source.Version, in.Version), versionLabel(in.Tool, in.Version)
	if in.Tool == "rust" && in.Rust != nil && in.Rust.Name != "" {
		version = cmp.Or(in.Source.Version, in.Rust.Name)
		label = copyVersion(in)
	}
	makeDefault, canDefault := a.versionPlan(ops.VersionDefault, in.Tool, manager, version, label, in.Source.Root)
	remove, canRemove := a.versionPlan(ops.VersionUninstall, in.Tool, manager, version, label, in.Source.Root)
	if version == "" || in.Source.Shim || !canDefault && !canRemove {
		return false
	}
	m.Separator()
	if canDefault && m.Item(locales.MakeDefault()).Disabled(in.Default).Chosen() {
		a.Propose(makeDefault)
	}
	if canRemove && m.Item(locales.RemoveEllipsis()).Chosen() {
		a.Propose(remove)
	}
	return true
}

// moveMenu adds the operations of a copy no version manager keeps: move
// it to mise, set up first, and remove a python.org copy on macOS, which
// nothing else removes.
func (a *App) moveMenu(m *ui.Menu, in inventory.Installation) {
	s := in.Source
	if a.BuildPlan == nil || in.Stub || s.Shim || inventory.NeededBySystem(in) || s.Kind != inventory.KindInstaller && s.Kind != inventory.KindPackageManager {
		return
	}
	version := in.Version
	if in.Tool == "java" {
		version, _, _ = strings.Cut(version, ".") // mise installs Temurin by its line
	}
	t := ops.VersionTarget{Tool: in.Tool, Title: toolTitle(in.Tool), Version: version, Label: versionLabel(in.Tool, in.Version)}
	move, canMove := ops.MovePlan(t, sourceName(s), a.detect(in.Tool))
	pythonOrg := ""
	if in.Tool == "python" {
		pythonOrg = ops.PythonOrgVersion(in.RealPath)
	}
	if !canMove && pythonOrg == "" {
		return
	}
	m.Separator()
	if canMove && m.Item(locales.MoveToMise()).Chosen() {
		a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			s, err := ops.SetUpMise(env)
			return ops.WithMise(move, s), err
		}, a.proposeOrSay)
	}
	if pythonOrg != "" && m.Item(locales.RemoveEllipsis()).Chosen() {
		detect := a.detect("python")
		a.BuildPlan(func(sysenv.Env, ops.Terminal) (ops.Plan, error) {
			return ops.PythonOrgRemove(pythonOrg, in.Version, detect)
		}, a.proposeOrSay)
	}
}

// installHeader is the header's button that installs version of tool,
// the one recommended, once the scan says what there is; nothing when
// Hearth cannot.
func (a *App) installHeader(c *ui.Context, tool, version string) {
	st, found := a.toolState(tool)
	if !found && a.canInstallTool(tool) {
		return
	}
	st.Tool = tool
	manager := cmp.Or(a.installManager(st), "mise")
	if a.Snapshot == nil || version == "" || manager == "mise" && a.installManager(st) == "" && a.BuildPlan == nil {
		return
	}
	if _, ok := a.versionPlan(ops.VersionInstall, tool, manager, version, "", ""); !ok {
		return
	}
	label := versionLabel(tool, version)
	tip := locales.InstallVersionVia(label, manager)
	installVersionButton(c, tool, label, tip, hasVersion(st.Installations, "", version), func() { a.installVersion(tool, version) })
}

// installVersionButton is the header's "Install version" button.
func installVersionButton(c *ui.Context, tool, target, tooltip string, disabled bool, onClick func()) {
	text := locales.InstallVersion()
	if target != "" {
		text = locales.InstallNamed(target)
	}
	b := ui.PrimaryButton(c.Key(tool+"-install"), "").Height(controlHeight).Disabled(disabled).OnClick(onClick)
	if tooltip != "" {
		b.Tooltip(tooltip)
	}
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, Icon("plus")).Size(14, 14)
			ui.Text(c, text)
		})
	})
}

func (a *App) versionInputPtr(tool string) *string {
	if a.versionInput == nil {
		a.versionInput = map[string]*string{}
	}
	p, ok := a.versionInput[tool]
	if !ok {
		p = new(string)
		a.versionInput[tool] = p
	}
	return p
}

// customVersionRow renders an inline input at the bottom of a language's
// available versions card so users can install any specific version spec.
func (a *App) customVersionRow(c *ui.Context, st inventory.ToolState, example string) {
	manager := a.installManager(st)
	if manager == "" && a.BuildPlan == nil {
		return
	}
	if _, ok := a.versionPlan(ops.VersionInstall, st.Tool, cmp.Or(manager, "mise"), "0", "", ""); !ok {
		return
	}
	pal := Colors(c.Theme())
	ptr := a.versionInputPtr(st.Tool)
	install := func() {
		v := strings.TrimSpace(*ptr)
		if !ops.ValidVersionSpec(v) {
			return
		}
		*ptr = ""
		a.installVersion(st.Tool, v)
	}
	ui.Row(c).Gap(8).Padding(8, 12, 10, 14).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.Line).Children(func() {
		ui.TextInput(c.Key(st.Tool+"-custom-version-input"), ptr).
			Placeholder(locales.CustomVersionPlaceholder(example)).Height(28).Grow(1).MinWidth(0).
			OnChange(c.Invalidate).OnSubmit(install)
		v := strings.TrimSpace(*ptr)
		label := locales.InstallVersion()
		if v != "" {
			label = locales.InstallNamed(v)
		}
		ui.Button(c.Key(st.Tool+"-custom-version-install"), locales.Install()).Label(label).
			Disabled(!ops.ValidVersionSpec(v)).Shrink(0).OnClick(install)
	})
}

// availableAside is the note of a tool's available versions: the manager
// they install through, when Hearth can install them, else asOf.
func (a *App) availableAside(st inventory.ToolState, asOf string) string {
	manager := a.installManager(st)
	if manager == "" && a.BuildPlan == nil {
		return asOf
	}
	if _, ok := a.versionPlan(ops.VersionInstall, st.Tool, cmp.Or(manager, "mise"), "0", "", ""); !ok {
		return asOf
	}
	return locales.Through(cmp.Or(manager, "mise"))
}

// versionLabel is a version as a plan names it: v22.14.0 for Node, as
// Node does, the version alone for the rest.
func versionLabel(tool, version string) string {
	if tool == "node" {
		return versionText(version)
	}
	return version
}
