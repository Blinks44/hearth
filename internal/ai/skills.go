package ai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// skillsRoot is a skills directory with the agents reading it.
type skillsRoot struct {
	path   string
	agents []string
}

// maxDepth is how deep in a skills directory skills are looked for: Claude
// Code keeps the synced ones as skills/synced/<account>/<name>.
const maxDepth = 3

// Skills finds the user's skills in the skills directories of every agent
// Hearth knows; env gives the home and the agents' settings directories.
func Skills(env sysenv.Env) []inventory.Skill {
	home := homeDir(env)
	var roots []skillsRoot
	for _, a := range agents {
		d := dir(a.dir, env, home)
		for _, s := range a.skills {
			roots = addRoot(roots, resolve(s, d, home), a.id)
		}
	}
	return synced(readSkills(roots, ""), dir(agents[0].dir, env, home), home)
}

// synced marks the skills Claude syncs from claude.ai, kept as
// claude/skills/synced/<organization>_<…>/<name>, and drops those of an
// organization the user is not signed in to now: Claude reads only the
// current one's, the others are what an earlier sign-in left.
func synced(skills []inventory.Skill, claude, home string) []inventory.Skill {
	if claude == "" {
		return skills
	}
	root := filepath.Join(claude, "skills", "synced") + string(filepath.Separator)
	if real, err := sysenv.RealPath(strings.TrimSuffix(root, string(filepath.Separator))); err == nil {
		root = real + string(filepath.Separator)
	}
	org := organization(filepath.Join(home, ".claude.json"))
	out := skills[:0]
	for _, s := range skills {
		if rest, ok := strings.CutPrefix(s.Dir, root); ok {
			bucket, _, _ := strings.Cut(rest, string(filepath.Separator))
			if org != "" && !strings.HasPrefix(bucket, org+"_") {
				continue
			}
			s.Synced = true
		}
		out = append(out, s)
	}
	return out
}

// organization is the claude.ai organization Claude Code is signed in
// to, from its state file; "" when unknown.
func organization(state string) string {
	data, err := os.ReadFile(state)
	if err != nil {
		return ""
	}
	var v struct {
		OAuthAccount struct {
			OrganizationUUID string `json:"organizationUuid"`
		} `json:"oauthAccount"`
	}
	if json.Unmarshal(data, &v) != nil {
		return ""
	}
	return v.OAuthAccount.OrganizationUUID
}

// ProjectSkills finds the skills in the skills directories of the project
// in dir.
func ProjectSkills(dir string) []inventory.Skill {
	var roots []skillsRoot
	for _, a := range agents {
		for _, s := range a.projectSkills {
			roots = addRoot(roots, filepath.Join(dir, filepath.FromSlash(s)), a.id)
		}
	}
	return readSkills(roots, dir)
}

// SharedSkillsDir is the canonical storage for user or project skills.
func SharedSkillsDir(env sysenv.Env, project string) string {
	if project != "" {
		return filepath.Join(project, ".agents", "skills")
	}
	return filepath.Join(homeDir(env), ".agents", "skills")
}

// SkillsDir prefers common storage when the agent reads it, otherwise its
// own skills directory, the user's
// or, when project is not "", that project's, with the agents that read
// it: a skill linked there is theirs too. "" when the agent reads none.
func SkillsDir(id string, env sysenv.Env, project string) (skills string, readers []string) {
	home := homeDir(env)
	paths := func(a agent) []string {
		var out []string
		if project != "" {
			for _, s := range a.projectSkills {
				out = append(out, filepath.Join(project, filepath.FromSlash(s)))
			}
			return out
		}
		d := dir(a.dir, env, home)
		for _, s := range a.skills {
			if p := resolve(s, d, home); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	for _, a := range agents {
		if a.id == id {
			if p := paths(a); len(p) > 0 {
				skills = p[0]
				if shared := SharedSkillsDir(env, project); slices.Contains(p, shared) {
					skills = shared
				}
			}
		}
	}
	if skills == "" {
		return "", nil
	}
	for _, a := range agents {
		if slices.Contains(paths(a), skills) {
			readers = append(readers, a.id)
		}
	}
	return skills, readers
}

// addRoot adds agent as a reader of the skills directory path, the
// directory once.
func addRoot(roots []skillsRoot, path, agent string) []skillsRoot {
	if path == "" {
		return roots
	}
	for i := range roots {
		if roots[i].path == path {
			roots[i].agents = append(roots[i].agents, agent)
			return roots
		}
	}
	return append(roots, skillsRoot{path, []string{agent}})
}

// linkMode covers Windows' junctions too, which Go reports as irregular:
// they are how a user without the symlink right shares a folder there.
const linkMode = fs.ModeSymlink | fs.ModeIrregular

// readSkills reads the skills in roots, a folder that several paths lead
// to once, in the order found.
func readSkills(roots []skillsRoot, project string) []inventory.Skill {
	var out []inventory.Skill
	byDir := map[string]int{}
	for _, r := range roots {
		info, err := os.Lstat(r.path)
		if err != nil {
			continue
		}
		var found []foundSkill
		findSkills(r.path, info.Mode()&linkMode != 0, maxDepth, &found)
		for _, f := range found {
			p := inventory.SkillPath{Path: f.path, Link: f.link, Agents: r.agents}
			if f.broken {
				out = append(out, brokenSkill(p, project))
				continue
			}
			real, err := sysenv.RealPath(f.path)
			if err != nil {
				real = f.path
			}
			if i, ok := byDir[real]; ok {
				out[i].Paths = append(out[i].Paths, p)
				continue
			}
			byDir[real] = len(out)
			out = append(out, readSkill(real, p, project))
		}
	}
	return out
}

// foundSkill is a folder with a SKILL.md, or a symlink leading nowhere.
type foundSkill struct {
	path         string
	link, broken bool
}

// findSkills finds the skills in dir and, depth levels down, in its
// folders that are not skills; link tells dir is reached by a symlink.
// Hidden entries are skipped.
func findSkills(dir string, link bool, depth int, out *[]foundSkill) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		symlink := e.Type()&linkMode != 0
		info, err := os.Stat(p)
		switch {
		case err != nil:
			if symlink {
				*out = append(*out, foundSkill{p, true, true})
			}
		case !info.IsDir():
		case exists(filepath.Join(p, "SKILL.md")):
			*out = append(*out, foundSkill{p, link || symlink, false})
		case depth > 1:
			findSkills(p, link || symlink, depth-1, out)
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// brokenSkill is a symlink at p that leads nowhere, named by the link.
func brokenSkill(p inventory.SkillPath, project string) inventory.Skill {
	target, _ := os.Readlink(p.Path)
	if target != "" && !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(p.Path), target)
	}
	return inventory.Skill{Name: filepath.Base(p.Path), Dir: target, Project: project,
		Paths: []inventory.SkillPath{p}, Broken: true}
}

// readSkill reads the skill in the folder dir, found at p.
func readSkill(dir string, p inventory.SkillPath, project string) inventory.Skill {
	s := inventory.Skill{Name: filepath.Base(dir), Dir: dir, Project: project, Paths: []inventory.SkillPath{p}}
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err == nil {
		var fm frontmatter
		if fm, err = readFrontmatter(data); err == nil {
			if fm.Name != "" {
				s.Name = fm.Name
			}
			s.Description = fm.Description
		}
	}
	if err != nil {
		s.Err = fmt.Errorf("read SKILL.md: %w", err).Error()
	}
	s.Sum = sum(dir)
	return s
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// readFrontmatter reads the YAML between the "---" lines SKILL.md starts
// with.
func readFrontmatter(data []byte) (frontmatter, error) {
	var fm frontmatter
	lines := strings.SplitAfter(strings.TrimPrefix(string(data), "\ufeff"), "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return fm, errors.New("no frontmatter")
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			err := yaml.Unmarshal([]byte(strings.Join(lines[1:i], "")), &fm)
			return fm, err
		}
	}
	return fm, errors.New("frontmatter not closed")
}

// sum hashes the names and contents of the files in dir, hidden ones and
// symlinks skipped, so that copies of a skill compare equal; "" when a
// file cannot be read.
func sum(dir string) string {
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }() // read only
		h.Write([]byte(filepath.ToSlash(rel) + "\x00"))
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		h.Write([]byte{0})
		return nil
	})
	if err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SkillFolder reads the folder of a skill: its entries, hidden ones aside,
// and the text of its SKILL.md.
func SkillFolder(dir string) (entries []inventory.Entry, skillMD string, err error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", err
	}
	for _, d := range des {
		if strings.HasPrefix(d.Name(), ".") {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, d.Name()))
		if err != nil {
			continue
		}
		e := inventory.Entry{Name: d.Name(), Dir: info.IsDir(), Size: info.Size(), Modified: info.ModTime()}
		if e.Dir {
			e.Size = 0
			_ = filepath.WalkDir(filepath.Join(dir, d.Name()), func(_ string, f fs.DirEntry, err error) error {
				if err == nil && f.Type().IsRegular() && !strings.HasPrefix(f.Name(), ".") {
					e.Files++
				}
				return nil
			})
		}
		entries = append(entries, e)
	}
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return entries, "", err
	}
	return entries, string(data), nil
}

// SkillDirectories returns the user directories for agents that support skills.
func SkillDirectories(env sysenv.Env) map[string]string {
	out := map[string]string{}
	for _, a := range agents {
		if dir, _ := SkillsDir(a.id, env, ""); dir != "" {
			out[a.id] = dir
		}
	}
	return out
}

// ProjectSkillDirectories returns the project skills directories for agents
// that support project-scoped skills.
func ProjectSkillDirectories(project string) map[string]string {
	if project == "" {
		return nil
	}
	out := map[string]string{}
	for _, a := range agents {
		if dir, _ := SkillsDir(a.id, nil, project); dir != "" {
			out[a.id] = dir
		}
	}
	return out
}

// SkillRoots returns all known skills directories for the user or project,
// including canonical shared storage.
func SkillRoots(env sysenv.Env, project string) []string {
	var out []string
	add := func(p string) {
		if p == "" {
			return
		}
		if real, err := sysenv.RealPath(p); err == nil {
			p = real
		}
		p = filepath.Clean(p)
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	add(SharedSkillsDir(env, project))
	if project != "" {
		for _, a := range agents {
			for _, s := range a.projectSkills {
				add(filepath.Join(project, filepath.FromSlash(s)))
			}
		}
		return out
	}
	home := homeDir(env)
	for _, a := range agents {
		d := dir(a.dir, env, home)
		for _, s := range a.skills {
			add(resolve(s, d, home))
		}
	}
	return out
}

// SkillFileDiff describes a file that differs between two copies of a skill.
type SkillFileDiff struct {
	Path     string
	Status   string // "modified", "added", "removed"
	Before   string
	After    string
	Binary   bool
	TooLarge bool
}

const (
	skillDiffMaxBytes = 32 * 1024
	skillDiffMaxLines = 500
)

type skillFile struct {
	data   []byte
	hash   [32]byte
	size   int64
	capped bool
}

func (f skillFile) equal(other skillFile) bool {
	if f.size != other.size {
		return false
	}
	if !f.capped && !other.capped {
		return bytes.Equal(f.data, other.data)
	}
	return f.hash == other.hash
}

// SkillDiff compares regular non-hidden files between dirA and dirB, matching
// the set of files hashed by sum.
func SkillDiff(dirA, dirB string) ([]SkillFileDiff, error) {
	filesA, err := readSkillTree(dirA)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dirA, err)
	}
	filesB, err := readSkillTree(dirB)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dirB, err)
	}
	var paths []string
	for p := range filesA {
		paths = append(paths, p)
	}
	for p := range filesB {
		if _, ok := filesA[p]; !ok {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)
	var out []SkillFileDiff
	for _, p := range paths {
		a, okA := filesA[p]
		b, okB := filesB[p]
		if okA && okB && a.equal(b) {
			continue
		}
		d := SkillFileDiff{Path: p}
		switch {
		case okA && !okB:
			d.Status = "removed"
		case !okA && okB:
			d.Status = "added"
		default:
			d.Status = "modified"
		}
		if isBinarySkillFile(a) || isBinarySkillFile(b) {
			d.Binary = true
		} else if a.capped || b.capped || len(a.data) > skillDiffMaxBytes || len(b.data) > skillDiffMaxBytes ||
			bytes.Count(a.data, []byte{'\n'}) > skillDiffMaxLines || bytes.Count(b.data, []byte{'\n'}) > skillDiffMaxLines {
			d.TooLarge = true
		} else {
			d.Before = string(a.data)
			d.After = string(b.data)
		}
		out = append(out, d)
	}
	return out, nil
}

func readSkillTree(dir string) (map[string]skillFile, error) {
	files := map[string]skillFile{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		sf, err := readSkillFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = sf
		return nil
	})
	return files, err
}

func readSkillFile(path string) (skillFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return skillFile{}, err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	prefix, err := io.ReadAll(io.TeeReader(io.LimitReader(f, skillDiffMaxBytes+1), h))
	if err != nil {
		return skillFile{}, err
	}
	size := int64(len(prefix))
	capped := len(prefix) > skillDiffMaxBytes
	if capped {
		prefix = prefix[:skillDiffMaxBytes]
		n, err := io.Copy(h, f)
		if err != nil {
			return skillFile{}, err
		}
		size += n
	}
	var sum [32]byte
	h.Sum(sum[:0])
	return skillFile{data: prefix, hash: sum, size: size, capped: capped}, nil
}

func isBinarySkillFile(f skillFile) bool {
	if len(f.data) == 0 {
		return false
	}
	if bytes.IndexByte(f.data, 0) >= 0 {
		return true
	}
	check := f.data
	if f.capped {
		// Trim up to 3 trailing bytes of a UTF-8 rune split at the prefix cap.
		for cut := 1; cut <= 3 && cut <= len(check); cut++ {
			if utf8.Valid(check[:len(check)-cut]) {
				return false
			}
		}
	}
	return !utf8.Valid(check)
}
