package projects

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Instructions describes the two instruction files at a project's root.
// It compares files, not the effective context of an agent session.
type Instructions struct {
	Agents, Claude InstructionFile
}

// InstructionFile is a bounded, UTF-8 instruction file read during discovery.
type InstructionFile struct {
	Text         string
	Exists, Link bool
	Err          string
}

// ReadInstructions reads only root instruction files; failures stay on their file.
func ReadInstructions(dir string) Instructions {
	return Instructions{readInstruction(filepath.Join(dir, "AGENTS.md")), readInstruction(filepath.Join(dir, "CLAUDE.md"))}
}

func readInstruction(path string) InstructionFile {
	var out InstructionFile
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return out
	}
	out.Exists = true
	if err != nil {
		out.Err = err.Error()
		return out
	}
	out.Link = info.Mode()&os.ModeSymlink != 0
	target, err := os.Stat(path)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	if !target.Mode().IsRegular() {
		out.Err = fmt.Sprintf("not a regular file: %s", path)
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	defer func() { _ = f.Close() }()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		out.Err = fmt.Sprintf("not a regular file: %s", path)
		return out
	}
	data, err := io.ReadAll(io.LimitReader(f, 256*1024+1))
	if err != nil {
		out.Err = err.Error()
		return out
	}
	if len(data) > 256*1024 {
		out.Err = fmt.Sprintf("instruction file exceeds 256 KiB: %s", path)
		return out
	}
	if !utf8.Valid(data) {
		out.Err = fmt.Sprintf("instruction file is not UTF-8: %s", path)
		return out
	}
	out.Text = string(data)
	return out
}

// State is missing, agents, claude, same, linked, different, or unreadable.
func (in Instructions) State() string {
	if in.Agents.Err != "" || in.Claude.Err != "" {
		return "unreadable"
	}
	a, c := instructionText(in.Agents.Text), instructionText(in.Claude.Text)
	switch {
	case a == "" && c == "":
		return "missing"
	case a == "":
		return "claude"
	case c == "":
		return "agents"
	case ImportsAgents(in.Claude.Text):
		return "linked"
	case a == c:
		return "same"
	default:
		return "different"
	}
}

func instructionText(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
}

// ImportsAgents recognizes the standalone import Hearth writes, outside
// fenced code and HTML comments. Other import forms are left for manual review.
func ImportsAgents(text string) bool {
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
		if fence == "" && (line == "@AGENTS.md" || line == "@./AGENTS.md") {
			return true
		}
	}
	return false
}
