package disk

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// defaults are where the tools keep their caches on macOS when no
// variable moves them.
func defaults(env sysenv.Env, home string) map[string]string {
	caches := filepath.Join(home, "Library", "Caches")
	return map[string]string{
		"npm":      filepath.Join(home, ".npm", "_cacache"),
		"pnpm":     filepath.Join(home, "Library", "pnpm", "store"),
		"yarn":     filepath.Join(caches, "Yarn"),
		"uv":       filepath.Join(or(env.Get("XDG_CACHE_HOME"), filepath.Join(home, ".cache")), "uv"),
		"pip":      filepath.Join(caches, "pip"),
		"brew":     filepath.Join(caches, "Homebrew"),
		"go-build": filepath.Join(caches, "go-build"),
		"composer": filepath.Join(caches, "composer"),
		"ccache":   filepath.Join(caches, "ccache"),
		// Xcode's build products and indexes, made again by the next build.
		"xcode":      filepath.Join(home, "Library", "Developer", "Xcode", "DerivedData"),
		"cocoapods":  filepath.Join(caches, "CocoaPods"),
		"pub":        filepath.Join(home, ".pub-cache"),
		"playwright": filepath.Join(caches, "ms-playwright"),
	}
}

// extraPlaywrightPaths are additional default locations where Playwright
// browsers may live on macOS when PLAYWRIGHT_BROWSERS_PATH is unset.
func extraPlaywrightPaths(home string) []string {
	return []string{filepath.Join(home, ".cache", "ms-playwright")}
}

// cursorAppData returns the Cursor Electron application data directory on macOS.
func cursorAppData(_ sysenv.Env, home string) string {
	return filepath.Join(home, "Library", "Application Support", "Cursor")
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
