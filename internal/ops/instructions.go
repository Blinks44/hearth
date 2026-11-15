package ops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

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
	if strings.Contains(c, "@") || strings.Contains(c, "<!--") || strings.Contains(c, "-->") {
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
