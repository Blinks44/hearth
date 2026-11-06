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
	}
}
