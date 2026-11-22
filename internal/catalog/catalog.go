// Package catalog describes the tools Hearth installs on a new computer:
// what each is for, how it installs on each system and what it needs
// first. It holds data only: the operations in internal/ops turn a
// method into a plan.
package catalog

import (
	"slices"

	"github.com/Blinks44/hearth/locales"
)

// Group is where a tool stands in the new computer's choice.
type Group int

const (
	// Base installs first: what the rest installs with.
	Base Group = iota
	Language
	Tool
	Agent
	Database
)

// Method is a way to install a tool.
type Method struct {
	// Manager installs it: a system package manager as
	// inventory.PackageManager's ID ("brew", "winget", "apt"…), "mise",
	// "npm", "corepack", "xcode-select" (Apple's Command Line Tools), or
	// "installer", the maker's own.
	Manager string
	// Package is the name the manager knows the tool by; for "installer",
	// the address it comes from. Kind is Homebrew's formula or cask.
	Package, Kind string
	// OS are the systems it works on, as runtime.GOOS names them; the
	// manager's when empty.
	OS []string
	// Terms is the address of terms of use beyond open source, as a
	// license that is paid for work; "" when there are none.
	Terms string
	// Engine is the container engine it installs, as
	// inventory.DockerEngine names it; "" for a tool that is not one.
	Engine string
}

// Engines are the container engines Hearth installs, each through each
// manager that has it: the Docker page's and, those the wizard offers,
// the catalog's Docker. Linux distributions package Docker Engine with
// its CLI, each under a name of its own. Kiri is in no system manager:
// its "installer" is the maker's download page, which Hearth only links.
var Engines = []Method{
	{Engine: "Colima", Manager: "brew", Package: "colima", Kind: "formula"},
	{Engine: "OrbStack", Manager: "brew", Package: "orbstack", Kind: "cask", Terms: "https://orbstack.dev/pricing"},
	{Engine: "Docker Desktop", Manager: "brew", Package: "docker-desktop", Kind: "cask", Terms: dockerDesktopTerms},
	{Engine: "Podman Desktop", Manager: "brew", Package: "podman-desktop", Kind: "cask"},
	{Engine: "Rancher Desktop", Manager: "brew", Package: "rancher", Kind: "cask"},
	{Engine: "Podman", Manager: "brew", Package: "podman", Kind: "formula"},
	{Engine: "Docker Desktop", Manager: "winget", Package: "Docker.DockerDesktop", Terms: dockerDesktopTerms},
	{Engine: "Podman", Manager: "winget", Package: "RedHat.Podman"},
	{Engine: "Podman Desktop", Manager: "winget", Package: "RedHat.Podman-Desktop"},
	{Engine: "Rancher Desktop", Manager: "winget", Package: "SUSE.RancherDesktop"},
	{Engine: "Docker Engine", Manager: "apt", Package: "docker.io"},
	{Engine: "Docker Engine", Manager: "dnf", Package: "moby-engine"},
	{Engine: "Docker Engine", Manager: "zypper", Package: "docker"},
	{Engine: "Docker Engine", Manager: "pacman", Package: "docker"},
	{Engine: "Docker Engine", Manager: "apk", Package: "docker"},
	{Engine: "Podman", Manager: "apt", Package: "podman"},
	{Engine: "Podman", Manager: "dnf", Package: "podman"},
	{Engine: "Podman", Manager: "zypper", Package: "podman"},
	{Engine: "Podman", Manager: "pacman", Package: "podman"},
	{Engine: "Podman", Manager: "apk", Package: "podman"},
	{Engine: "Kiri", Manager: "installer", Package: "https://runkiri.com/releases/"},
}

const dockerDesktopTerms = "https://docs.docker.com/subscription-billing/desktop-license/"

// EngineMethod is how engine installs through manager; false when it
// does not.
func EngineMethod(engine, manager string) (Method, bool) {
	i := slices.IndexFunc(Engines, func(m Method) bool { return m.Engine == engine && m.Manager == manager })
	if i < 0 {
		return Method{}, false
	}
	return Engines[i], true
}

// engines are the methods of Engines of each engine and manager, in
// order.
func engines(pairs ...string) []Method {
	var out []Method
	for i := 0; i < len(pairs); i += 2 {
		m, ok := EngineMethod(pairs[i], pairs[i+1])
		if !ok {
			panic("catalog: no engine " + pairs[i] + " through " + pairs[i+1])
		}
		out = append(out, m)
	}
	return out
}

// managerOS are the systems each manager works on; all for the others.
// Homebrew runs on Linux too, but there Hearth installs with the
// system's own manager.
var managerOS = map[string][]string{
	"brew": {"darwin"}, "xcode-select": {"darwin"},
	"winget": {"windows"}, "choco": {"windows"}, "scoop": {"windows"},
	"apt": {"linux"}, "dnf": {"linux"}, "zypper": {"linux"}, "pacman": {"linux"}, "apk": {"linux"}, "snap": {"linux"}, "flatpak": {"linux"},
}

// Systems are the systems m works on.
func (m Method) Systems() []string {
	if len(m.OS) > 0 {
		return m.OS
	}
	if os, ok := managerOS[m.Manager]; ok {
		return os
	}
	return All
}

// All are the systems Hearth runs on.
var All = []string{"darwin", "windows", "linux"}

// managerTools are the tools in the catalog each manager is, or runs
// on: they install before what the manager installs.
var managerTools = map[string]string{"brew": "homebrew", "mise": "mise", "npm": "node", "corepack": "node"}

// Entry is a tool of the catalog.
type Entry struct {
	// ID is the tool's, as inventory and the sidebar name it, or the
	// agent's; "homebrew" for Homebrew.
	ID    string
	Group Group
	// OS are the systems the tool installs on; All when empty.
	OS []string
	// Methods are the ways it installs, in the order Hearth prefers: the
	// first that works on a system is the one recommended there.
	Methods []Method
	// Needs are the tools it needs first beyond its manager.
	Needs []string
	// Conflicts are the tools it does not install beside: their packages
	// put the same commands in the same place.
	Conflicts []string
}

// Systems are the systems e installs on.
func (e Entry) Systems() []string {
	if len(e.OS) > 0 {
		return e.OS
	}
	return All
}

// On returns e's methods that work on goos, in order.
func (e Entry) On(goos string) []Method {
	var out []Method
	for _, m := range e.Methods {
		if slices.Contains(m.Systems(), goos) {
			out = append(out, m)
		}
	}
	return out
}

// NeedsWith returns the tools e needs before m installs it: its own needs,
// then its manager.
func (e Entry) NeedsWith(m Method) []string {
	out := slices.Clone(e.Needs)
	if t, ok := managerTools[m.Manager]; ok && !slices.Contains(out, t) {
		out = append(out, t)
	}
	return out
}

// mise installs a tool through mise, on every system.
func mise(name string) []Method { return []Method{{Manager: "mise", Package: name}} }

// unixInstaller is the maker's installer script at url, on macOS and
// Linux.
func unixInstaller(url string) Method {
	return Method{Manager: "installer", Package: url, OS: []string{"darwin", "linux"}}
}

// Tools are the tools of the catalog, the bases first. Languages and
// command-line tools install through mise by default, the rest through
// the system's manager.
var Tools = []Entry{
	{ID: "homebrew", Group: Base, OS: []string{"darwin"}, Methods: []Method{
		// Its own signed package, which brings the Command Line Tools
		// too.
		{Manager: "installer", Package: "https://github.com/Homebrew/brew/releases/latest", OS: []string{"darwin"}},
	}},
	{ID: "mise", Group: Base, Methods: []Method{
		{Manager: "brew", Package: "mise", Kind: "formula"},
		{Manager: "winget", Package: "jdx.mise"},
		// Arch and Alpine package mise; elsewhere its own script puts it
		// in ~/.local/bin.
		{Manager: "pacman", Package: "mise"},
		{Manager: "apk", Package: "mise"},
		{Manager: "installer", Package: "https://mise.run", OS: []string{"linux"}},
	}},

	{ID: "node", Group: Language, Methods: mise("node")},
	{ID: "python", Group: Language, Methods: mise("python")},
	{ID: "go", Group: Language, Methods: mise("go")},
	// rustup is how Rust says to install it: it keeps the toolchains and
	// their components.
	{ID: "rust", Group: Language, Methods: []Method{
		unixInstaller("https://sh.rustup.rs"),
		{Manager: "winget", Package: "Rustlang.Rustup"},
	}},
	{ID: "bun", Group: Language, Methods: mise("bun")},
	// mise builds PHP from source, which takes libraries a computer
	// seldom has: the system's manager installs it. WinGet names it by
	// line, PHP.PHP.8.4: the line follows the name.
	{ID: "php", Group: Language, Methods: []Method{
		{Manager: "brew", Package: "php", Kind: "formula"},
		{Manager: "winget", Package: "PHP.PHP"},
		{Manager: "apt", Package: "php-cli"},
		{Manager: "dnf", Package: "php-cli"},
		{Manager: "pacman", Package: "php"},
		{Manager: "zypper", Package: "php8"},
	}},
	{ID: "java", Group: Language, Methods: mise("java")},
	{ID: "ruby", Group: Language, Methods: mise("ruby")},
	{ID: "dotnet", Group: Language, Methods: mise("dotnet")},
	{ID: "deno", Group: Language, Methods: mise("deno")},
	{ID: "zig", Group: Language, Methods: mise("zig")},
	{ID: "elixir", Group: Language, Methods: mise("elixir")},

	{ID: "pnpm", Group: Tool, Methods: mise("pnpm"), Needs: []string{"node"}},
	// Yarn's current releases install through Corepack alone.
	{ID: "yarn", Group: Tool, Methods: []Method{{Manager: "corepack", Package: "yarn"}}},
	{ID: "git", Group: Tool, Methods: []Method{
		{Manager: "xcode-select", Package: "Command Line Tools"},
		{Manager: "brew", Package: "git", Kind: "formula"},
		{Manager: "winget", Package: "Git.Git"},
		{Manager: "apt", Package: "git"},
		{Manager: "dnf", Package: "git"},
		{Manager: "pacman", Package: "git"},
		{Manager: "zypper", Package: "git"},
		{Manager: "apk", Package: "git"},
	}},
	// The engine runs containers. Colima, Podman, Rancher Desktop and the
	// engines of Linux are open source; Docker Desktop and OrbStack are
	// paid for work at larger companies.
	{ID: "docker", Group: Tool, Methods: engines(
		"Colima", "brew", "OrbStack", "brew", "Docker Desktop", "brew", "Podman Desktop", "brew", "Podman", "brew", "Rancher Desktop", "brew",
		"Docker Desktop", "winget",
		"Docker Engine", "apt", "Docker Engine", "dnf", "Docker Engine", "zypper", "Docker Engine", "pacman", "Docker Engine", "apk",
	)},
	{ID: "gh", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "gh", Kind: "formula"},
		{Manager: "winget", Package: "GitHub.cli"},
		{Manager: "apt", Package: "gh"},
		{Manager: "dnf", Package: "gh"},
		{Manager: "pacman", Package: "github-cli"},
		{Manager: "zypper", Package: "gh"},
		{Manager: "apk", Package: "github-cli"},
		{Manager: "mise", Package: "gh"},
	}},

	// Each language's own tools, after the language. mise has no
	// Composer and WinGet has no official Composer package: on Windows,
	// Chocolatey and Scoop install it when present, otherwise Composer's
	// official Windows installer runs for the current user.
	{ID: "composer", Group: Tool, Needs: []string{"php"}, Methods: []Method{
		{Manager: "brew", Package: "composer", Kind: "formula"},
		{Manager: "choco", Package: "composer"},
		{Manager: "scoop", Package: "composer"},
		{Manager: "installer", Package: "https://getcomposer.org/Composer-Setup.exe", OS: []string{"windows"}},
		{Manager: "apt", Package: "composer"},
		{Manager: "dnf", Package: "composer"},
		{Manager: "pacman", Package: "composer"},
		{Manager: "zypper", Package: "php-composer2"},
		{Manager: "apk", Package: "composer"},
	}},
	{ID: "uv", Group: Tool, Methods: mise("uv")},
	{ID: "pipx", Group: Tool, Methods: mise("pipx"), Needs: []string{"python"}},
	{ID: "bundler", Group: Tool, Methods: mise("gem:bundler"), Needs: []string{"ruby"}},
	{ID: "maven", Group: Tool, Methods: mise("maven"), Needs: []string{"java"}},
	{ID: "gradle", Group: Tool, Methods: mise("gradle"), Needs: []string{"java"}},

	// Kubernetes' tools; kind runs its clusters in the Docker engine,
	// which it needs only to create one.
	{ID: "kubectl", Group: Tool, Methods: mise("kubectl")},
	{ID: "helm", Group: Tool, Methods: mise("helm")},
	{ID: "kind", Group: Tool, Methods: mise("kind")},

	// Cloud CLIs and infrastructure tools: Homebrew on macOS, WinGet on
	// Windows, distro packages or mise on Linux; Terraform and OpenTofu
	// install through mise so projects can pin versions.
	{ID: "aws", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "awscli", Kind: "formula"},
		{Manager: "winget", Package: "Amazon.AWSCLI"},
		{Manager: "dnf", Package: "awscli2"},
		{Manager: "pacman", Package: "aws-cli-v2"},
		{Manager: "apk", Package: "aws-cli"},
		{Manager: "mise", Package: "awscli"},
	}},
	{ID: "gcloud", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "gcloud-cli", Kind: "cask"},
		{Manager: "winget", Package: "Google.CloudSDK"},
		{Manager: "mise", Package: "gcloud"},
	}},
	{ID: "az", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "azure-cli", Kind: "formula"},
		{Manager: "winget", Package: "Microsoft.AzureCLI"},
		{Manager: "dnf", Package: "azure-cli"},
		{Manager: "pacman", Package: "azure-cli"},
		{Manager: "zypper", Package: "azure-cli"},
		{Manager: "mise", Package: "azure-cli"},
	}},
	{ID: "terraform", Group: Tool, Methods: mise("terraform")},
	{ID: "tofu", Group: Tool, Methods: mise("opentofu")},

	// Everyday command-line utilities: Homebrew on macOS, WinGet on
	// Windows, distro packages or mise on Linux.
	{ID: "jq", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "jq", Kind: "formula"},
		{Manager: "winget", Package: "jqlang.jq"},
		{Manager: "apt", Package: "jq"},
		{Manager: "dnf", Package: "jq"},
		{Manager: "pacman", Package: "jq"},
		{Manager: "zypper", Package: "jq"},
		{Manager: "apk", Package: "jq"},
		{Manager: "mise", Package: "jq"},
	}},
	{ID: "ripgrep", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "ripgrep", Kind: "formula"},
		{Manager: "winget", Package: "BurntSushi.ripgrep.MSVC"},
		{Manager: "apt", Package: "ripgrep"},
		{Manager: "dnf", Package: "ripgrep"},
		{Manager: "pacman", Package: "ripgrep"},
		{Manager: "zypper", Package: "ripgrep"},
		{Manager: "apk", Package: "ripgrep"},
		{Manager: "mise", Package: "ripgrep"},
	}},
	{ID: "fzf", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "fzf", Kind: "formula"},
		{Manager: "winget", Package: "junegunn.fzf"},
		{Manager: "apt", Package: "fzf"},
		{Manager: "dnf", Package: "fzf"},
		{Manager: "pacman", Package: "fzf"},
		{Manager: "zypper", Package: "fzf"},
		{Manager: "apk", Package: "fzf"},
		{Manager: "mise", Package: "fzf"},
	}},
	{ID: "fd", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "fd", Kind: "formula"},
		{Manager: "winget", Package: "sharkdp.fd"},
		{Manager: "apt", Package: "fd-find"},
		{Manager: "dnf", Package: "fd-find"},
		{Manager: "pacman", Package: "fd"},
		{Manager: "zypper", Package: "fd"},
		{Manager: "apk", Package: "fd"},
		{Manager: "mise", Package: "fd"},
	}},
	{ID: "bat", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "bat", Kind: "formula"},
		{Manager: "winget", Package: "sharkdp.bat"},
		{Manager: "apt", Package: "bat"},
		{Manager: "dnf", Package: "bat"},
		{Manager: "pacman", Package: "bat"},
		{Manager: "zypper", Package: "bat"},
		{Manager: "apk", Package: "bat"},
		{Manager: "mise", Package: "bat"},
	}},
	{ID: "just", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "just", Kind: "formula"},
		{Manager: "winget", Package: "Casey.Just"},
		{Manager: "dnf", Package: "just"},
		{Manager: "pacman", Package: "just"},
		{Manager: "zypper", Package: "just"},
		{Manager: "apk", Package: "just"},
		{Manager: "mise", Package: "just"},
	}},
	{ID: "direnv", Group: Tool, OS: []string{"darwin", "linux"}, Methods: []Method{
		{Manager: "brew", Package: "direnv", Kind: "formula"},
		{Manager: "apt", Package: "direnv"},
		{Manager: "dnf", Package: "direnv"},
		{Manager: "pacman", Package: "direnv"},
		{Manager: "zypper", Package: "direnv"},
		{Manager: "apk", Package: "direnv"},
		{Manager: "mise", Package: "direnv"},
	}},
	{ID: "k9s", Group: Tool, Methods: []Method{
		{Manager: "brew", Package: "k9s", Kind: "formula"},
		{Manager: "winget", Package: "Derailed.k9s"},
		{Manager: "pacman", Package: "k9s"},
		{Manager: "apk", Package: "k9s"},
		{Manager: "mise", Package: "k9s"},
	}},

	// Database servers install through the system's manager, which
	// sets their service up; Homebrew's starts with brew services. On
	// Linux, APT packages create a server that runs as installed, while
	// DNF and pacman install the package without initializing or starting
	// the systemd service automatically. On Windows EDB's PostgreSQL and
	// MongoDB's installers set a service up: MySQL needs its configuration
	// wizard. MariaDB requires additional server setup and is not offered
	// on Windows; Redis has no Windows build of its own. MongoDB is in no
	// Linux distribution and in Homebrew only through MongoDB's tap.
	{ID: "postgresql", Group: Database, Methods: []Method{
		{Manager: "brew", Package: "postgresql@17", Kind: "formula"},
		{Manager: "winget", Package: "PostgreSQL.PostgreSQL.17"},
		{Manager: "apt", Package: "postgresql"},
		{Manager: "dnf", Package: "postgresql-server"},
		{Manager: "pacman", Package: "postgresql"},
	}},
	{ID: "redis", Group: Database, OS: []string{"darwin", "linux"}, Methods: []Method{
		{Manager: "brew", Package: "redis", Kind: "formula"},
		{Manager: "apt", Package: "redis-server"},
		{Manager: "dnf", Package: "redis"},
		{Manager: "pacman", Package: "redis"},
	}},
	{ID: "mysql", Group: Database, Conflicts: []string{"mariadb"}, Methods: []Method{
		{Manager: "brew", Package: "mysql@8.4", Kind: "formula"},
		{Manager: "apt", Package: "mysql-server"},
		{Manager: "dnf", Package: "community-mysql-server"},
		{Manager: "winget", Package: "Oracle.MySQL"},
	}},
	{ID: "mariadb", Group: Database, OS: []string{"darwin", "linux"}, Conflicts: []string{"mysql"}, Methods: []Method{
		{Manager: "brew", Package: "mariadb", Kind: "formula"},
		{Manager: "apt", Package: "mariadb-server"},
		{Manager: "dnf", Package: "mariadb-server"},
		{Manager: "pacman", Package: "mariadb"},
	}},
	{ID: "mongodb", Group: Database, OS: []string{"darwin", "windows"}, Methods: []Method{
		{Manager: "brew", Package: "mongodb/brew/mongodb-community", Kind: "formula"},
		{Manager: "winget", Package: "MongoDB.Server"},
	}},

	// Agents install through the system's manager first, as a tool that
	// is not a language, then npm. Claude Code's own installer comes
	// first: it is how Anthropic says to install it, and it updates
	// itself.
	{ID: "claude", Group: Agent, Methods: []Method{
		unixInstaller("https://claude.ai/install.sh"),
		{Manager: "brew", Package: "claude-code", Kind: "cask"},
		{Manager: "winget", Package: "Anthropic.ClaudeCode"},
		{Manager: "npm", Package: "@anthropic-ai/claude-code"},
	}},
	{ID: "codex", Group: Agent, Methods: []Method{
		{Manager: "brew", Package: "codex", Kind: "cask"},
		{Manager: "npm", Package: "@openai/codex"},
	}},
	{ID: "copilot", Group: Agent, Methods: []Method{
		{Manager: "brew", Package: "copilot-cli", Kind: "cask"},
		{Manager: "winget", Package: "GitHub.Copilot"},
		{Manager: "npm", Package: "@github/copilot"},
	}},
	{ID: "antigravity", Group: Agent, Methods: []Method{
		{Manager: "brew", Package: "antigravity-cli", Kind: "cask"},
		{Manager: "winget", Package: "Google.AntigravityCLI"},
		unixInstaller("https://antigravity.google/cli/install.sh"),
	}},
	{ID: "gemini", Group: Agent, Methods: []Method{
		{Manager: "brew", Package: "gemini-cli", Kind: "formula"},
		{Manager: "npm", Package: "@google/gemini-cli"},
	}},
	{ID: "cursor", Group: Agent, OS: []string{"darwin", "windows"}, Methods: []Method{
		{Manager: "brew", Package: "cursor", Kind: "cask"},
		{Manager: "winget", Package: "Anysphere.Cursor"},
	}},
	{ID: "opencode", Group: Agent, Methods: []Method{
		{Manager: "brew", Package: "opencode", Kind: "formula"},
		{Manager: "npm", Package: "opencode-ai"},
	}},
	{ID: "grok", Group: Agent, Methods: []Method{
		{Manager: "winget", Package: "xAI.GrokBuild"},
		{Manager: "npm", Package: "@xai-official/grok"},
	}},
	{ID: "kimi", Group: Agent, OS: []string{"windows"}, Methods: []Method{
		{Manager: "winget", Package: "MoonshotAI.KimiCodeCLI"},
	}},
}

// DatabaseKinds are the kinds of server, as inventory.DatabaseKind names
// them, of the catalog's databases.
var DatabaseKinds = map[string]string{"postgresql": "PostgreSQL", "redis": "Redis", "mysql": "MySQL", "mariadb": "MariaDB", "mongodb": "MongoDB"}

// Find returns the entry of the tool id.
func Find(id string) (Entry, bool) {
	i := slices.IndexFunc(Tools, func(e Entry) bool { return e.ID == id })
	if i < 0 {
		return Entry{}, false
	}
	return Tools[i], true
}

// AgentNpmPackages returns the npm package name for each catalog AI agent
// distributed through npm, keyed by agent ID.
func AgentNpmPackages() map[string]string {
	out := map[string]string{}
	for _, e := range Tools {
		if e.Group != Agent {
			continue
		}
		for _, m := range e.Methods {
			if m.Manager == "npm" && m.Package != "" {
				out[e.ID] = m.Package
				break
			}
		}
	}
	return out
}

// About says what the tool id is for, to someone new to it; "" for a
// tool not in the catalog.
func About(id string) string {
	about, ok := map[string]func() string{
		"homebrew":    locales.CatalogHomebrew,
		"mise":        locales.CatalogMise,
		"node":        locales.CatalogNode,
		"python":      locales.CatalogPython,
		"go":          locales.CatalogGo,
		"rust":        locales.CatalogRust,
		"bun":         locales.CatalogBun,
		"php":         locales.CatalogPhp,
		"java":        locales.CatalogJava,
		"ruby":        locales.CatalogRuby,
		"dotnet":      locales.CatalogDotnet,
		"deno":        locales.CatalogDeno,
		"zig":         locales.CatalogZig,
		"elixir":      locales.CatalogElixir,
		"composer":    locales.CatalogComposer,
		"uv":          locales.CatalogUv,
		"pipx":        locales.CatalogPipx,
		"bundler":     locales.CatalogBundler,
		"maven":       locales.CatalogMaven,
		"gradle":      locales.CatalogGradle,
		"kubectl":     locales.CatalogKubectl,
		"helm":        locales.CatalogHelm,
		"kind":        locales.CatalogKind,
		"aws":         locales.CatalogAws,
		"gcloud":      locales.CatalogGcloud,
		"az":          locales.CatalogAz,
		"terraform":   locales.CatalogTerraform,
		"tofu":        locales.CatalogTofu,
		"jq":          locales.CatalogJq,
		"ripgrep":     locales.CatalogRipgrep,
		"fzf":         locales.CatalogFzf,
		"fd":          locales.CatalogFd,
		"bat":         locales.CatalogBat,
		"just":        locales.CatalogJust,
		"direnv":      locales.CatalogDirenv,
		"k9s":         locales.CatalogK9s,
		"pnpm":        locales.CatalogPnpm,
		"yarn":        locales.CatalogYarn,
		"git":         locales.CatalogGit,
		"gh":          locales.CatalogGh,
		"docker":      locales.CatalogDocker,
		"claude":      locales.CatalogClaude,
		"codex":       locales.CatalogCodex,
		"copilot":     locales.CatalogCopilot,
		"gemini":      locales.CatalogGemini,
		"antigravity": locales.CatalogAntigravity,
		"cursor":      locales.CatalogCursor,
		"opencode":    locales.CatalogOpencode,
		"grok":        locales.CatalogGrok,
		"kimi":        locales.CatalogKimi,
		"postgresql":  locales.CatalogPostgresql,
		"redis":       locales.CatalogRedis,
		"mysql":       locales.CatalogMysql,
		"mariadb":     locales.CatalogMariadb,
		"mongodb":     locales.CatalogMongodb,
	}[id]
	if !ok {
		return ""
	}
	return about()
}
