package ops

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

// PinTarget is a version to pin in a project: the tool, as "node" or
// "pnpm", its title, as the user sees it, and the version manager the
// user switches Node with, which picks the file a project without one
// gets.
type PinTarget struct {
	Tool, Title, Version, Manager string
}

// nodeFiles are the files that pin Node, in the order a project's own is
// looked for.
var nodeFiles = []string{".nvmrc", ".node-version", ".tool-versions", "mise.toml", ".mise.toml"}

// PinPlan returns the plan that writes t's version to the file project p
// already pins it in, or that its version manager reads; ok is false when
// the file pins that version already or Hearth cannot write it.
func PinPlan(p projects.Project, t PinTarget) (plan Plan, ok bool) {
	file, key := pinFile(p, t)
	if file == "" {
		return Plan{}, false
	}
	before, existed := p.Texts[file]
	after, err := pinText(file, before, t)
	if err != nil || existed && after == before {
		return Plan{}, false
	}
	shown := file
	if key != "" {
		shown += " · " + key
	}
	plan = Plan{
		Title:   locales.PinTitle(t.Title, t.Version, filepath.Base(p.Dir)),
		Why:     locales.PinWhy(shown),
		Action:  locales.PinAction(t.Version),
		Steps:   []Step{{Edit: &FileEdit{Path: filepath.Join(p.Dir, file), Before: before, After: after, New: !existed}}},
		Changes: []string{locales.PinChange(shown, t.Title, t.Version)},
	}
	if !existed {
		plan.Why = locales.PinWhyNew(file, t.Manager)
	}
	plan.Verify = func(context.Context) (string, error) {
		for _, r := range projects.Read(p.Dir).Requirements {
			if r.Tool == t.Tool && r.File == file && r.Want == t.Version {
				return locales.PinChecked(shown, t.Title, t.Version), nil
			}
		}
		return "", errors.New(locales.PinNotChecked(shown, t.Title, t.Version))
	}
	return plan, true
}

// pinFile is the file t's version goes to in p, with the key in it, ""
// when there is none to write: for Node, the file that pins it already,
// else one of mise or asdf the project has, else the one t.Manager
// reads; for pnpm and Yarn, packageManager in package.json.
func pinFile(p projects.Project, t PinTarget) (file, key string) {
	if t.Tool != "node" {
		if _, ok := p.Texts["package.json"]; !ok || t.Tool != "pnpm" && t.Tool != "yarn" {
			return "", ""
		}
		// A project another package manager is set for stays so.
		for _, r := range p.Requirements {
			if r.File == "package.json" && r.Key == "packageManager" && r.Tool != t.Tool {
				return "", ""
			}
		}
		return "package.json", "packageManager"
	}
	for _, f := range nodeFiles {
		if slices.ContainsFunc(p.Requirements, func(r projects.Requirement) bool { return r.Tool == "node" && r.File == f }) {
			return f, ""
		}
	}
	for _, f := range nodeFiles[2:] {
		if _, ok := p.Texts[f]; ok {
			return f, ""
		}
	}
	switch t.Manager {
	case "asdf":
		return ".tool-versions", ""
	case "mise":
		return "mise.toml", ""
	}
	return ".nvmrc", ""
}

// pinText returns text, file's contents, pinning t's version.
func pinText(file, text string, t PinTarget) (string, error) {
	switch file {
	case "package.json":
		return SetJSON(text, "packageManager", t.Tool+"@"+t.Version)
	case "mise.toml", ".mise.toml":
		return SetTOML(text, "tools", "node", t.Version)
	case ".tool-versions":
		lines := splitLines(text)
		for i, l := range lines {
			if f := strings.Fields(l); len(f) > 0 && (f[0] == "nodejs" || f[0] == "node") {
				lines[i] = f[0] + " " + t.Version
				return joinLines(lines), nil
			}
		}
		return joinLines(append(lines, "nodejs "+t.Version)), nil
	}
	// .nvmrc and .node-version: the version is the first line that is not
	// empty or a comment.
	lines := splitLines(text)
	for i, l := range lines {
		if v, _, _ := strings.Cut(l, "#"); strings.TrimSpace(v) != "" {
			lines[i] = t.Version
			return joinLines(lines), nil
		}
	}
	return joinLines(append(lines, t.Version)), nil
}
