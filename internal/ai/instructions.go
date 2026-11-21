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

func instructionFile(path string) (inventory.InstructionFile, string) {
	out := inventory.InstructionFile{Path: path, Empty: true}
	if path == "" {
		out.Err = "instruction directory is unavailable"
		return out, ""
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		out.Target = instructionDestination(path)
		return out, ""
	}
	out.Exists = true
	if err != nil {
		out.Err = err.Error()
		return out, ""
	}
	out.Link = info.Mode()&os.ModeSymlink != 0
	out.Target, err = filepath.EvalSymlinks(path)
	if err != nil {
		out.Err = err.Error()
		return out, ""
	}
	text, err := InstructionText(path)
	if err != nil {
		out.Err = err.Error()
		return out, ""
	}
	sum := sha256.Sum256([]byte(text))
	out.Sum = hex.EncodeToString(sum[:])
	out.Empty = strings.TrimSpace(text) == ""
	return out, text
}

// Resolve the existing ancestors too when an agent's directory is not there yet.
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
	in, _ := AgentInstructions("codex", env)
	return in
}

// AgentInstructions inspects an agent's global instruction files using the
// terminal environment (including agent-specific home variables). It reports
// false for agents that do not use file-based global instructions.
// A shared ~/.agents/AGENTS.md file alone is not an active source for any agent.
func AgentInstructions(id string, env sysenv.Env) (inventory.GlobalInstructions, bool) {
	var spec *agent
	for i := range agents {
		if agents[i].id == id {
			spec = &agents[i]
			break
		}
	}
	if spec == nil {
		return inventory.GlobalInstructions{}, false
	}
	home := homeDir(env)
	base := dir(spec.dir, env, home)
	var candidatePaths []string
	var overridePath string
	for _, f := range spec.files {
		if f.kind != instructions {
			continue
		}
		p := resolve(f.path, base, home)
		if f.path == "AGENTS.override.md" {
			overridePath = p
		} else {
			candidatePaths = append(candidatePaths, p)
		}
	}
	if len(candidatePaths) == 0 {
		return inventory.GlobalInstructions{}, false
	}
	sharedPath := ""
	if home != "" {
		sharedPath = filepath.Join(home, ".agents", "AGENTS.md")
	}
	shared, _ := instructionFile(sharedPath)
	var override inventory.InstructionFile
	if overridePath != "" {
		override, _ = instructionFile(overridePath)
	}
	native, nativeText := pickNativeInstruction(id, home, shared, candidatePaths)
	out := inventory.GlobalInstructions{
		Shared:   shared,
		Native:   native,
		Override: override,
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
		case importsSharedInstructions(id, nativeText, home):
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
	return out, true
}

// pickNativeInstruction evaluates all standalone global instruction paths an
// agent reads. Any unreadable file or broken link wins so it is surfaced and
// protected; otherwise an existing linked, non-empty, or empty file wins before
// falling back to the canonical default path candidates[0].
func pickNativeInstruction(id, home string, shared inventory.InstructionFile, candidates []string) (inventory.InstructionFile, string) {
	type evaluated struct {
		file inventory.InstructionFile
		text string
	}
	items := make([]evaluated, len(candidates))
	for i, p := range candidates {
		f, text := instructionFile(p)
		items[i] = evaluated{f, text}
	}
	for _, it := range items {
		if it.file.Err != "" {
			return it.file, it.text
		}
	}
	if shared.Exists && !shared.Empty && shared.Err == "" {
		for _, it := range items {
			if !it.file.Exists || it.file.Empty {
				continue
			}
			if (it.file.Link && it.file.Target == shared.Target) || importsSharedInstructions(id, it.text, home) {
				return it.file, it.text
			}
		}
	}
	for _, it := range items {
		if it.file.Exists && !it.file.Empty {
			return it.file, it.text
		}
	}
	for _, it := range items {
		if it.file.Exists {
			return it.file, it.text
		}
	}
	return items[0].file, items[0].text
}

// importsSharedInstructions recognizes standalone file-import directives
// pointing to ~/.agents/AGENTS.md outside fenced code blocks and HTML comments.
func importsSharedInstructions(agentID, text, home string) bool {
	switch agentID {
	case "claude", "gemini", "antigravity":
	default:
		return false
	}
	abs := ""
	if home != "" {
		abs = filepath.ToSlash(filepath.Join(home, ".agents", "AGENTS.md"))
	}
	matchesShared := func(raw string) bool {
		p := filepath.ToSlash(strings.TrimSpace(raw))
		return p == "~/.agents/AGENTS.md" || (abs != "" && p == abs)
	}
	fence := ""
	comment := false
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}
		line = strings.TrimSpace(line)
		if comment {
			if strings.Contains(line, "-->") {
				comment = false
			}
			continue
		}
		if strings.HasPrefix(line, "<!--") {
			comment = !strings.Contains(line, "-->")
			continue
		}
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			mark := line[:3]
			switch fence {
			case "":
				fence = mark
			case mark:
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		switch agentID {
		case "claude", "gemini":
			if rest, ok := strings.CutPrefix(line, "@"); ok && matchesShared(rest) {
				return true
			}
		case "antigravity":
			if rest, ok := strings.CutPrefix(line, "@["); ok && strings.HasSuffix(rest, ")") {
				if _, target, ok := strings.Cut(strings.TrimSuffix(rest, ")"), "]("); ok && matchesShared(target) {
					return true
				}
			}
		}
	}
	return false
}
