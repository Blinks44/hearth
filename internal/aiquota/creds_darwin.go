//go:build darwin

package aiquota

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func cursorStateDBPath(home string, _ sysenv.Env) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")
}

func windsurfStateDBPath(home string, _ sysenv.Env) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "Windsurf", "User", "globalStorage", "state.vscdb")
}

func jetBrainsConfigRoots(home string, _ sysenv.Env) []string {
	if home == "" {
		return nil
	}
	base := filepath.Join(home, "Library", "Application Support")
	return []string{
		filepath.Join(base, "JetBrains"),
		filepath.Join(base, "Google"),
	}
}

func jetBrainsLogRoots(home string, _ sysenv.Env) []string {
	if home == "" {
		return nil
	}
	base := filepath.Join(home, "Library", "Logs")
	return []string{
		filepath.Join(base, "JetBrains"),
		filepath.Join(base, "Google"),
	}
}

func kiroDBPath(home string, _ sysenv.Env) string {
	if home == "" {
		return ""
	}
	return filepath.Join(home, "Library", "Application Support", "kiro-cli", "data.sqlite3")
}

func readKeychainSecret(ctx context.Context, runner run.Runner, service, account string) (string, bool) {
	if runner == nil || service == "" {
		return "", false
	}
	args := []string{"/usr/bin/security", "find-generic-password", "-s", service}
	if account != "" {
		args = append(args, "-a", account)
	}
	args = append(args, "-w")
	res, err := runner.Run(ctx, run.Cmd{
		Args:      args,
		Timeout:   3 * time.Second,
		MaxOutput: 64 << 10,
	})
	if err != nil || res.ExitCode != 0 {
		return "", false
	}
	val := strings.TrimSpace(string(res.Stdout))
	return val, val != ""
}

// hasKeychainSecret checks whether a Keychain item exists without passing -w
// (so the secret value itself is never read or decrypted).
func hasKeychainSecret(ctx context.Context, runner run.Runner, service, account string) bool {
	if runner == nil || service == "" {
		return false
	}
	args := []string{"/usr/bin/security", "find-generic-password", "-s", service}
	if account != "" {
		args = append(args, "-a", account)
	}
	res, err := runner.Run(ctx, run.Cmd{
		Args:      args,
		Timeout:   3 * time.Second,
		MaxOutput: 16 << 10,
	})
	return err == nil && res.ExitCode == 0
}
