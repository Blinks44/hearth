package disk

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// cacheVars are the variables that move a cache from where its tool keeps
// it by default, by cache ID; the first one set wins.
var cacheVars = map[string][]string{
	"npm":      {"npm_config_cache", "NPM_CONFIG_CACHE"},
	"yarn":     {"YARN_CACHE_FOLDER"},
	"bun":      {"BUN_INSTALL_CACHE_DIR"},
	"uv":       {"UV_CACHE_DIR"},
	"pip":      {"PIP_CACHE_DIR"},
	"brew":     {"HOMEBREW_CACHE"},
	"go-build": {"GOCACHE"},
	"nuget":    {"NUGET_PACKAGES"},
	"composer": {"COMPOSER_CACHE_DIR"},
	"ccache":   {"CCACHE_DIR"},
	"pub":      {"PUB_CACHE"},
}

// cacheOrder is the order caches come in.
var cacheOrder = []string{
	"npm", "pnpm", "yarn", "yarn-berry", "bun",
	"uv", "pip", "brew", "go-mod", "go-build",
	"cargo", "rustup", "gradle", "maven", "nuget",
	"composer", "ccache", "xcode", "cocoapods", "pub", "playwright",
	"claude-cache", "codex-cache", "codex-worktrees",
	"gemini-cache", "copilot-cache", "opencode-cache", "cursor-cache",
}

// cursorElectronCaches are recreatable Electron cache and log folders inside
// Cursor's application data directory. Never includes User/, globalStorage/,
// workspaceStorage/, extensions/, rules, or skills.
var cursorElectronCaches = []string{
	"Cache",
	"CachedData",
	"CachedExtensions",
	"CachedExtensionVSIXs",
	"Code Cache",
	"GPUCache",
	"DawnGraphiteCache",
	"DawnWebGPUCache",
	"Crashpad",
	"logs",
}

// claudeTempDirs are safe recreatable temporary folders inside ~/.claude.
// Never includes settings.json, CLAUDE.md, skills, projects, or auth files.
var claudeTempDirs = []string{
	"file-history",
	"debug",
	"paste-cache",
	"shell-snapshots",
	"session-env",
	"telemetry",
	"statsig",
	"backups",
}

// codexTempDirs are safe recreatable temporary folders inside ~/.codex.
// Never includes config.toml, auth.json, sessions, skills, or AGENTS.md.
var codexTempDirs = []string{"log", "tmp", "shell_snapshots"}

// copilotTempDirs are safe recreatable temporary folders inside ~/.copilot.
var copilotTempDirs = []string{"logs", "pkg"}

// Caches returns the tools' caches that are on disk, by the terminal's
// environment and the snapshot's Go settings and agent installations.
func Caches(env sysenv.Env, snap *inventory.Snapshot) []inventory.Cache {
	home := sources.ManagerDirs(env).Home
	if home == "" {
		return nil
	}
	paths := defaults(env, home)
	paths["yarn-berry"] = filepath.Join(home, ".yarn", "berry", "cache")
	bun := env.Get("BUN_INSTALL")
	if bun == "" {
		bun = filepath.Join(home, ".bun")
	}
	paths["bun"] = filepath.Join(bun, "install", "cache")
	paths["gradle"] = filepath.Join(or(env.Get("GRADLE_USER_HOME"), filepath.Join(home, ".gradle")), "caches")
	paths["maven"] = filepath.Join(home, ".m2", "repository")
	paths["nuget"] = filepath.Join(home, ".nuget", "packages")
	// ccache keeps using its old ~/.ccache while it is there.
	if legacy := filepath.Join(home, ".ccache"); isDir(legacy) {
		paths["ccache"] = legacy
	}
	if goTool, ok := snap.Tool("go"); ok && goTool.Vars["GOMODCACHE"] != "" {
		paths["go-mod"] = goTool.Vars["GOMODCACHE"]
	}
	for id, names := range cacheVars {
		for _, name := range names {
			if v := env.Get(name); v != "" {
				paths[id] = v
				break
			}
		}
	}

	multi := map[string][]string{}
	cargoHome := or(env.Get("CARGO_HOME"), filepath.Join(home, ".cargo"))
	multi["cargo"] = []string{
		filepath.Join(cargoHome, "registry"),
		filepath.Join(cargoHome, "git"),
	}
	rustupHome := or(env.Get("RUSTUP_HOME"), filepath.Join(home, ".rustup"))
	multi["rustup"] = []string{
		filepath.Join(rustupHome, "downloads"),
		filepath.Join(rustupHome, "tmp"),
	}
	if pw := env.Get("PLAYWRIGHT_BROWSERS_PATH"); pw != "" && pw != "0" {
		multi["playwright"] = []string{pw}
	} else {
		pwPaths := make([]string, 0, 1+len(extraPlaywrightPaths(home)))
		if paths["playwright"] != "" {
			pwPaths = append(pwPaths, paths["playwright"])
		}
		pwPaths = append(pwPaths, extraPlaywrightPaths(home)...)
		multi["playwright"] = pwPaths
	}

	// Safe temporary AI agent caches (never configs, auth, skills, rules,
	// memory, or session databases).
	claudeDir := or(env.Get("CLAUDE_CONFIG_DIR"), filepath.Join(home, ".claude"))
	var claudePaths []string
	for _, sub := range claudeTempDirs {
		claudePaths = append(claudePaths, filepath.Join(claudeDir, sub))
	}
	claudePaths = append(claudePaths, oldClaudeVersions(env, home, snap)...)
	multi["claude-cache"] = claudePaths

	codexDir := or(env.Get("CODEX_HOME"), filepath.Join(home, ".codex"))
	var codexPaths []string
	for _, sub := range codexTempDirs {
		codexPaths = append(codexPaths, filepath.Join(codexDir, sub))
	}
	multi["codex-cache"] = codexPaths
	multi["codex-worktrees"] = []string{filepath.Join(codexDir, "worktrees")}

	multi["gemini-cache"] = geminiCachePaths(home)

	copilotDir := or(env.Get("COPILOT_HOME"), filepath.Join(home, ".copilot"))
	var copilotPaths []string
	for _, sub := range copilotTempDirs {
		copilotPaths = append(copilotPaths, filepath.Join(copilotDir, sub))
	}
	multi["copilot-cache"] = copilotPaths

	multi["opencode-cache"] = opencodeCachePaths(env, home)
	multi["cursor-cache"] = cursorCachePaths(env, home)

	var out []inventory.Cache
	seen := map[string]bool{}
	for _, id := range cacheOrder {
		candidates := multi[id]
		if len(candidates) == 0 && paths[id] != "" {
			candidates = []string{paths[id]}
		}
		allowFiles := id == "claude-cache"
		strictNoSymlink := isAgentCache(id)
		var kept []string
		for _, p := range candidates {
			if p == "" || seen[p] || !filepath.IsAbs(p) {
				continue
			}
			if strictNoSymlink && isSymlink(p) {
				continue
			}
			if allowFiles {
				if !isEntry(p) {
					continue
				}
			} else if !isDir(p) {
				continue
			}
			seen[p] = true
			kept = append(kept, p)
		}
		if len(kept) == 0 {
			continue
		}
		c := inventory.Cache{ID: id, Path: kept[0], OptIn: id == "codex-worktrees"}
		if len(kept) > 1 {
			c.Paths = kept
		}
		out = append(out, c)
	}
	// A JetBrains IDE's caches and indexes, made again when it opens a
	// project, one folder each.
	if ed, ok := snap.Tool("editors"); ok && ed.Editors != nil {
		for _, e := range ed.Editors.Editors {
			if e.Caches != "" && !seen[e.Caches] && isDir(e.Caches) {
				seen[e.Caches] = true
				out = append(out, inventory.Cache{ID: "jetbrains", Path: e.Caches})
			}
		}
	}
	return out
}

func isAgentCache(id string) bool {
	switch id {
	case "claude-cache", "codex-cache", "codex-worktrees",
		"gemini-cache", "copilot-cache", "opencode-cache", "cursor-cache":
		return true
	}
	return false
}

func geminiCachePaths(home string) []string {
	geminiDir := filepath.Join(home, ".gemini")
	out := []string{filepath.Join(geminiDir, "cache")}
	tmpDir := filepath.Join(geminiDir, "tmp")
	if isSymlink(tmpDir) || !isDir(tmpDir) {
		return out
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		sub := filepath.Join(tmpDir, entry.Name())
		for _, name := range []string{"logs", "checkpoints", "shell-snapshots"} {
			out = append(out, filepath.Join(sub, name))
		}
	}
	return out
}

func cursorCachePaths(env sysenv.Env, home string) []string {
	out := []string{filepath.Join(home, ".cursor", "logs")}
	appData := cursorAppData(env, home)
	if appData == "" || isSymlink(appData) {
		return out
	}
	for _, sub := range cursorElectronCaches {
		out = append(out, filepath.Join(appData, sub))
	}
	return out
}

// oldClaudeVersions returns old native installer versions in
// ~/.local/share/claude/versions/*, always protecting any active version
// referenced by Claude installations or ~/.local/bin/claude, and falling back
// to protecting the newest entry so versions/ is never emptied.
func oldClaudeVersions(env sysenv.Env, home string, snap *inventory.Snapshot) []string {
	active := map[string]bool{}
	markActive := func(p string) {
		if p == "" {
			return
		}
		active[filepath.Clean(p)] = true
		if real, err := filepath.EvalSymlinks(p); err == nil && real != "" {
			active[filepath.Clean(real)] = true
		}
	}
	if snap != nil {
		for _, ag := range snap.Agents {
			if ag.ID != "claude" {
				continue
			}
			for _, in := range ag.Installations {
				markActive(in.Path)
				markActive(in.RealPath)
			}
		}
	}
	for _, name := range []string{"claude", "claude.exe"} {
		markActive(filepath.Join(home, ".local", "bin", name))
	}

	var out []string
	for _, dir := range claudeVersionsDirs(env, home) {
		if dir == "" || isSymlink(dir) || !isDir(dir) {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		type verEntry struct {
			name    string
			path    string
			modTime int64
			active  bool
		}
		var list []verEntry
		for _, e := range entries {
			if e.Type()&os.ModeSymlink != 0 {
				continue
			}
			p := filepath.Join(dir, e.Name())
			clean := filepath.Clean(p)
			isAct := active[clean]
			if !isAct {
				if real, err := filepath.EvalSymlinks(p); err == nil && active[filepath.Clean(real)] {
					isAct = true
				}
			}
			if !isAct {
				for act := range active {
					if withinDir(act, clean) {
						isAct = true
						break
					}
				}
			}
			var mt int64
			if info, err := e.Info(); err == nil {
				mt = info.ModTime().UnixNano()
			}
			list = append(list, verEntry{name: e.Name(), path: p, modTime: mt, active: isAct})
		}
		if len(list) <= 1 {
			continue
		}
		// Always protect the newest version by semver (or mtime if unparseable),
		// in addition to any active binary path, so staged updates are never deleted.
		best := 0
		for i := 1; i < len(list); i++ {
			if compareClaudeVersion(list[i].name, list[i].modTime, list[best].name, list[best].modTime) > 0 {
				best = i
			}
		}
		list[best].active = true
		for _, item := range list {
			if !item.active {
				out = append(out, item.path)
			}
		}
	}
	slices.Sort(out)
	return out
}

func compareClaudeVersion(aName string, aMod int64, bName string, bMod int64) int {
	av := "v" + strings.TrimPrefix(aName, "v")
	bv := "v" + strings.TrimPrefix(bName, "v")
	if semver.IsValid(av) && semver.IsValid(bv) {
		if c := semver.Compare(av, bv); c != 0 {
			return c
		}
	}
	return cmp.Or(cmp.Compare(aMod, bMod), cmp.Compare(aName, bName))
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isEntry(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}
