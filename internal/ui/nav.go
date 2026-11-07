package ui

import (
	"strings"

	"github.com/Blinks44/hearth/locales"
)

// page is a section of the app: its path in the router, the icon the
// sidebar shows, and the title and subtitle of its header. They are
// functions, which say them in the language the app shows as it changes.
type page struct {
	path, icon      string // icon: "lucide/<name>" or "brand/<slug>"
	title, subtitle func() string
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

	// sections are the sidebar's titled groups after the languages.
	sections = []struct {
		title func() string
		pages []page
	}{
		{locales.SectionTools, []page{
			{"/tools/jspm", "brand/pnpm", name("npm · pnpm · Yarn"), locales.SectionToolsJavaScript},
			{"/tools/docker", "brand/docker", name("Docker"), locales.SectionToolsContainers},
			{"/tools/databases", "lucide/database", locales.DatabasesTitle, locales.SectionTools},
			{gitPage, "brand/git", name("Git"), locales.SectionToolsVcs},
			{cliPage, "lucide/square-terminal", locales.CliTitle, locales.SectionToolsCli},
		}},
		{locales.SectionAI, []page{
			{"/ai/agents", "lucide/bot", locales.AgentsTitle, locales.SectionAI},
			{"/ai/skills", "lucide/puzzle", locales.SkillsTitle, locales.SectionAI},
			{"/ai/mcp", "lucide/plug", name("MCP"), locales.SectionAI},
			{modelsPage, "lucide/brain", locales.ModelsTitle, locales.ModelsSubtitle},
		}},
		{locales.SectionSystem, []page{
			{"/system/packages", "lucide/package", locales.PackageManagersTitle, locales.SectionSystem},
			{pathPage, "lucide/terminal", locales.PathTitle, locales.SectionSystem},
		}},
	}
)

// pages lists every page, for the router.
func pages() []page {
	all := []page{overview, projectsPage, cleanupNav}
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
