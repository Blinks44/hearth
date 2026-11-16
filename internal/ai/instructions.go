package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// InstructionText reads a bounded UTF-8 regular file, following dotfile links.
// Call in the background; contents must not be stored in the inventory.
func InstructionText(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat instructions: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("instructions are not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open instructions: %w", err)
	}
	defer func() { _ = f.Close() }()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("instructions are not a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, 256*1024+1))
	if err != nil {
		return "", fmt.Errorf("read instructions: %w", err)
	}
	if len(data) > 256*1024 || !utf8.Valid(data) {
		return "", errors.New("instructions exceed 256 KiB or are not UTF-8")
	}
	return string(data), nil
}

func instructionFile(path string) inventory.InstructionFile {
	out := inventory.InstructionFile{Path: path, Empty: true}
	if path == "" {
		out.Err = "instruction directory is unavailable"
		return out
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		out.Target = instructionDestination(path)
		return out
	}
	out.Exists = true
	if err != nil {
		out.Err = err.Error()
		return out
	}
	out.Link = info.Mode()&os.ModeSymlink != 0
	out.Target, err = filepath.EvalSymlinks(path)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	text, err := InstructionText(path)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	sum := sha256.Sum256([]byte(text))
	out.Sum = hex.EncodeToString(sum[:])
	out.Empty = strings.TrimSpace(text) == ""
	return out
}

// Resolve the existing ancestors too when Codex's directory is not there yet.
func instructionDestination(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(instructionDestination(parent), filepath.Base(path))
}

// CodexInstructions uses the terminal environment, including CODEX_HOME.
// A shared file alone is not a Codex global instruction source.
func CodexInstructions(env sysenv.Env) inventory.GlobalInstructions {
	home := homeDir(env)
	base := dir([]string{"$CODEX_HOME", "~/.codex"}, env, home)
	path := func(base, name string) string {
		if base == "" {
			return ""
		}
		return filepath.Join(base, name)
	}
	out := inventory.GlobalInstructions{
		Shared:   instructionFile(path(home, ".agents/AGENTS.md")),
		Native:   instructionFile(path(base, "AGENTS.md")),
		Override: instructionFile(path(base, "AGENTS.override.md")),
	}
	if out.Override.Exists && !out.Override.Empty && out.Override.Err == "" {
		out.Active = out.Override.Path
	} else if out.Override.Err == "" && out.Native.Exists && !out.Native.Empty && out.Native.Err == "" {
		out.Active = out.Native.Path
	}
	switch {
	case out.Shared.Err != "" || out.Native.Err != "" || out.Override.Err != "":
		out.State = "unreadable"
	case out.Active == out.Override.Path && out.Active != "":
		out.State = "override"
	case out.Native.Exists && out.Native.Empty:
		out.State = "empty"
	case out.Native.Exists && out.Shared.Exists && !out.Shared.Empty:
		switch {
		case out.Native.Link && out.Native.Target == out.Shared.Target:
			out.State = "linked"
		case out.Native.Sum == out.Shared.Sum:
			out.State = "same"
		default:
			out.State = "different"
		}
	case out.Native.Exists:
		out.State = "native"
	case out.Shared.Exists && !out.Shared.Empty:
		out.State = "shared"
	case out.Shared.Exists:
		out.State = "empty"
	default:
		out.State = "missing"
	}
	return out
}
