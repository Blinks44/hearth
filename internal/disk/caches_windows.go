package disk

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// defaults are where the tools keep their caches on Windows when no
// variable moves them: under %LOCALAPPDATA%.
func defaults(env sysenv.Env, _ string) map[string]string {
	local := env.Get("LOCALAPPDATA")
	if local == "" {
		return map[string]string{}
	}
	return map[string]string{
		"npm":        filepath.Join(local, "npm-cache", "_cacache"),
		"pnpm":       filepath.Join(local, "pnpm", "store"),
		"yarn":       filepath.Join(local, "Yarn", "Cache"),
		"uv":         filepath.Join(local, "uv", "cache"),
		"pip":        filepath.Join(local, "pip", "Cache"),
		"go-build":   filepath.Join(local, "go-build"),
		"composer":   filepath.Join(local, "Composer"),
		"ccache":     filepath.Join(local, "ccache"),
		"pub":        filepath.Join(local, "Pub", "Cache"),
		"playwright": filepath.Join(local, "ms-playwright"),
	}
}

func extraPlaywrightPaths(home string) []string {
	return []string{filepath.Join(home, ".cache", "ms-playwright")}
}

// cursorAppData returns the Cursor Electron application data directory on Windows.
func cursorAppData(env sysenv.Env, home string) string {
	if appData := env.Get("APPDATA"); appData != "" {
		return filepath.Join(appData, "Cursor")
	}
	return filepath.Join(home, "AppData", "Roaming", "Cursor")
}

// opencodeCachePaths returns OpenCode's recreatable snapshot, log, and cache folders.
func opencodeCachePaths(env sysenv.Env, home string) []string {
	data := or(env.Get("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	cache := or(env.Get("XDG_CACHE_HOME"), filepath.Join(home, ".cache"))
	out := []string{
		filepath.Join(data, "opencode", "snapshot"),
		filepath.Join(data, "opencode", "log"),
		filepath.Join(cache, "opencode"),
	}
	if local := env.Get("LOCALAPPDATA"); local != "" {
		out = append(out,
			filepath.Join(local, "opencode", "snapshot"),
			filepath.Join(local, "opencode", "log"),
			filepath.Join(local, "opencode", "cache"),
		)
	}
	return out
}

// claudeVersionsDirs returns directories where Claude Code's native installer
// keeps downloaded binary versions.
func claudeVersionsDirs(env sysenv.Env, home string) []string {
	data := or(env.Get("XDG_DATA_HOME"), filepath.Join(home, ".local", "share"))
	out := []string{filepath.Join(data, "claude", "versions")}
	if local := env.Get("LOCALAPPDATA"); local != "" {
		out = append(out, filepath.Join(local, "claude", "versions"))
	}
	return out
}
