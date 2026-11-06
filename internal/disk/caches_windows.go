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
		"npm":      filepath.Join(local, "npm-cache", "_cacache"),
		"pnpm":     filepath.Join(local, "pnpm", "store"),
		"yarn":     filepath.Join(local, "Yarn", "Cache"),
		"uv":       filepath.Join(local, "uv", "cache"),
		"pip":      filepath.Join(local, "pip", "Cache"),
		"go-build": filepath.Join(local, "go-build"),
		"composer": filepath.Join(local, "Composer"),
		"ccache":   filepath.Join(local, "ccache"),
	}
}
