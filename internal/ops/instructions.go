package ops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

// InstructionPlan shares root project instructions through a Claude import.
// Originals outside Hearth's block are retained as HTML comments and in backups.
func InstructionPlan(dir string) (Plan, error) {
	in := projects.ReadInstructions(dir)
	state := in.State()
	if state == "unreadable" || state == "missing" || state == "linked" || in.Agents.Link || in.Claude.Link {
		return Plan{}, errors.New(locales.InstructionsCannotChange())
	}
	a, c := in.Agents.Text, in.Claude.Text
	for _, text := range []string{a, c} {
		if len(text) > 32*1024 || strings.Count(text, "\n") > 500 {
			return Plan{}, errors.New(locales.InstructionsCannotChange())
		}
	}
	// Existing imports can change meaning when moved to another instruction file.
	// Offer a manual review rather than guessing how to rewrite them.
	if hasClaudeImport(c) || strings.Contains(c, "<!--") || strings.Contains(c, "-->") {
		return Plan{}, errors.New(locales.InstructionsManual())
	}
	plan := Plan{Title: locales.InstructionsPlanTitle(filepath.Base(dir)), Why: locales.InstructionsPlanWhy(), Action: locales.InstructionsConnect()}
	add := func(name, before, after string, exists bool) {
		if before != after {
			plan.Steps = append(plan.Steps, Step{Edit: &FileEdit{Path: filepath.Join(dir, name), Before: before, After: after, New: !exists}})
		}
	}
	if state == "claude" || state == "different" {
		body := strings.TrimSpace(c)
		if a != "" && !strings.HasSuffix(a, "\n") {
			a += "\n"
		}
		a += "\n<!-- >>> hearth: instructions >>> -->\n" + body + "\n<!-- <<< hearth: instructions <<< -->\n"
		add("AGENTS.md", in.Agents.Text, a, in.Agents.Exists)
	}
	after := ""
	if c != "" {
		// Keep all original lines, including Markdown fences, inside one block
		// comment; the comment starts outside a code block.
		after = "<!-- hearth: original instructions moved to AGENTS.md\n" + c
		if !strings.HasSuffix(after, "\n") {
			after += "\n"
		}
		after += "-->\n\n"
	}
	after += "<!-- >>> hearth: instructions >>> -->\n@AGENTS.md\n<!-- <<< hearth: instructions <<< -->\n"
	add("CLAUDE.md", c, after, in.Claude.Exists)
	plan.Notes = []Note{{Title: locales.InstructionsReview(), Text: locales.InstructionsReviewText()}}
	plan.Changes = []string{locales.InstructionsPlanChange()}
	plan.Verify = func(context.Context) (string, error) {
		if projects.ReadInstructions(dir).State() != "linked" {
			return "", errors.New(locales.InstructionsCannotChange())
		}
		return locales.InstructionsPlanChange(), nil
	}
	return plan, nil
}

// CreateProjectInstructionsPlan creates a new root AGENTS.md file in dir.
func CreateProjectInstructionsPlan(dir, text string) (Plan, error) {
	if dir == "" {
		return Plan{}, errors.New(locales.InstructionsCannotChange())
	}
	in := projects.ReadInstructions(dir)
	if in.Agents.Exists || in.Agents.Err != "" {
		return Plan{}, errors.New(locales.InstructionsCreateExists())
	}
	if !utf8.ValidString(text) || strings.TrimSpace(text) == "" || len(text) > 32*1024 || strings.Count(text, "\n") > 500 {
		return Plan{}, errors.New(locales.InstructionsCannotChange())
	}
	after := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasSuffix(after, "\n") {
		after += "\n"
	}
	path := filepath.Join(dir, "AGENTS.md")
	plan := Plan{
		Title:   locales.InstructionsCreateTitle(filepath.Base(dir)),
		Why:     locales.InstructionsCreateWhy(),
		Action:  locales.InstructionsCreateAction(),
		Steps:   []Step{{Edit: &FileEdit{Path: path, After: after, New: true}}},
		Changes: []string{locales.InstructionsCreateDone()},
	}
	plan.Verify = func(context.Context) (string, error) {
		got := projects.ReadInstructions(dir)
		if !got.Agents.Exists || got.Agents.Err != "" || strings.TrimSpace(got.Agents.Text) == "" {
			return "", errors.New(locales.InstructionsCannotChange())
		}
		return locales.InstructionsCreateDone(), nil
	}
	return plan, nil
}

// hasClaudeImport reports whether Markdown text contains a Claude Code @file
// import outside fenced/indented code, HTML comments, and inline code spans.
// Emails (user@domain), decorators (@deprecated), and npm scopes (@scope/pkg)
// are not treated as file imports.
func hasClaudeImport(text string) bool {
	fence := ""
	comment := false
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if comment {
			if strings.Contains(trimmed, "-->") {
				comment = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			comment = !strings.Contains(trimmed, "-->")
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			mark := trimmed[:3]
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
		if lineHasClaudeImport(stripInlineCode(line)) {
			return true
		}
	}
	return false
}

func stripInlineCode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '`' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		ticks := s[i:j]
		end := strings.Index(s[j:], ticks)
		if end < 0 {
			b.WriteString(ticks)
			i = j
			continue
		}
		b.WriteByte(' ')
		i = j + end + len(ticks)
	}
	return b.String()
}

func lineHasClaudeImport(line string) bool {
	for i := 0; i < len(line); i++ {
		if line[i] != '@' {
			continue
		}
		if i > 0 {
			prev, _ := utf8.DecodeLastRuneInString(line[:i])
			if unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_' || prev == '.' || prev == '-' || prev == '+' || prev == '/' {
				continue
			}
		}
		rest := line[i+1:]
		end := strings.IndexAny(rest, " \t\r\n)]\"',;>")
		tok := rest
		if end >= 0 {
			tok = rest[:end]
		}
		tok = strings.TrimRight(tok, ".:!?")
		if isImportTarget(tok) {
			return true
		}
	}
	return false
}

func isImportTarget(tok string) bool {
	if tok == "" {
		return false
	}
	if strings.HasPrefix(tok, "./") || strings.HasPrefix(tok, "../") || strings.HasPrefix(tok, "~/") || strings.HasPrefix(tok, "/") {
		return len(tok) > 1
	}
	ext := strings.ToLower(filepath.Ext(tok))
	switch ext {
	case ".md", ".markdown", ".mdx", ".txt", ".mdc", ".yaml", ".yml", ".json", ".toml", ".xml", ".rst":
		return len(tok) > len(ext)
	}
	return false
}
