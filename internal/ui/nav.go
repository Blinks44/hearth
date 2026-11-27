package ui

import (
	"cmp"
	"strings"
	"unicode"

	"golang.org/x/text/collate"
	lang "golang.org/x/text/language"

	"github.com/Blinks44/hearth/locales"
)

// page is a section of the app: its path in the router, the icon the
// sidebar shows, and the title and subtitle of its header. They are
// functions, which say them in the language the app shows as it changes.
type page struct {
	path, icon      string // icon: "lucide/<name>" or "brand/<slug>"
	title, subtitle func() string
}

// byTitle orders x and y by their titles as the language shown sorts
// words, Russian's before Latin ones in Russian, regardless of case: the
// sidebar's languages, the tools, servers and skills in their lists.
func (a *App) byTitle(x, y string) int {
	if a.collator == nil || a.collatorLocale != a.Locale {
		a.collator, a.collatorLocale = collate.New(lang.Make(a.Locale), collate.IgnoreCase), a.Locale
	}
	if a.Locale == "ru" {
		if c := cmp.Compare(latin(x), latin(y)); c != 0 {
			return c
		}
	}
	return a.collator.CompareString(x, y)
}

// latin is 1 for a title whose first letter is not Cyrillic, 0 for one
// whose is: x/text's collation knows no script order.
func latin(s string) int {
	for _, r := range s {
		if unicode.IsLetter(r) {
			if unicode.Is(unicode.Cyrillic, r) {
				return 0
			}
			return 1
		}
	}
	return 1
}

// name is a title that stays the same in every language, as a product's.
func name(s string) func() string { return func() string { return s } }

// language is a page under locales.SectionLanguages, named by its ID in
// Languages.
type language struct {
	id string
	page
}

// pathPage is the PATH's page, where its issues live.
const pathPage = "/system/path"

var (
	overview     = page{"/overview", "lucide/layout-dashboard", locales.OverviewTitle, locales.OverviewSubtitle}
	installNav   = page{setupPage, "lucide/download", locales.InstallTitle, locales.InstallSubtitle}
	problemsNav  = page{problemsPage, "lucide/triangle-alert", locales.ProblemsTitle, locales.ProblemsSubtitle}
	updatesNav   = page{updatesPage, "lucide/circle-arrow-up", locales.UpdatesTitle, locales.UpdatesSubtitle}
	projectsPage = page{"/projects", "lucide/folder-git-2", locales.ProjectsTitle, locales.ProjectsSubtitle}
	cleanupNav   = page{cleanupPage, "lucide/brush-cleaning", locales.CleanupTitle, locales.CleanupSubtitle}
	settings     = page{"/settings", "lucide/settings", locales.SettingsTitle, name("Hearth")}

	languages = []language{
		{"node", page{"/tools/node", "brand/nodedotjs", name("Node.js"), locales.SectionLanguagesJavaScript}},
		{"bun", page{"/tools/bun", "brand/bun", name("Bun"), locales.SectionLanguagesJavaScript}},
		{"python", page{"/tools/python", "brand/python", name("Python"), locales.SectionLanguages}},
		{"php", page{"/tools/php", "brand/php", name("PHP"), locales.SectionLanguages}},
		{"cpp", page{cppPage, "brand/cplusplus", name("C/C++"), locales.SectionLanguagesCpp}},
		{"rust", page{"/tools/rust", "brand/rust", name("Rust"), locales.SectionLanguages}},
		{"go", page{"/tools/go", "brand/go", name("Go"), locales.SectionLanguages}},
		{"java", page{"/tools/java", "brand/openjdk", name("Java"), locales.SectionLanguages}},
		{"ruby", page{"/tools/ruby", "brand/ruby", name("Ruby"), locales.SectionLanguages}},
		{"dotnet", page{"/tools/dotnet", "brand/dotnet", name(".NET"), locales.SectionLanguages}},
	}

	// maintenance is the sidebar's section of what to do on the computer,
	// under Overview and Projects.
	maintenance = []page{problemsNav, updatesNav, cleanupNav, installNav}

	// sections are the sidebar's titled groups after the languages, each
	// in a fixed order, the pages used most first.
	sections = []struct {
		title func() string
		pages []page
	}{
		{locales.SectionInfrastructure, []page{
			{"/tools/docker", "brand/docker", name("Docker"), locales.SectionToolsContainers},
			{kubePage, "brand/kubernetes", name("Kubernetes"), locales.KubeSubtitle},
			{"/tools/databases", "lucide/database", locales.DatabasesTitle, locales.SectionInfrastructure},
			{portsPage, "lucide/ethernet-port", locales.ServicesPortsTitle, locales.ServicesPortsSubtitle},
			{cliPage, "lucide/square-terminal", locales.CliTitle, locales.SectionToolsCli},
		}},
		{locales.SectionTools, []page{
			{gitPage, "brand/git", name("Git"), locales.SectionToolsVcs},
			{"/tools/jspm", "brand/pnpm", name("npm · pnpm · Yarn"), locales.SectionToolsJavaScript},
			{editorsPage, "lucide/code-xml", locales.EditorsTitle, locales.SectionToolsEditors},
			{mobilePage, "lucide/smartphone", locales.MobileTitle, locales.SectionToolsMobile},
		}},
		{locales.SectionAI, []page{
			{"/ai/agents", "lucide/bot", locales.AgentsTitle, locales.SectionAI},
			{"/ai/skills", "lucide/puzzle", locales.SkillsTitle, locales.SectionAI},
			{"/ai/mcp", "lucide/plug", name("MCP"), locales.SectionAI},
			{instructionsPage, "lucide/file-text", locales.InstructionsTitle, locales.InstructionsSubtitle},
			{modelsPage, "lucide/brain", locales.ModelsTitle, locales.ModelsSubtitle},
			{usagePage, "lucide/chart-column", locales.UsageTitle, locales.UsageSubtitle},
			{quotasPage, "lucide/gauge", locales.QuotasTitle, locales.QuotasSubtitle},
		}},
		{locales.SectionSystem, []page{
			{pathPage, "lucide/terminal", locales.PathTitle, locales.SectionSystem},
			{"/system/packages", "lucide/package", locales.PackageManagersTitle, locales.SectionSystem},
		}},
	}
)

// pages lists every page, for the router.
func pages() []page {
	all := append([]page{overview, projectsPage}, maintenance...)
	for _, l := range languages {
		all = append(all, l.page)
	}
	for _, s := range sections {
		all = append(all, s.pages...)
	}
	return append(all, settings)
}

// within reports whether path is the page at base or one under it, so
// that /projects/x keeps Projects chosen in the sidebar.
func within(path, base string) bool {
	return path == base || strings.HasPrefix(path, base+"/")
}
