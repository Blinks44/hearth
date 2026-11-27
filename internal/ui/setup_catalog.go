package ui

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/catalog"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// setupTitle names a tool of the catalog in the wizard.
func setupTitle(id string) string {
	switch id {
	case "homebrew":
		return "Homebrew"
	case "mise":
		return "mise"
	case "node":
		return "Node.js LTS"
	case "docker":
		return locales.SetupDockerEngine()
	case "aws":
		return "AWS CLI"
	case "gcloud":
		return "Google Cloud CLI"
	case "az":
		return "Azure CLI"
	case "terraform":
		return "Terraform"
	case "tofu":
		return "OpenTofu"
	case "gh":
		return "GitHub CLI"
	case "deno":
		return "Deno"
	case "zig":
		return "Zig"
	case "elixir":
		return "Elixir"
	case "jq":
		return "jq"
	case "ripgrep":
		return "ripgrep"
	case "fzf":
		return "fzf"
	case "fd":
		return "fd"
	case "bat":
		return "bat"
	case "just":
		return "just"
	case "direnv":
		return "direnv"
	case "k9s":
		return "k9s"
	}
	if e, _ := catalog.Find(id); e.Group == catalog.Agent {
		return agentTitle(id)
	}
	if kind, ok := catalog.DatabaseKinds[id]; ok {
		return kind
	}
	return toolName(id)
}

// managerTitle names how a method installs, as "mise" or "Homebrew".
func managerTitle(m catalog.Method) string {
	switch m.Manager {
	case "installer":
		return locales.SetupOfficialInstaller()
	case "xcode-select":
		return "Xcode Command Line Tools"
	case "brew":
		return "Homebrew"
	case "winget":
		return "WinGet"
	case "corepack":
		return "Corepack"
	case "choco":
		return "Chocolatey"
	case "scoop":
		return "Scoop"
	}
	if m.Manager == "apt" || m.Manager == "dnf" || m.Manager == "apk" {
		return strings.ToUpper(m.Manager)
	}
	return m.Manager
}

type setupCatInfo struct {
	cat   installCat
	icon  string
	title func() string
	chip  func() string
}

var setupCategories = []setupCatInfo{
	{catLanguages, "lucide/code-xml", locales.SetupLanguages, locales.SetupCatLanguages},
	{catPackages, "lucide/package", locales.SetupCatPackages, locales.SetupCatPackages},
	{catInfra, "lucide/container", locales.SetupCatInfra, locales.SetupCatInfra},
	{catDatabases, "lucide/database", locales.SetupDatabases, locales.SetupDatabases},
	{catAgents, "lucide/bot", locales.SetupAgents, locales.SetupAgents},
	{catTerminal, "lucide/terminal", locales.SetupCatTerminal, locales.SetupCatTerminal},
}

// setupCategory maps a catalog entry to its visual category on the Install page.
func setupCategory(e catalog.Entry) installCat {
	switch e.Group {
	case catalog.Base:
		return catAll
	case catalog.Language:
		return catLanguages
	case catalog.Database:
		return catDatabases
	case catalog.Agent:
		return catAgents
	}
	switch e.ID {
	case "docker", "kubectl", "helm", "kind", "k9s", "aws", "gcloud", "az", "terraform", "tofu":
		return catInfra
	case "jq", "ripgrep", "fzf", "fd", "bat", "just", "direnv":
		return catTerminal
	default:
		return catPackages
	}
}

// setupToolIcon returns the "brand/<slug>" or "lucide/<name>" icon key for a
// catalog tool ID.
func setupToolIcon(id string) string {
	switch id {
	case "homebrew":
		return "brand/homebrew"
	case "mise":
		return "brand/mise"
	case "node":
		return "brand/nodedotjs"
	case "python":
		return "brand/python"
	case "go":
		return "brand/go"
	case "rust":
		return "brand/rust"
	case "bun":
		return "brand/bun"
	case "php":
		return "brand/php"
	case "java":
		return "brand/openjdk"
	case "ruby":
		return "brand/ruby"
	case "dotnet":
		return "brand/dotnet"
	case "deno":
		return "brand/deno"
	case "zig":
		return "brand/zig"
	case "elixir":
		return "brand/elixir"
	case "pnpm":
		return "brand/pnpm"
	case "yarn":
		return "brand/yarn"
	case "git":
		return "brand/git"
	case "gh":
		return "brand/github"
	case "composer":
		return "brand/composer"
	case "uv":
		return "brand/uv"
	case "pipx":
		return "brand/pipx"
	case "bundler":
		return "brand/rubygems"
	case "maven":
		return "brand/apachemaven"
	case "gradle":
		return "brand/gradle"
	case "docker":
		return "brand/docker"
	case "kubectl", "kind", "k9s":
		return "brand/kubernetes"
	case "helm":
		return "brand/helm"
	case "aws":
		return "brand/aws"
	case "gcloud":
		return "brand/googlecloud"
	case "az":
		return "brand/azure"
	case "terraform":
		return "brand/terraform"
	case "tofu":
		return "brand/opentofu"
	case "bat":
		return "brand/bat"
	case "just":
		return "brand/just"
	case "jq", "ripgrep", "fzf", "fd", "direnv":
		return "lucide/terminal"
	case "postgresql":
		return "brand/postgresql"
	case "redis":
		return "brand/redis"
	case "mysql":
		return "brand/mysql"
	case "mariadb":
		return "brand/mariadb"
	case "mongodb":
		return "brand/mongodb"
	}
	if e, _ := catalog.Find(id); e.Group == catalog.Agent {
		return agentIcon(id)
	}
	return "lucide/download"
}

// setupToolPath returns the Hearth page path for an installed catalog tool,
// or "" when the tool has no dedicated page.
func setupToolPath(id string) string {
	switch id {
	case "homebrew":
		return "/system/packages"
	case "node", "bun", "python", "php", "rust", "go", "java", "ruby", "dotnet":
		return "/tools/" + id
	case "pnpm", "yarn":
		return "/tools/jspm"
	case "git", "gh":
		return gitPage
	case "docker":
		return "/tools/docker"
	case "kubectl", "helm", "kind":
		return kubePage
	case "aws", "gcloud", "az", "terraform", "tofu", "jq", "ripgrep", "fzf", "fd", "bat", "just", "direnv", "k9s":
		return cliPage
	case "composer":
		return "/tools/php"
	case "uv", "pipx":
		return "/tools/python"
	case "bundler":
		return "/tools/ruby"
	case "maven", "gradle":
		return "/tools/java"
	case "postgresql", "redis", "mysql", "mariadb", "mongodb":
		return "/tools/databases"
	}
	if e, _ := catalog.Find(id); e.Group == catalog.Agent {
		return agentPath(id)
	}
	return ""
}

// setupToolTile draws a compact rounded icon tile for a catalog tool.
func setupToolTile(c *ui.Context, id string, size float32, selected, muted bool) {
	t := c.Theme()
	pal := Colors(t)
	bg, border, fg := pal.Subtle, pal.Line, t.TextMuted
	switch {
	case selected:
		bg, border, fg = pal.AccentSoft, Accent.Alpha(0.35), Accent
	case muted:
		fg = pal.TextFaint
	}
	ui.Box(c).Size(size, size).Shrink(0).Radius(size / 4).Center().Background(bg).
		BorderWidth(1).BorderColor(border).Children(func() {
		iconOrLogo(c, setupToolIcon(id), size*0.58, fg)
	})
}

// setupGroupCard renders a catalog category card with a small category icon
// before its title and a monospaced count aside on the right.
func setupGroupCard(c *ui.Context, icon, title, aside string, rows func()) {
	t := c.Theme()
	pal := Colors(t)
	ui.Column(c).Radius(12).Clip().Background(t.Background).BorderWidth(1).BorderColor(pal.Divider).Children(func() {
		ui.Row(c).Gap(8).Padding(12, 14, 10, 14).AlignItems(ui.Center).Children(func() {
			if icon != "" {
				iconOrLogo(c, icon, 14, pal.TextFaint)
			}
			ui.Text(c, title).FontSize(13.5).FontWeight(600).Grow(1).SingleLine()
			if aside != "" {
				ui.Text(c, aside).Font(FontMono).FontSize(11).TextColor(pal.TextFaint).SingleLine()
			}
		})
		rows()
	})
}

// setupVersionOption is one selectable version or package variant for a catalog tool.
type setupVersionOption struct {
	label string
	value string // package name when isPkg, else mise version spec ("" = default)
	isPkg bool
}

// setupVersionOptions returns selectable versions for tool id under method m
// (default option first), or nil when only one option is available.
func (a *App) setupVersionOptions(id string, m catalog.Method, now time.Time) []setupVersionOption {
	if pvs := catalog.PackageVersions(id, m.Manager); len(pvs) > 1 {
		out := make([]setupVersionOption, len(pvs))
		for i, pv := range pvs {
			out[i] = setupVersionOption{label: pv.Version, value: pv.Package, isPkg: true}
		}
		return out
	}
	if m.Manager != "mise" {
		return nil
	}
	switch id {
	case "node":
		return a.setupNodeOptions(now)
	case "java":
		return a.setupJavaOptions(now)
	case "python":
		return setupCycleOptions(a.PythonVersions, now)
	case "ruby":
		return setupCycleOptions(a.RubyVersions, now)
	case "php":
		return setupCycleOptions(a.PHPVersions, now)
	case "go":
		if a.GoVersions == nil || len(a.GoVersions.Releases) < 2 {
			return nil
		}
		limit := min(len(a.GoVersions.Releases), 4)
		out := make([]setupVersionOption, 0, limit)
		for i, rel := range a.GoVersions.Releases[:limit] {
			val := rel
			if i == 0 {
				val = ""
			}
			out = append(out, setupVersionOption{label: rel, value: val})
		}
		return out
	case "dotnet":
		if a.DotnetVersions == nil {
			return nil
		}
		var out []setupVersionOption
		for _, l := range a.DotnetVersions.Lines {
			if l.Version == "" || l.Ended(now) || strings.Contains(l.Version, "-") {
				continue
			}
			val := l.Version
			if len(out) == 0 {
				val = ""
			}
			out = append(out, setupVersionOption{label: l.Version, value: val})
			if len(out) == 4 {
				break
			}
		}
		if len(out) < 2 {
			return nil
		}
		return out
	}
	return nil
}

func (a *App) setupNodeOptions(now time.Time) []setupVersionOption {
	if a.NodeVersions == nil || len(a.NodeVersions.Releases) == 0 {
		return []setupVersionOption{
			{label: "LTS", value: ""},
			{label: "24", value: "24"},
			{label: "22", value: "22"},
			{label: "20", value: "20"},
		}
	}
	var out []setupVersionOption
	seen := map[int]bool{}
	if rec, ok := a.NodeVersions.Recommended(); ok && rec.Version != "" {
		major := versions.Major(rec.Version)
		seen[major] = true
		out = append(out, setupVersionOption{label: strconv.Itoa(major) + " LTS", value: ""})
	}
	for _, r := range a.NodeVersions.Releases {
		major := versions.Major(r.Version)
		if major == 0 || seen[major] {
			continue
		}
		if line, ok := a.NodeVersions.Lines[major]; ok && line.Ended(now) {
			continue
		}
		seen[major] = true
		label := strconv.Itoa(major)
		if r.LTS != "" {
			label += " LTS"
		}
		val := strconv.Itoa(major)
		if len(out) == 0 {
			val = ""
		}
		out = append(out, setupVersionOption{label: label, value: val})
		if len(out) == 4 {
			break
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func (a *App) setupJavaOptions(now time.Time) []setupVersionOption {
	if a.JavaVersions == nil || len(a.JavaVersions.Lines) == 0 {
		return []setupVersionOption{
			{label: "Temurin 25", value: ""},
			{label: "Temurin 21", value: "temurin-21"},
			{label: "Temurin 17", value: "temurin-17"},
		}
	}
	var out []setupVersionOption
	for _, l := range a.JavaVersions.Lines {
		if l.Cycle == "" || !l.LTS || l.Ended(now) || (!l.Release.IsZero() && now.Before(l.Release)) {
			continue
		}
		val := "temurin-" + l.Cycle
		if len(out) == 0 && l.Cycle == "25" {
			val = ""
		}
		out = append(out, setupVersionOption{label: "Temurin " + l.Cycle, value: val})
		if len(out) == 4 {
			break
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func setupCycleOptions(cycles *versions.Cycles, now time.Time) []setupVersionOption {
	if cycles == nil {
		return nil
	}
	var out []setupVersionOption
	for _, l := range cycles.Lines {
		if l.Cycle == "" || l.Ended(now) || (!l.Release.IsZero() && now.Before(l.Release)) {
			continue
		}
		val := l.Cycle
		if len(out) == 0 {
			val = ""
		}
		out = append(out, setupVersionOption{label: l.Cycle, value: val})
		if len(out) == 4 {
			break
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

// setupSelectedOption returns the active version option for tool id from opts.
func (a *App) setupSelectedOption(id string, opts []setupVersionOption) setupVersionOption {
	if len(opts) == 0 {
		return setupVersionOption{}
	}
	if opts[0].isPkg {
		if pkg := a.setup.packages[id]; pkg != "" {
			for _, o := range opts {
				if o.value == pkg {
					return o
				}
			}
		}
		return opts[0]
	}
	if ver := a.setup.versions[id]; ver != "" {
		for _, o := range opts {
			if o.value == ver {
				return o
			}
		}
	}
	return opts[0]
}

func (a *App) setupTargetVersion(id string, m catalog.Method, now time.Time) string {
	if opts := a.setupVersionOptions(id, m, now); len(opts) > 0 {
		sel := a.setupSelectedOption(id, opts)
		if sel.isPkg && a.setup.packages[id] != "" {
			return sel.label
		}
		if !sel.isPkg && a.setup.versions[id] != "" {
			return sel.label
		}
	}
	switch id {
	case "node":
		if a.NodeVersions != nil {
			if rel, ok := a.NodeVersions.Recommended(); ok && rel.Version != "" {
				return rel.Version + " LTS"
			}
		}
	case "java":
		if opts := a.setupJavaOptions(now); len(opts) > 0 {
			return opts[0].label
		}
	case "python":
		if v := a.PythonVersions.Newest(now); v != "" {
			return v
		}
	case "go":
		if a.GoVersions != nil && len(a.GoVersions.Releases) > 0 {
			return a.GoVersions.Releases[0]
		}
	case "rust":
		if v := a.RustVersions.Newest(now); v != "" {
			return v
		}
	case "bun":
		if a.BunLatest != nil && a.BunLatest.Version != "" {
			return a.BunLatest.Version
		}
	case "ruby":
		if v := a.RubyVersions.Newest(now); v != "" {
			return v
		}
	case "dotnet":
		if a.DotnetVersions != nil {
			for _, l := range a.DotnetVersions.Lines {
				if l.Version != "" && !l.Ended(now) && !strings.Contains(l.Version, "-") {
					return l.Version
				}
			}
		}
	case "pnpm":
		if a.JSVersions != nil && a.JSVersions.Pnpm.Version != "" {
			return a.JSVersions.Pnpm.Version
		}
	case "yarn":
		if a.JSVersions != nil && a.JSVersions.Yarn.Version != "" {
			return a.JSVersions.Yarn.Version
		}
	}
	v := ops.SetupTargetVersion(id, m)
	if v == "latest" {
		return locales.SetupVersionLatest()
	}
	return v
}

func setupMatches(e catalog.Entry, q string) bool {
	return strings.Contains(strings.ToLower(setupTitle(e.ID)), q) ||
		strings.Contains(strings.ToLower(e.ID), q) ||
		strings.Contains(strings.ToLower(catalog.About(e.ID)), q)
}

// setupConflict is the tool there or chosen that e does not install
// beside; "" for none.
func (a *App) setupConflict(e catalog.Entry) string {
	for _, id := range e.Conflicts {
		if a.setupHave(id) || a.setupChosen(id) {
			return id
		}
	}
	return ""
}

// setupAllOffers returns every catalog tool offered on this system except Base.
func (a *App) setupAllOffers(in ops.Setup) []catalog.Entry {
	var out []catalog.Entry
	for _, e := range a.setupEntries() {
		if e.Group != catalog.Base && (a.setupHave(e.ID) || slices.ContainsFunc(e.On(goos), in.Usable)) {
			out = append(out, e)
		}
	}
	return out
}

// setupOffers are the tools of group g the page offers here: those that
// install here, and those there already.
func (a *App) setupOffers(in ops.Setup, g catalog.Group) []catalog.Entry {
	var out []catalog.Entry
	for _, e := range a.setupEntries() {
		if e.Group == g && (a.setupHave(e.ID) || slices.ContainsFunc(e.On(goos), in.Usable)) {
			out = append(out, e)
		}
	}
	return out
}

// setupCounts are how many tools the page offers here, and how many of
// them are there.
func (a *App) setupCounts() (have, total int) {
	in := a.setupInput()
	for _, g := range []catalog.Group{catalog.Base, catalog.Language, catalog.Tool, catalog.Agent, catalog.Database} {
		for _, e := range a.setupOffers(in, g) {
			total++
			if a.setupHave(e.ID) {
				have++
			}
		}
	}
	return have, total
}

// setupChosenTools are the tools switched on that are not there, in the
// catalog's order.
func (a *App) setupChosenTools() []string {
	var out []string
	for _, e := range a.setupEntries() {
		if a.setupChosen(e.ID) && !a.setupHave(e.ID) {
			out = append(out, e.ID)
		}
	}
	return out
}
