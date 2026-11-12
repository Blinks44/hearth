//go:build !windows

package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// The scripts below download a maker's installer and run it, failing as
// the download does: a pipe into the shell would run what half came.
// They are fixed. A new Ubuntu has wget, not curl: both installers take
// either.
const (
	// rustupScript installs rustup and Rust's stable toolchain without
	// changing the startup files: Hearth's block does that.
	rustupScript = `set -e
f=$(mktemp)
if command -v curl >/dev/null 2>&1; then curl --proto '=https' --tlsv1.2 -sSf -o "$f" https://sh.rustup.rs
else wget --https-only --secure-protocol=TLSv1_2 -qO "$f" https://sh.rustup.rs; fi
sh "$f" -y --no-modify-path
rm -f "$f"`
	// claudeScript runs Claude Code's own installer, which puts claude in
	// ~/.local/bin.
	claudeScript = `set -e
f=$(mktemp)
if command -v curl >/dev/null 2>&1; then curl -fsSL -o "$f" https://claude.ai/install.sh
else wget -qO "$f" https://claude.ai/install.sh; fi
bash "$f"
rm -f "$f"`
)

// installerSteps are the steps that install the tool id with its maker's
// installer, and put what it installs on the terminal's PATH; false when
// Hearth has none for it.
func installerSteps(id string, env sysenv.Env) ([]Step, bool) {
	var steps []Step
	var posix, fish string
	switch id {
	case "rust":
		steps = []Step{{Argv: []string{"/bin/sh", "-c", rustupScript}, Note: locales.SetupRustup()}}
		posix, fish = `. "$HOME/.cargo/env"`, `source "$HOME/.cargo/env.fish"`
	case "claude":
		steps = []Step{{Argv: []string{"/bin/sh", "-c", claudeScript}, Note: locales.SetupClaudeInstaller()}}
		bin := filepath.Join(env.Get("HOME"), ".local", "bin")
		if slices.ContainsFunc(filepath.SplitList(env.Get("PATH")), func(d string) bool { return sysenv.SamePath(d, bin) }) {
			return steps, true
		}
		posix, fish = `export PATH="$HOME/.local/bin:$PATH"`, `fish_add_path $HOME/.local/bin`
	default:
		return nil, false
	}
	edits, err := startupEdits(env, posix, fish)
	if err != nil {
		return nil, false
	}
	return append(steps, edits...), true
}

// startupEdits are the steps that add a line to Hearth's block in the
// user's shell's startup files, as mise's hook goes: the interactive
// shell's and the login shell's; fish's one file takes the fish line.
func startupEdits(env sysenv.Env, posix, fish string) ([]Step, error) {
	shell := filepath.Base(env.Get("SHELL"))
	files, line := []string{rcFile(env, shell)}, posix
	switch shell {
	case "zsh":
		files = append(files, filepath.Join(filepath.Dir(files[0]), ".zprofile"))
	case "bash":
		files = append(files, filepath.Join(env.Get("HOME"), ".profile"))
	case "fish":
		line = fish
	default:
		return nil, errors.New(locales.SetupShellUnsupported(shell))
	}
	var steps []Step
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		if after := AddToBlock(string(data), line); after != string(data) {
			steps = append(steps, Step{Edit: &FileEdit{Path: file, Before: string(data), After: after, New: err != nil}})
		}
	}
	return steps, nil
}
