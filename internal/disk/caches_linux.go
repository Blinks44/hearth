package disk

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// defaults are where the tools keep their caches on Linux when no
// variable moves them: under XDG's directories.
func defaults(env sysenv.Env, home string) map[string]string {
	caches := or(env.Get("XDG_CACHE_HOME"), filepath.Join(home, ".cache"))
	data := or(env.Get("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	return map[string]string{
		"npm":        filepath.Join(home, ".npm", "_cacache"),
		"pnpm":       filepath.Join(data, "pnpm", "store"),
		"yarn":       filepath.Join(caches, "yarn"),
		"uv":         filepath.Join(caches, "uv"),
		"pip":        filepath.Join(caches, "pip"),
		"brew":       filepath.Join(caches, "Homebrew"),
		"go-build":   filepath.Join(caches, "go-build"),
		"composer":   filepath.Join(caches, "composer"),
		"ccache":     filepath.Join(caches, "ccache"),
		"pub":        filepath.Join(home, ".pub-cache"),
		"playwright": filepath.Join(caches, "ms-playwright"),
	}
}

func extraPlaywrightPaths(_ string) []string { return nil }

// cursorAppData returns the Cursor Electron application data directory on Linux.
func cursorAppData(env sysenv.Env, home string) string {
	config := or(env.Get("XDG_CONFIG_HOME"), filepath.Join(home, ".config"))
	return filepath.Join(config, "Cursor")
}

// opencodeCachePaths returns OpenCode's recreatable snapshot, log, and cache folders.
func opencodeCachePaths(env sysenv.Env, home string) []string {
	data := or(env.Get("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	cache := or(env.Get("XDG_CACHE_HOME"), filepath.Join(home, ".cache"))
	return []string{
		filepath.Join(data, "opencode", "snapshot"),
		filepath.Join(data, "opencode", "log"),
		filepath.Join(cache, "opencode"),
	}
}

// claudeVersionsDirs returns directories where Claude Code's native installer
// keeps downloaded binary versions.
func claudeVersionsDirs(env sysenv.Env, home string) []string {
	data := or(env.Get("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	return []string{filepath.Join(data, "claude", "versions")}
}
