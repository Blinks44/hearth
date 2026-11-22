package sysenv

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

// Paths reads PATH directories in their original order. Sources are possible
// declarations, not a trace of shell evaluation; no configuration is executed.
func Paths(env Env) []inventory.PathEntry {
	sources := pathSources(env)
	var out []inventory.PathEntry
	for _, dir := range filepath.SplitList(env.Get("PATH")) {
		if !pathEntryUsed(dir) {
			continue
		}
		e := inventory.PathEntry{Dir: dir, Sources: sources[pathKey(dir)]}
		if filepath.IsAbs(dir) {
			info, err := os.Stat(dir)
			e.Missing = errors.Is(err, os.ErrNotExist) || (err == nil && !info.IsDir())
		}
		out = append(out, e)
	}
	return out
}

// Variables returns only known tool settings. Arbitrary environment values,
// credentials and tokens must never enter the snapshot.
func Variables(term, gui Env) []inventory.EnvironmentVariable {
	var out []inventory.EnvironmentVariable
	for _, key := range []string{
		"JAVA_HOME", "GOPATH", "GOROOT", "GOTOOLCHAIN",
		"NVM_DIR", "FNM_DIR", "VOLTA_HOME", "MISE_DATA_DIR", "ASDF_DATA_DIR",
		"BUN_INSTALL", "PNPM_HOME", "COREPACK_HOME",
		"PYENV_ROOT", "VIRTUAL_ENV", "CONDA_PREFIX",
		"CARGO_HOME", "RUSTUP_HOME",
		"GEM_HOME", "GEM_PATH", "RBENV_ROOT",
		"DOTNET_ROOT",
		"OLLAMA_MODELS", "CLAUDE_CONFIG_DIR", "CODEX_HOME",
		"ANDROID_HOME", "FLUTTER_ROOT",
		"HTTP_PROXY", "HTTPS_PROXY",
	} {
		t, g := term.Get(key), gui.Get(key)
		if key == "HTTP_PROXY" || key == "HTTPS_PROXY" {
			t, g = sanitizeProxy(t), sanitizeProxy(g)
		}
		if t != "" || g != "" {
			out = append(out, inventory.EnvironmentVariable{Name: key, Terminal: t, GUI: g})
		}
	}
	return out
}

// sanitizeProxy masks credentials embedded in proxy URLs before they enter the snapshot.
func sanitizeProxy(raw string) string {
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil && u.Scheme != "" {
			if u.User == nil {
				return raw
			}
			u.User = nil
			return strings.Replace(u.String(), "://", "://"+inventory.Hidden+"@", 1)
		}
	}
	scheme, rest := "", raw
	if s, after, ok := strings.Cut(raw, "://"); ok {
		scheme, rest = s+"://", after
	}
	authority, tail := rest, ""
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		authority, tail = rest[:i], rest[i:]
	}
	if i := strings.LastIndex(authority, "@"); i >= 0 {
		return scheme + inventory.Hidden + "@" + authority[i+1:] + tail
	}
	return raw
}

// SamePath compares directory spelling using the platform's path rules.
func SamePath(a, b string) bool { return pathKey(a) == pathKey(b) }
