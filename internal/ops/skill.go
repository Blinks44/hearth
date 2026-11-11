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
	if _, err := os.Lstat(link); err == nil {
		return Plan{}, errors.New(locales.SkillLinkTaken(link))
	}
	var steps []Step
	if _, err := os.Stat(dir); err != nil {
		steps = append(steps, Step{Argv: mkdirCommand(dir)})
	}
	steps = append(steps, Step{Argv: linkCommand(sk.Dir, link)})
	p := Plan{
		Title:   locales.SkillLinkTitle(sk.Name, title),
		Why:     locales.SkillLinkWhy(),
		Steps:   steps,
		Changes: []string{"+ " + link + " → " + sk.Dir},
	}
	// The other readers of the folder get it too, but for those that
	// have it by another path already.
	var others []string
	for _, r := range readers {
		if r != id && !slices.ContainsFunc(sk.Paths, func(sp inventory.SkillPath) bool { return slices.Contains(sp.Agents, r) }) {
			others = append(others, names(r))
		}
	}
	if len(others) > 0 {
		p.Notes = []Note{{Title: locales.SkillShared(), Text: locales.SkillSharedText(strings.Join(others, ", "))}}
	}
	p.Verify = func(context.Context) (string, error) {
		for _, s := range readSkills(env, sk.Project) {
			if s.Dir == sk.Dir && slices.ContainsFunc(s.Paths, func(sp inventory.SkillPath) bool {
				return sp.Path == link && slices.Contains(sp.Agents, id)
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
	var merged [][]inventory.Skill
	if _, err := os.Stat(backups); err != nil {
		steps = append(steps, Step{Argv: mkdirCommand(backups)})
	}
	stamp := now.Format("2006-01-02-150405")
	for _, g := range groups {
		if len(g) < 2 || slices.ContainsFunc(g, func(s inventory.Skill) bool {
			return s.Broken || s.Sum == "" || s.Sum != g[0].Sum || s.Project != g[0].Project
		}) {
			continue
		}
		keep := g[0]
		for n, c := range g[1:] {
			backup := filepath.Join(backups, filepath.Base(c.Dir)+"-"+stamp)
			if n > 0 {
				backup += "-" + strconv.Itoa(n+1)
			}
			steps = append(steps, Step{Argv: moveCommand(c.Dir, backup)}, Step{Argv: linkCommand(keep.Dir, c.Dir)})
			changes = append(changes, c.Dir+" → "+keep.Dir)
		}
		merged = append(merged, g)
	}
	if len(merged) == 0 {
		return Plan{}, errors.New(locales.SkillNothingToMerge())
	}
	p := Plan{
		Title:   locales.SkillMergeTitle(len(merged)),
		Why:     locales.SkillMergeWhy(backups),
		Steps:   steps,
		Changes: changes,
	}
	p.Verify = func(context.Context) (string, error) {
		for _, g := range merged {
			for _, s := range readSkills(env, g[0].Project) {
				if s.Name == g[0].Name && s.Dir != g[0].Dir {
					return "", errors.New(locales.SkillCheckNotMerged(s.Name))
				}
			}
		}
		return locales.SkillCheckMerged(len(merged)), nil
	}
	return p, nil
}
