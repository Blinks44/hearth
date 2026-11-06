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
		"npm":      filepath.Join(home, ".npm", "_cacache"),
		"pnpm":     filepath.Join(data, "pnpm", "store"),
		"yarn":     filepath.Join(caches, "yarn"),
		"uv":       filepath.Join(caches, "uv"),
		"pip":      filepath.Join(caches, "pip"),
		"brew":     filepath.Join(caches, "Homebrew"),
		"go-build": filepath.Join(caches, "go-build"),
		"composer": filepath.Join(caches, "composer"),
	}
}
