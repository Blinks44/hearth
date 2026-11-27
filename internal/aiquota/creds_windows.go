//go:build windows

package aiquota

import (
	"context"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func appDataDir(home string, env sysenv.Env) string {
	if v := env.Get("APPDATA"); v != "" {
		return v
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, "AppData", "Roaming")
}

func localAppDataDir(home string, env sysenv.Env) string {
	if v := env.Get("LOCALAPPDATA"); v != "" {
		return v
	}
	if home == "" {
		return ""
	}
	return filepath.Join(home, "AppData", "Local")
}

func cursorStateDBPath(home string, env sysenv.Env) string {
	base := appDataDir(home, env)
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Cursor", "User", "globalStorage", "state.vscdb")
}

func windsurfStateDBPath(home string, env sysenv.Env) string {
	base := appDataDir(home, env)
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Windsurf", "User", "globalStorage", "state.vscdb")
}

func jetBrainsConfigRoots(home string, env sysenv.Env) []string {
	base := appDataDir(home, env)
	if base == "" {
		return nil
	}
	return []string{
		filepath.Join(base, "JetBrains"),
		filepath.Join(base, "Google"),
	}
}

func jetBrainsLogRoots(home string, env sysenv.Env) []string {
	base := localAppDataDir(home, env)
	if base == "" {
		return nil
	}
	return []string{
		filepath.Join(base, "JetBrains"),
		filepath.Join(base, "Google"),
	}
}

func kiroDBPath(home string, env sysenv.Env) string {
	base := localAppDataDir(home, env)
	if base == "" {
		return ""
	}
	return filepath.Join(base, "kiro-cli", "data.sqlite3")
}

// readKeychainSecret is a no-op on Windows where supported CLI tools store
// OAuth credentials in user-profile files rather than macOS Keychain.
func readKeychainSecret(_ context.Context, _ run.Runner, _, _ string) (string, bool) {
	return "", false
}

func hasKeychainSecret(_ context.Context, _ run.Runner, _, _ string) bool {
	return false
}
