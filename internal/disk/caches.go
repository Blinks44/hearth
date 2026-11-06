package disk

import (
	"os"
	"path/filepath"

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
	"composer": {"COMPOSER_CACHE_DIR"},
}

// cacheOrder is the order caches come in.
var cacheOrder = []string{"npm", "pnpm", "yarn", "yarn-berry", "bun", "uv", "pip", "brew", "go-mod", "go-build", "cargo", "gradle", "maven", "composer"}

// Caches returns the tools' caches that are on disk, by the terminal's
// environment and the snapshot's Go settings.
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
	paths["cargo"] = filepath.Join(or(env.Get("CARGO_HOME"), filepath.Join(home, ".cargo")), "registry")
	paths["gradle"] = filepath.Join(or(env.Get("GRADLE_USER_HOME"), filepath.Join(home, ".gradle")), "caches")
	paths["maven"] = filepath.Join(home, ".m2", "repository")
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
	var out []inventory.Cache
	seen := map[string]bool{}
	for _, id := range cacheOrder {
		path := paths[id]
		if path == "" || seen[path] || !filepath.IsAbs(path) {
			continue
		}
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			continue
		}
		seen[path] = true
		out = append(out, inventory.Cache{ID: id, Path: path})
	}
	return out
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
