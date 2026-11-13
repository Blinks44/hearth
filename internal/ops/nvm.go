package ops

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// nvmLine matches a line of a startup file that loads nvm or sets it up:
// NVM_DIR, nvm.sh, its completion and nvm use.
var nvmLine = regexp.MustCompile(`\bNVM_DIR\b|nvm\.sh|nvm/bash_completion|(^|[\s;&])nvm\s+use\b`)

// NvmMovePlan returns the plan that stops nvm from loading at the start of
// every terminal, which line of a startup file does: it sets up mise, has
// mise install the version of Node nvm makes the default, of node, the
// scan's, and comments out the lines of line's file that load nvm. Node's
// title is title; env is the terminal's, detect checks Node's default.
// Versions nvm keeps stay in its folder.
func NvmMovePlan(line inventory.StartupLine, node inventory.ToolState, title string, env sysenv.Env, detect Detect) (Plan, error) {
	data, err := os.ReadFile(line.File)
	if err != nil {
		return Plan{}, fmt.Errorf("read %s: %w", line.File, err)
	}
	if lines := splitLines(string(data)); line.Line < 1 || line.Line > len(lines) || sysenv.StartupCulprit(lines[line.Line-1]) != sysenv.CulpritNvm {
		return Plan{}, errors.New(locales.StartupLineChanged())
	}
	s, err := SetUpMise(env)
	if err != nil {
		return Plan{}, err
	}
	var p Plan
	version := ""
	for _, in := range node.Installations {
		if in.Source.Manager == "nvm" && in.Default {
			version = in.Version
		}
	}
	if version != "" {
		p, _ = MovePlan(VersionTarget{Tool: "node", Title: title, Version: version}, "nvm", detect)
	}
	// After WithMise, whose "no version manager yet" is not so with nvm.
	p = WithMise(p, s)
	p.Title, p.Why, p.Action = locales.NvmMoveTitle(), locales.NvmMoveWhy(), locales.NvmMoveAction()
	// The text to comment out in is the file as mise's hook leaves it,
	// when the hook goes in the same file.
	before := string(data)
	for _, st := range p.Steps {
		if st.Edit != nil && st.Edit.Path == line.File {
			before = st.Edit.After
		}
	}
	all := splitLines(before)
	start, end := blockLines(all)
	var commented []int
	for i, l := range all {
		if (start < 0 || i < start || i > end) && !strings.HasPrefix(strings.TrimSpace(l), "#") && nvmLine.MatchString(l) {
			commented = append(commented, i+1)
		}
	}
	p.Steps = append(p.Steps, Step{Edit: &FileEdit{Path: line.File, Before: before, After: CommentOut(before, commented, "nvm moved to mise")}})
	p.Changes = append(p.Changes, locales.NvmMoveComment(len(commented)))
	p.Notes = append(p.Notes, Note{Title: locales.NvmMoveKeptTitle(), Text: locales.NvmMoveKeptText()})
	return p, nil
}
