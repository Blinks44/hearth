//go:build linux

package aiquota

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func xdgConfigHome(home string, env sysenv.Env) string {
	if v := env.Get("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config")
}

func xdgDataHome(home string, env sysenv.Env) string {
	if v := env.Get("XDG_DATA_HOME"); v != "" {
		return v
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".local", "share")
}

func xdgCacheHome(home string, env sysenv.Env) string {
	if v := env.Get("XDG_CACHE_HOME"); v != "" {
		return v
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".cache")
}

func cursorStateDBPath(home string, env sysenv.Env) string {
	cfg := xdgConfigHome(home, env)
	if cfg == "" {
		return ""
	}
	return filepath.Join(cfg, "Cursor", "User", "globalStorage", "state.vscdb")
}

func windsurfStateDBPath(home string, env sysenv.Env) string {
	cfg := xdgConfigHome(home, env)
	if cfg == "" {
		return ""
	}
	return filepath.Join(cfg, "Windsurf", "User", "globalStorage", "state.vscdb")
}

func jetBrainsConfigRoots(home string, env sysenv.Env) []string {
	cfg := xdgConfigHome(home, env)
	if cfg == "" {
		return nil
	}
	return []string{
		filepath.Join(cfg, "JetBrains"),
		filepath.Join(cfg, "Google"),
	}
}

func jetBrainsLogRoots(home string, env sysenv.Env) []string {
	cache := xdgCacheHome(home, env)
	if cache == "" {
		return nil
	}
	return []string{
		filepath.Join(cache, "JetBrains"),
		filepath.Join(cache, "Google"),
	}
}

func kiroDBPath(home string, env sysenv.Env) string {
	data := xdgDataHome(home, env)
	if data == "" {
		return ""
	}
	return filepath.Join(data, "kiro-cli", "data.sqlite3")
}

// readKeychainSecret attempts a read-only lookup via freedesktop secret-tool
// on Linux when available; returns ("", false) silently if not installed.
func readKeychainSecret(ctx context.Context, runner run.Runner, service, account string) (string, bool) {
	if runner == nil || service == "" {
		return "", false
	}
	args := []string{"secret-tool", "lookup", "service", service}
	if account != "" {
		args = append(args, "username", account)
	}
	res, err := runner.Run(ctx, run.Cmd{
		Args:      args,
		Timeout:   2 * time.Second,
		MaxOutput: 64 << 10,
	})
	if err != nil || res.ExitCode != 0 {
		return "", false
	}
	val := strings.TrimSpace(string(res.Stdout))
	return val, val != ""
}

func hasKeychainSecret(ctx context.Context, runner run.Runner, service, account string) bool {
	if runner == nil || service == "" {
		return false
	}
	args := []string{"secret-tool", "search", "service", service}
	if account != "" {
		args = append(args, "username", account)
	}
	res, err := runner.Run(ctx, run.Cmd{
		Args:      args,
		Timeout:   2 * time.Second,
		MaxOutput: 16 << 10,
	})
	return err == nil && res.ExitCode == 0 && len(strings.TrimSpace(string(res.Stdout)+string(res.Stderr))) > 0
}
