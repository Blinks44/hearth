package ops

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// linkMode covers Windows' junctions, which Go reports as irregular.
const linkMode = fs.ModeSymlink | fs.ModeIrregular

// readSkills reads again the skills of project, the user's when it is
// "", as a scan would.
func readSkills(env sysenv.Env, project string) []inventory.Skill {
	if project != "" {
		return ai.ProjectSkills(project)
	}
	return ai.Skills(env)
}

// canonicalSkill moves an existing skill into common storage. Existing links
// through its old folder keep working because that folder becomes a link.
func canonicalSkill(sk inventory.Skill, env sysenv.Env) (string, []Step, error) {
	if sk.Synced {
		return "", nil, errors.New(locales.SkillNothingToMerge())
	}
	dest := filepath.Join(ai.SharedSkillsDir(env, sk.Project), filepath.Base(sk.Dir))
	if dest == sk.Dir {
		return dest, nil, nil
	}
	var steps []Step
	if info, err := os.Lstat(dest); err == nil {
		if !sameSkillPath(dest, sk.Dir) {
			return "", nil, errors.New(locales.SkillLinkTaken(dest))
		}
		if info.Mode()&linkMode == 0 {
			return dest, nil, nil
		}
		steps = append(steps, Step{Argv: unlinkCommand(dest)})
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, err
	}
	if _, err := os.Stat(filepath.Dir(dest)); errors.Is(err, os.ErrNotExist) {
		steps = append(steps, Step{Argv: mkdirCommand(filepath.Dir(dest))})
	}
	steps = append(steps, Step{Argv: moveCommand(sk.Dir, dest)}, Step{Argv: linkCommand(dest, sk.Dir)})
	return dest, steps, nil
}

func sameSkillPath(a, b string) bool {
	x, e1 := os.Stat(a)
	y, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(x, y)
}

func skillStorageNotes(name, dest string, links []string) []Note {
	notes := []Note{{Title: locales.SkillStorageFolder() + " · " + name, Text: dest}}
	if len(links) > 0 {
		notes = append(notes, Note{Title: locales.SkillStorageLinks(), Text: strings.Join(links, "\n")})
	}
	return notes
}

// SkillLinkPlan returns the plan that makes sk available to the agent id,
// named title: a link to its folder in the skills directory the agent
// reads, the user's or sk's project's. Other agents reading that
// directory get it too, which the plan says. It reads the disk: call it
// in the background. names gives the agents' names.
func SkillLinkPlan(sk inventory.Skill, id string, env sysenv.Env, names func(string) string) (Plan, error) {
	title := names(id)
	dir, readers := ai.SkillsDir(id, env, sk.Project)
	if dir == "" || sk.Broken {
		return Plan{}, errors.New(locales.SkillNoDir(title))
	}
	link := filepath.Join(dir, filepath.Base(sk.Dir))
	canonical, steps, err := canonicalSkill(sk, env)
	if err != nil {
		return Plan{}, err
	}
	if link != canonical {
		if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
			return Plan{}, errors.New(locales.SkillLinkTaken(link))
		}
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			steps = append(steps, Step{Argv: mkdirCommand(dir)})
		}
		steps = append(steps, Step{Argv: linkCommand(canonical, link)})
	} else if len(steps) == 0 {
		return Plan{}, errors.New(locales.SkillLinkTaken(link))
	}
	var links []string
	if len(steps) > 0 && !sameSkillPath(sk.Dir, canonical) {
		links = append(links, sk.Dir+" → "+canonical)
	}
	if link != canonical {
		links = append(links, link+" → "+canonical)
	}
	p := Plan{
		Title:   locales.SkillLinkTitle(sk.Name, title),
		Why:     locales.SkillLinkWhy(),
		Steps:   steps,
		Changes: append([]string{canonical}, links...),
		Notes:   skillStorageNotes(sk.Name, canonical, links),
	}
	// The other readers of the folder get it too, but for those that
	// have it by another path already.
	_, commonReaders := ai.SkillsDir("codex", env, sk.Project)
	readers = slices.Concat(readers, commonReaders)
	var others []string
	for _, r := range readers {
		if r != id && !slices.Contains(others, names(r)) && !slices.ContainsFunc(sk.Paths, func(sp inventory.SkillPath) bool { return slices.Contains(sp.Agents, r) }) {
			others = append(others, names(r))
		}
	}
	if len(others) > 0 {
		p.Notes = append(p.Notes, Note{Title: locales.SkillShared(), Text: locales.SkillSharedText(strings.Join(others, ", "))})
	}
	p.Verify = func(context.Context) (string, error) {
		for _, s := range readSkills(env, sk.Project) {
			if sameSkillPath(s.Dir, canonical) && slices.ContainsFunc(s.Paths, func(sp inventory.SkillPath) bool {
				return sameSkillPath(sp.Path, link) && slices.Contains(sp.Agents, id)
			}) {
				return locales.SkillCheckLinked(sk.Name, title), nil
			}
		}
		return "", errors.New(locales.SkillCheckNotLinked(sk.Name, title))
	}
	return p, nil
}

// SkillUnlinkPlan returns the plan that takes away the link at path to
// sk, which the agents reading its directory find sk by. Only a link
// goes: the folder of a skill is the user's to delete. It reads the
// disk: call it in the background. names gives the agents' names.
func SkillUnlinkPlan(sk inventory.Skill, path string, env sysenv.Env, names func(string) string) (Plan, error) {
	i := slices.IndexFunc(sk.Paths, func(p inventory.SkillPath) bool { return p.Path == path })
	info, err := os.Lstat(path)
	if i < 0 || err != nil || info.Mode()&linkMode == 0 {
		return Plan{}, errors.New(locales.SkillNotLink(path))
	}
	// The agents that lose it: those that find it by no other path.
	var losing, titles []string
	for _, id := range sk.Paths[i].Agents {
		if !slices.ContainsFunc(sk.Paths, func(sp inventory.SkillPath) bool { return sp.Path != path && slices.Contains(sp.Agents, id) }) {
			losing = append(losing, id)
			titles = append(titles, names(id))
		}
	}
	if len(losing) == 0 {
		return Plan{}, errors.New(locales.SkillNotLink(path))
	}
	agents := strings.Join(titles, ", ")
	p := Plan{
		Title:   locales.SkillUnlinkTitle(sk.Name, agents),
		Why:     locales.SkillUnlinkWhy(),
		Steps:   []Step{{Argv: unlinkCommand(path)}},
		Changes: []string{"− " + path + " → " + sk.Dir},
	}
	p.Verify = func(context.Context) (string, error) {
		for _, s := range readSkills(env, sk.Project) {
			if s.Dir == sk.Dir && slices.ContainsFunc(s.Paths, func(sp inventory.SkillPath) bool {
				return sp.Path == path || slices.ContainsFunc(losing, func(id string) bool { return slices.Contains(sp.Agents, id) })
			}) {
				return "", errors.New(locales.SkillCheckStillLinked(sk.Name, agents))
			}
		}
		return locales.SkillCheckUnlinked(sk.Name, agents), nil
	}
	return p, nil
}

// SkillMergePlan returns the plan that keeps one folder of each group of
// copies, skills by one name in one scope with the same files: each other
// copy moves into backups, and a link to the kept folder takes its place,
// so every agent finds the one folder by the path it did. Groups whose
// copies differ are left out. It reads the disk: call it in the
// background.
func SkillMergePlan(groups [][]inventory.Skill, backups string, env sysenv.Env, now time.Time) (Plan, error) {
	var steps []Step
	var changes []string
	var merged []inventory.Skill
	var notes []Note
	if _, err := os.Stat(backups); err != nil {
		steps = append(steps, Step{Argv: mkdirCommand(backups)})
	}
	stamp := now.Format("2006-01-02-150405")
	for _, g := range groups {
		if len(g) < 2 || slices.ContainsFunc(g, func(s inventory.Skill) bool {
			return s.Broken || s.Synced || s.Sum == "" || s.Sum != g[0].Sum || s.Project != g[0].Project
		}) {
			continue
		}
		g = slices.Clone(g)
		shared := filepath.Join(ai.SharedSkillsDir(env, g[0].Project), filepath.Base(g[0].Dir))
		if i := slices.IndexFunc(g, func(s inventory.Skill) bool {
			return sameSkillPath(shared, s.Dir)
		}); i > 0 {
			g[0], g[i] = g[i], g[0]
		}
		keep := g[0]
		dest, promote, err := canonicalSkill(keep, env)
		if err != nil {
			return Plan{}, err
		}
		steps = append(steps, promote...)
		var links []string
		if len(promote) > 0 {
			links = append(links, keep.Dir+" → "+dest)
		}
		keep.Dir = dest
		for n, c := range g[1:] {
			backup := filepath.Join(backups, filepath.Base(c.Dir)+"-"+stamp)
			if n > 0 {
				backup += "-" + strconv.Itoa(n+1)
			}
			steps = append(steps, Step{Argv: moveCommand(c.Dir, backup)}, Step{Argv: linkCommand(keep.Dir, c.Dir)})
			changes = append(changes, c.Dir+" → "+keep.Dir)
			links = append(links, c.Dir+" → "+keep.Dir)
		}
		merged = append(merged, keep)
		notes = append(notes, skillStorageNotes(keep.Name, dest, links)...)
	}
	if len(merged) == 0 {
		return Plan{}, errors.New(locales.SkillNothingToMerge())
	}
	p := Plan{
		Title:   locales.SkillMergeTitle(len(merged)),
		Why:     locales.SkillMergeWhy(backups),
		Steps:   steps,
		Changes: changes,
		Notes:   notes,
	}
	p.Verify = func(context.Context) (string, error) {
		for _, g := range merged {
			found := false
			for _, s := range readSkills(env, g.Project) {
				if s.Name == g.Name && sameSkillPath(s.Dir, g.Dir) {
					found = true
				}
				if s.Name == g.Name && !sameSkillPath(s.Dir, g.Dir) {
					return "", errors.New(locales.SkillCheckNotMerged(s.Name))
				}
			}
			if !found {
				return "", errors.New(locales.SkillCheckNotMerged(g.Name))
			}
		}
		return locales.SkillCheckMerged(len(merged)), nil
	}
	return p, nil
}

// SkillRemoveBrokenPlan removes a broken symlink/junction from a skills folder.
func SkillRemoveBrokenPlan(sk inventory.Skill, env sysenv.Env) (Plan, error) {
	if !sk.Broken || len(sk.Paths) == 0 {
		return Plan{}, errors.New(locales.SkillNotLink(sk.Dir))
	}
	link := sk.Paths[0].Path
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&linkMode == 0 {
		return Plan{}, errors.New(locales.SkillNotLink(link))
	}
	if _, err := os.Stat(link); err == nil {
		return Plan{}, errors.New(locales.SkillNotLink(link))
	}
	p := Plan{
		Title:   locales.SkillRemoveBrokenTitle(sk.Name),
		Why:     locales.SkillRemoveBrokenWhy(),
		Steps:   []Step{{Argv: unlinkCommand(link)}},
		Changes: []string{"− " + link},
	}
	p.Verify = func(context.Context) (string, error) {
		if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
			return "", errors.New(locales.SkillCheckBrokenStillExists(sk.Name))
		}
		return locales.SkillCheckBrokenRemoved(sk.Name), nil
	}
	return p, nil
}

// SkillRemovePlan removes a skill and its agent symlinks. When the skill's
// folder lives outside Hearth's known skills directories (an external user
// folder linked in), only the symlinks are removed and the external folder
// stays untouched. Synced skills are refused.
func SkillRemovePlan(sk inventory.Skill, env sysenv.Env, names func(string) string) (Plan, error) {
	if sk.Synced {
		return Plan{}, errors.New(locales.SkillRemoveSynced())
	}
	if sk.Broken {
		return SkillRemoveBrokenPlan(sk, env)
	}
	var steps []Step
	var changes []string
	var linkPaths []string
	for _, sp := range sk.Paths {
		info, err := os.Lstat(sp.Path)
		if err != nil || info.Mode()&linkMode == 0 {
			continue
		}
		if slices.Contains(linkPaths, sp.Path) {
			continue
		}
		linkPaths = append(linkPaths, sp.Path)
		steps = append(steps, Step{Argv: unlinkCommand(sp.Path)})
		changes = append(changes, "− "+sp.Path+" → "+sk.Dir)
	}
	managed := isManagedSkillDir(sk.Dir, ai.SkillRoots(env, sk.Project))
	why := locales.SkillRemoveWhy()
	var notes []Note
	if managed {
		if _, err := os.Stat(sk.Dir); err != nil {
			return Plan{}, err
		}
		steps = append(steps, Step{Remove: sk.Dir})
		changes = append(changes, "− "+sk.Dir)
	} else {
		if len(steps) == 0 {
			return Plan{}, errors.New(locales.SkillNotLink(sk.Dir))
		}
		why = locales.SkillRemoveExternalWhy()
		notes = append(notes, Note{Title: locales.SkillRemoveExternalNote(), Text: locales.SkillRemoveExternalText(sk.Dir)})
	}
	var titles []string
	for _, sp := range sk.Paths {
		for _, id := range sp.Agents {
			title := names(id)
			if !slices.Contains(titles, title) {
				titles = append(titles, title)
			}
		}
	}
	if len(titles) > 0 {
		notes = append(notes, Note{Title: locales.AvailableToAgents(), Text: strings.Join(titles, ", ")})
	}
	p := Plan{
		Title:   locales.SkillRemoveTitle(sk.Name),
		Why:     why,
		Steps:   steps,
		Changes: changes,
		Notes:   notes,
	}
	p.Verify = func(context.Context) (string, error) {
		for _, lp := range linkPaths {
			if _, err := os.Lstat(lp); !errors.Is(err, os.ErrNotExist) {
				return "", errors.New(locales.SkillCheckStillExists(sk.Name))
			}
		}
		if managed {
			if _, err := os.Lstat(sk.Dir); !errors.Is(err, os.ErrNotExist) {
				return "", errors.New(locales.SkillCheckStillExists(sk.Name))
			}
		}
		for _, s := range readSkills(env, sk.Project) {
			if sameSkillPath(s.Dir, sk.Dir) {
				return "", errors.New(locales.SkillCheckStillExists(sk.Name))
			}
		}
		return locales.SkillCheckRemoved(sk.Name), nil
	}
	return p, nil
}

func isManagedSkillDir(dir string, roots []string) bool {
	if dir == "" {
		return false
	}
	if real, err := sysenv.RealPath(dir); err == nil {
		dir = real
	}
	dir = filepath.Clean(dir)
	for _, root := range roots {
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			continue
		}
		if rel != "." && rel != "" && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
