package inventory

import (
	"path/filepath"
	"regexp"
	"strings"
)

// ListenerRole categorizes an observed TCP listening process.
type ListenerRole int

const (
	RoleOther ListenerRole = iota
	RoleDevServer
	RoleDatabase
	RoleKubeForward
	RoleIDE
	RoleSystem
)

// DevServerInfo is the static classification of a listening process.
type DevServerInfo struct {
	// Kind is the recognized framework or runtime label (e.g. "Next.js", "Vite", "Go").
	Kind string
	// Role is the high-level category of the process.
	Role ListenerRole
	// HTTPOpenable reports whether opening http(s)://localhost:<port> makes sense.
	HTTPOpenable bool
}

var (
	secretKeyPattern   = regexp.MustCompile(`(?i)(?:^|[_-])(token|secret|password|passwd|pwd|api[_-]?key|access[_-]?key|private[_-]?key|auth|bearer|credential|cookie|session[_-]?key|dsn|connection[_-]?string)$`)
	urlUserPassPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^/\s@:]*):([^/\s@]+)@`)
	urlTokenPattern    = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)([^/\s@:]+)@`)
	authHeaderPattern  = regexp.MustCompile(`(?i)\b(bearer|basic)\s+\S+`)
)

func isPortToken(s string) bool {
	if len(s) == 0 || len(s) > 5 {
		return false
	}
	n := 0
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n >= 1 && n <= 65535
}

// RedactArgs returns a copy of command-line arguments with likely secret values
// replaced by "[redacted]". Flag names and non-secret paths are preserved so
// Hearth can classify dev servers without retaining credentials.
func RedactArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	redactNext := false
	for i, arg := range args {
		if i > 32 {
			break
		}
		if redactNext {
			out = append(out, "[redacted]")
			redactNext = false
			continue
		}
		// URLs with embedded user:pass@host, :pass@host, or token@host.
		if strings.Contains(arg, "://") && strings.Contains(arg, "@") {
			if urlUserPassPattern.MatchString(arg) {
				arg = urlUserPassPattern.ReplaceAllString(arg, `${1}${2}:[redacted]@`)
			} else {
				arg = urlTokenPattern.ReplaceAllString(arg, `${1}[redacted]@`)
			}
		}
		// --flag=value or KEY=value.
		if key, _, hasEq := strings.Cut(arg, "="); hasEq {
			cleanKey := strings.TrimLeft(key, "-")
			if secretKeyPattern.MatchString(cleanKey) {
				out = append(out, key+"=[redacted]")
				continue
			}
		}
		// Standalone secret flag whose value is the next argument.
		// Preserve numeric ports after -p (e.g. "next dev -p 3000").
		if strings.HasPrefix(arg, "-") {
			cleanFlag := strings.TrimLeft(arg, "-")
			if secretKeyPattern.MatchString(cleanFlag) {
				out = append(out, arg)
				redactNext = true
				continue
			}
			if cleanFlag == "p" {
				out = append(out, arg)
				if i+1 < len(args) && !isPortToken(args[i+1]) {
					redactNext = true
				}
				continue
			}
			// Attached mysql-style -pSECRET when not a numeric port.
			if strings.HasPrefix(arg, "-p") && len(arg) > 2 && !isPortToken(arg[2:]) {
				out = append(out, "-p[redacted]")
				continue
			}
		}
		// Inline or header-embedded "Bearer <token>" or "Basic <token>".
		if authHeaderPattern.MatchString(arg) {
			arg = authHeaderPattern.ReplaceAllString(arg, `${1} [redacted]`)
		}
		out = append(out, arg)
	}
	return out
}

// nonHTTPPorts are common database, broker, mail, and SSH ports.
var nonHTTPPorts = map[string]bool{
	"22": true, "25": true, "53": true, "110": true, "143": true,
	"465": true, "587": true, "993": true, "995": true,
	"1433": true, "1521": true, "2181": true, "2375": true, "2376": true,
	"3306": true, "33060": true, "4222": true, "5432": true, "5433": true,
	"5672": true, "6379": true, "6380": true, "6432": true, "9042": true,
	"9092": true, "11211": true, "27017": true, "27018": true, "28017": true,
}

var dbProcessNames = map[string]string{
	"postgres":      "PostgreSQL",
	"mysqld":        "MySQL",
	"mariadbd":      "MariaDB",
	"redis-server":  "Redis",
	"valkey-server": "Redis",
	"mongod":        "MongoDB",
	"sqlservr":      "SQL Server",
	"memcached":     "Memcached",
}

// ClassifyDevServer inspects a listener's process name, executable path, and
// redacted arguments to identify dev servers, databases, IDE helpers, and
// whether the port is suitable to open in a browser.
func ClassifyDevServer(l DatabaseListener) DevServerInfo {
	proc := strings.ToLower(strings.TrimSuffix(l.Process, ".exe"))
	pathLower := strings.ToLower(filepath.ToSlash(l.Path))
	joined := strings.ToLower(strings.Join(l.Args, " "))

	// Database servers.
	if kind, ok := dbProcessNames[proc]; ok {
		return DevServerInfo{Kind: kind, Role: RoleDatabase, HTTPOpenable: false}
	}

	// kubectl port-forward.
	if proc == "kubectl" && strings.Contains(joined, "port-forward") {
		return DevServerInfo{Kind: "kubectl", Role: RoleKubeForward, HTTPOpenable: !nonHTTPPorts[l.Port]}
	}

	// macOS AirPlay / system daemons.
	if proc == "controlcenter" && (l.Port == "5000" || l.Port == "7000") {
		return DevServerInfo{Kind: "AirPlay", Role: RoleSystem, HTTPOpenable: false}
	}
	if strings.HasPrefix(pathLower, "/system/") || strings.HasPrefix(pathLower, "/usr/libexec/") ||
		strings.HasPrefix(pathLower, "/usr/sbin/") || strings.HasPrefix(pathLower, "/sbin/") ||
		proc == "rapportd" || proc == "sharingd" || proc == "mdnsresponder" || proc == "cupsd" {
		return DevServerInfo{Role: RoleSystem, HTTPOpenable: false}
	}

	// IDE / editor / LSP background helpers (not user dev servers).
	if isIDEHelper(proc, pathLower, joined) {
		return DevServerInfo{Role: RoleIDE, HTTPOpenable: false}
	}

	// Specific web/dev frameworks from command line args.
	if kind := matchDevFramework(proc, pathLower, joined); kind != "" {
		return DevServerInfo{Kind: kind, Role: RoleDevServer, HTTPOpenable: !nonHTTPPorts[l.Port]}
	}

	// Generic runtimes running inside a user directory (likely a dev server).
	if kind := matchDevRuntime(proc); kind != "" {
		return DevServerInfo{Kind: kind, Role: RoleDevServer, HTTPOpenable: !nonHTTPPorts[l.Port]}
	}

	// Go compiled dev binary (e.g. `go run` writes into go-build temp cache, or binary inside a project dir).
	if strings.Contains(pathLower, "/go-build") || strings.Contains(pathLower, "/tmp/air") {
		return DevServerInfo{Kind: "Go", Role: RoleDevServer, HTTPOpenable: !nonHTTPPorts[l.Port]}
	}

	return DevServerInfo{
		Role:         RoleOther,
		HTTPOpenable: !nonHTTPPorts[l.Port],
	}
}

func isIDEHelper(proc, pathLower, joined string) bool {
	if strings.Contains(pathLower, ".app/contents/") {
		for _, ide := range []string{
			"visual studio code", "cursor.app", "windsurf.app", "zed.app",
			"jetbrains", "goland", "phpstorm", "webstorm", "intellij", "pycharm", "clion", "rider",
			"antigravity", "raycast", "spotify", "slack", "discord", "chrome", "firefox", "safari",
			"figma", "linear", "1password", "dropbox", "docker desktop", "orbstack",
		} {
			if strings.Contains(pathLower, ide) {
				return true
			}
		}
	}
	for _, dir := range []string{
		"/application support/raycast", "/application support/com.raycast",
		"/application support/code", "/application support/cursor", "/application support/windsurf",
		"/application support/jetbrains", "/application support/claude", "/application support/antigravity",
		"/raycast/",
	} {
		if strings.Contains(pathLower, dir) {
			return true
		}
	}
	for _, marker := range []string{
		"language-server", "language_server", "lsp-server", "gopls", "rust-analyzer",
		"typescript-language-server", "pyright", "pylsp", "clangd", "eslint_d",
		"code helper", "electron helper", "jetbrains", "extensionhost", "mcp-server",
		"raycast",
	} {
		if strings.Contains(proc, marker) || strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

func matchDevFramework(proc, pathLower, joined string) string {
	type rule struct {
		kind    string
		needles []string
	}
	rules := []rule{
		{"Next.js", []string{"next-server", "/.bin/next", " next dev", " next start", "next/dist/bin/next"}},
		{"Nuxt", []string{"/.bin/nuxt", "/.bin/nuxi", " nuxt dev", " nuxi dev", "nuxt/dist"}},
		{"Astro", []string{"/.bin/astro", " astro dev", " astro preview"}},
		{"SvelteKit", []string{"svelte-kit", "@sveltejs/kit"}},
		{"Remix", []string{"/.bin/remix", " remix vite:dev", " remix dev"}},
		{"Storybook", []string{"storybook", "start-storybook"}},
		{"Rsbuild", []string{"/.bin/rsbuild", "rsbuild/bin/", " rsbuild "}},
		{"Rspress", []string{"/.bin/rspress", "rspress/bin/", " rspress "}},
		{"VitePress", []string{"/.bin/vitepress", "vitepress/bin/", " vitepress "}},
		{"Docusaurus", []string{"/.bin/docusaurus", "@docusaurus/core"}},
		{"Vite", []string{"/.bin/vite", "vite/bin/vite", " vite "}},
		{"Webpack", []string{"webpack-dev-server", "webpack serve", "/.bin/webpack"}},
		{"Turbopack", []string{"turbopack"}},
		{"Parcel", []string{"/.bin/parcel", " parcel "}},
		{"Angular", []string{"/.bin/ng", "@angular/cli", " ng serve"}},
		{"Expo", []string{"/.bin/expo", " expo start", "@expo/cli"}},
		{"Uvicorn", []string{"uvicorn"}},
		{"Gunicorn", []string{"gunicorn"}},
		{"Django", []string{"manage.py runserver", "django"}},
		{"Flask", []string{"flask run", "/flask"}},
		{"Rails", []string{"rails server", "rails s ", "puma", "bin/rails"}},
		{"Spring", []string{"spring-boot", "org.springframework"}},
		{"Air", []string{"/bin/air", " air "}},
		{"Hugo", []string{"hugo server"}},
		{"Jekyll", []string{"jekyll serve"}},
		{"Laravel", []string{"artisan serve"}},
		{"Symfony", []string{"symfony server"}},
		{"Phoenix", []string{"phx.server"}},
	}
	haystack := " " + joined + " "
	for _, r := range rules {
		if proc == strings.ToLower(r.kind) {
			return r.kind
		}
		for _, n := range r.needles {
			if strings.Contains(haystack, n) || strings.Contains(pathLower, n) {
				return r.kind
			}
		}
	}
	return ""
}

func matchDevRuntime(proc string) string {
	switch proc {
	case "node", "nodejs":
		return "Node"
	case "bun":
		return "Bun"
	case "deno":
		return "Deno"
	case "python", "python3":
		return "Python"
	case "ruby":
		return "Ruby"
	case "php":
		return "PHP"
	case "go":
		return "Go"
	case "cargo":
		return "Rust"
	case "dotnet":
		return ".NET"
	case "beam.smp", "elixir":
		return "Elixir"
	case "java":
		return "Java"
	}
	if strings.HasPrefix(proc, "python3.") {
		return "Python"
	}
	return ""
}
