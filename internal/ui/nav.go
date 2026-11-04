package ui

import "strings"

// page is a section of the app: its path in the router, the icon the
// sidebar shows, and the title and subtitle of its header.
type page struct {
	path, icon      string // icon: "lucide/<name>" or "brand/<slug>"
	title, subtitle string
}

// language is a page under "Languages & runtimes", named by its ID in
// Languages.
type language struct {
	id string
	page
}

var (
	overview = page{"/overview", "lucide/layout-dashboard", "Overview", "Your whole developer environment on this computer"}
	projects = page{"/projects", "lucide/folder-git-2", "Projects", "Code folders Hearth checks"}
	settings = page{"/settings", "lucide/settings", "Settings", "Hearth"}

	languages = []language{
		{"node", page{"/tools/node", "brand/nodedotjs", "Node.js", "Languages & runtimes · JavaScript"}},
		{"bun", page{"/tools/bun", "brand/bun", "Bun", "Languages & runtimes · JavaScript"}},
		{"python", page{"/tools/python", "brand/python", "Python", "Languages & runtimes"}},
		{"php", page{"/tools/php", "brand/php", "PHP", "Languages & runtimes"}},
		{"rust", page{"/tools/rust", "brand/rust", "Rust", "Languages & runtimes"}},
		{"go", page{"/tools/go", "brand/go", "Go", "Languages & runtimes"}},
		{"java", page{"/tools/java", "brand/openjdk", "Java", "Languages & runtimes"}},
		{"ruby", page{"/tools/ruby", "brand/ruby", "Ruby", "Languages & runtimes"}},
		{"deno", page{"/tools/deno", "brand/deno", "Deno", "Languages & runtimes · JavaScript"}},
		{"dotnet", page{"/tools/dotnet", "brand/dotnet", ".NET", "Languages & runtimes"}},
	}

	// sections are the sidebar's titled groups after the languages.
	sections = []struct {
		title string
		pages []page
	}{
		{"Tools", []page{
			{"/tools/jspm", "brand/pnpm", "npm · pnpm · Yarn", "Tools · JavaScript"},
			{"/tools/docker", "brand/docker", "Docker", "Tools · containers"},
			{"/tools/databases", "lucide/database", "Databases", "Tools"},
		}},
		{"AI", []page{
			{"/ai/agents", "lucide/bot", "Agents", "AI"},
			{"/ai/skills", "lucide/puzzle", "Skills", "AI"},
			{"/ai/mcp", "lucide/plug", "MCP", "AI"},
		}},
		{"System", []page{
			{"/system/packages", "lucide/package", "Package managers", "System"},
			{"/system/path", "lucide/terminal", "PATH & variables", "System"},
		}},
	}
)

// pages lists every page, for the router.
func pages() []page {
	all := []page{overview, projects}
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
