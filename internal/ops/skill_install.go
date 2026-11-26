package ops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// SkillInstall creates a new skill directory from the exact previewed files.
// No file from the bundle is executed, and an existing directory is never reused.
type SkillInstall struct {
	Path  string
	Skill skillcatalog.Skill
	Link  string
	Links []string
}

func (in SkillInstall) allLinks() []string {
	var out []string
	if in.Link != "" {
		out = append(out, in.Link)
	}
	for _, l := range in.Links {
		if l != "" && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// SkillInstallPlan reads the user destination without creating it.
func SkillInstallPlan(sk skillcatalog.Skill, id string, env sysenv.Env, names func(string) string) (Plan, error) {
	return SkillInstallPlanFor(sk, id, "", env, names)
}

// SkillInstallPlanFor reads the user or project destination without creating it.
func SkillInstallPlanFor(sk skillcatalog.Skill, id, project string, env sysenv.Env, names func(string) string) (Plan, error) {
	if err := sk.Validate(); err != nil {
		return Plan{}, fmt.Errorf("prepare skill: %w", err)
	}
	dir, _ := ai.SkillsDir(id, env, project)
	if dir == "" || !filepath.IsAbs(dir) {
		return Plan{}, errors.New(locales.SkillNoDir(names(id)))
	}
	dest := filepath.Join(ai.SharedSkillsDir(env, project), sk.Dir)
	link := filepath.Join(dir, sk.Dir)
	if link != dest {
		if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
			return Plan{}, errors.New(locales.SkillLinkTaken(link))
		}
	}
	if _, err := os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		return Plan{}, errors.New(locales.SkillLinkTaken(dest))
	}
	// The operation owns its copy even when a caller reuses the preview.
	sk.Files = slices.Clone(sk.Files)
	var files []string
	for i := range sk.Files {
		sk.Files[i].Data = bytes.Clone(sk.Files[i].Data)
		files = append(files, sk.Files[i].Path)
	}
	p := Plan{Title: locales.SkillInstallTitle(sk.Name, names(id)), Why: locales.SkillInstallWhy(), Action: locales.SkillCatalogInstall(), Steps: []Step{{Skill: &SkillInstall{Path: dest, Skill: sk}}}, Changes: []string{dest}}
	p.Notes = []Note{{Title: locales.SkillInstallSource(), Text: sk.Repository + " · " + sk.Commit, URL: sk.URL()}, {Title: locales.FilesSection(), Text: strings.Join(files, "\n")}, {Title: locales.SkillInstallReview(), Text: locales.SkillInstallReviewText()}}
	p.Notes = append(p.Notes, skillStorageNotes(sk.Name, dest, nil)...)
	if link != dest {
		p.Steps[0].Skill.Link = link
		p.Notes = append(p.Notes, Note{Title: locales.SkillStorageLinks(), Text: link + " → " + dest})
	}
	// Common storage is visible to every agent that reads it.
	_, readers := ai.SkillsDir("codex", env, project)
	if len(readers) > 1 {
		var titles []string
		for _, r := range readers {
			if r != id {
				titles = append(titles, names(r))
			}
		}
		p.Notes = append(p.Notes, Note{Title: locales.SkillShared(), Text: locales.SkillSharedText(strings.Join(titles, ", "))})
	}
	p.Verify = func(ctx context.Context) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		root, err := os.OpenRoot(dest)
		if err != nil {
			return "", fmt.Errorf("open installed skill: %w", err)
		}
		defer func() { _ = root.Close() }()
		if err = checkSkillFiles(root, sk.Files); err != nil {
			return "", err
		}
		return locales.SkillCheckLinked(sk.Name, names(id)), nil
	}
	return p, nil
}

// SkillInstallAllPlanFor installs sk into common storage and links every unique
// agent skills directory for ids in a single confirmed plan.
func SkillInstallAllPlanFor(sk skillcatalog.Skill, ids []string, project string, env sysenv.Env, names func(string) string) (Plan, error) {
	if err := sk.Validate(); err != nil {
		return Plan{}, fmt.Errorf("prepare skill: %w", err)
	}
	dest := filepath.Join(ai.SharedSkillsDir(env, project), sk.Dir)
	if !filepath.IsAbs(dest) {
		return Plan{}, errors.New(locales.SkillCatalogNoAgent())
	}
	if _, err := os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		return Plan{}, errors.New(locales.SkillLinkTaken(dest))
	}
	var links []string
	var linkNotes []string
	var titles []string
	matched := 0
	for _, id := range ids {
		if id == "" || id == "aider" {
			continue
		}
		dir, _ := ai.SkillsDir(id, env, project)
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		matched++
		if t := names(id); !slices.Contains(titles, t) {
			titles = append(titles, t)
		}
		link := filepath.Join(dir, sk.Dir)
		if link == dest || slices.Contains(links, link) {
			continue
		}
		if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
			return Plan{}, errors.New(locales.SkillLinkTaken(link))
		}
		links = append(links, link)
		linkNotes = append(linkNotes, link+" → "+dest)
	}
	if matched == 0 {
		return Plan{}, errors.New(locales.SkillCatalogNoAgent())
	}
	sk.Files = slices.Clone(sk.Files)
	var files []string
	for i := range sk.Files {
		sk.Files[i].Data = bytes.Clone(sk.Files[i].Data)
		files = append(files, sk.Files[i].Path)
	}
	p := Plan{
		Title:   locales.SkillInstallAllTitle(sk.Name),
		Why:     locales.SkillInstallWhy(),
		Action:  locales.SkillCatalogInstall(),
		Steps:   []Step{{Skill: &SkillInstall{Path: dest, Skill: sk, Links: links}}},
		Changes: append([]string{dest}, linkNotes...),
	}
	p.Notes = []Note{
		{Title: locales.SkillInstallSource(), Text: sk.Repository + " · " + sk.Commit, URL: sk.URL()},
		{Title: locales.FilesSection(), Text: strings.Join(files, "\n")},
		{Title: locales.SkillInstallReview(), Text: locales.SkillInstallReviewText()},
	}
	p.Notes = append(p.Notes, skillStorageNotes(sk.Name, dest, linkNotes)...)
	if len(titles) > 0 {
		p.Notes = append(p.Notes, Note{Title: locales.AvailableToAgents(), Text: strings.Join(titles, ", ")})
	}
	p.Verify = func(ctx context.Context) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		root, err := os.OpenRoot(dest)
		if err != nil {
			return "", fmt.Errorf("open installed skill: %w", err)
		}
		defer func() { _ = root.Close() }()
		if err = checkSkillFiles(root, sk.Files); err != nil {
			return "", err
		}
		for _, link := range links {
			if !sameSkillPath(link, dest) {
				return "", errors.New(locales.SkillCheckNotLinked(sk.Name, link))
			}
		}
		return locales.SkillCheckLinkedAll(sk.Name), nil
	}
	return p, nil
}

// SkillCreatePlan creates a new local skill folder with a starter SKILL.md in
// common storage (user or project scope) and links it to any agent whose skills
// directory differs from common storage.
func SkillCreatePlan(name, description, project string, ids []string, env sysenv.Env, names func(string) string) (Plan, error) {
	dir := strings.ToLower(strings.TrimSpace(name))
	dir = strings.NewReplacer(" ", "-", "_", "-").Replace(dir)
	if dir == "" || strings.Contains(dir, "/") || strings.Contains(dir, "\\") || !skillcatalog.ValidPath(dir) {
		return Plan{}, errors.New(locales.SkillCreateInvalidName())
	}
	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = "Describe when agents should use " + dir + "."
	}
	var sb strings.Builder
	sb.WriteString("---\nname: ")
	sb.WriteString(dir)
	sb.WriteString("\ndescription: ")
	sb.WriteString(strings.ReplaceAll(desc, "\n", " "))
	sb.WriteString("\n---\n\n# ")
	sb.WriteString(dir)
	sb.WriteString("\n\n")
	sb.WriteString(desc)
	sb.WriteString("\n")
	sk := skillcatalog.Skill{
		Name:        dir,
		Description: desc,
		Dir:         dir,
		Files:       []skillcatalog.File{{Path: "SKILL.md", Data: []byte(sb.String())}},
	}
	if err := sk.Validate(); err != nil {
		return Plan{}, errors.New(locales.SkillCreateInvalidName())
	}
	dest := filepath.Join(ai.SharedSkillsDir(env, project), sk.Dir)
	if !filepath.IsAbs(dest) {
		return Plan{}, errors.New(locales.SkillCatalogNoAgent())
	}
	if _, err := os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		return Plan{}, errors.New(locales.SkillLinkTaken(dest))
	}
	var links []string
	var linkNotes []string
	var titles []string
	for _, id := range ids {
		if id == "" || id == "aider" {
			continue
		}
		agentDir, _ := ai.SkillsDir(id, env, project)
		if agentDir == "" || !filepath.IsAbs(agentDir) {
			continue
		}
		if t := names(id); !slices.Contains(titles, t) {
			titles = append(titles, t)
		}
		link := filepath.Join(agentDir, sk.Dir)
		if link == dest || slices.Contains(links, link) {
			continue
		}
		if _, err := os.Lstat(link); !errors.Is(err, os.ErrNotExist) {
			return Plan{}, errors.New(locales.SkillLinkTaken(link))
		}
		links = append(links, link)
		linkNotes = append(linkNotes, link+" → "+dest)
	}
	p := Plan{
		Title:   locales.SkillCreatePlanTitle(sk.Name),
		Why:     locales.SkillCreateWhy(),
		Action:  locales.SkillCreateButton(),
		Steps:   []Step{{Skill: &SkillInstall{Path: dest, Skill: sk, Links: links}}},
		Changes: append([]string{dest}, linkNotes...),
	}
	p.Notes = append(skillStorageNotes(sk.Name, dest, linkNotes), Note{Title: locales.FilesSection(), Text: "SKILL.md"})
	if len(titles) > 0 {
		p.Notes = append(p.Notes, Note{Title: locales.AvailableToAgents(), Text: strings.Join(titles, ", ")})
	}
	p.Verify = func(ctx context.Context) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		root, err := os.OpenRoot(dest)
		if err != nil {
			return "", fmt.Errorf("open created skill: %w", err)
		}
		defer func() { _ = root.Close() }()
		if err = checkSkillFiles(root, sk.Files); err != nil {
			return "", err
		}
		for _, link := range links {
			if !sameSkillPath(link, dest) {
				return "", errors.New(locales.SkillCheckNotLinked(sk.Name, link))
			}
		}
		return locales.SkillCheckLinkedAll(sk.Name), nil
	}
	return p, nil
}

func checkSkillFiles(root *os.Root, files []skillcatalog.File) error {
	for _, f := range files {
		data, err := root.ReadFile(filepath.FromSlash(f.Path))
		if err != nil {
			return fmt.Errorf("check skill file: %w", err)
		}
		if !bytes.Equal(data, f.Data) {
			return fmt.Errorf("%s: %w", f.Path, ErrChanged)
		}
	}
	return nil
}

// applySkillInstall reserves the destination exclusively, writes SKILL.md last,
// and returns a rollback that refuses to delete files changed after installation.
func applySkillInstall(ctx context.Context, in SkillInstall) (func() error, error) {
	if err := in.Skill.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parent := filepath.Dir(in.Path)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return nil, fmt.Errorf("create skills folder: %w", err)
	}
	base, err := os.OpenRoot(parent)
	if err != nil {
		return nil, fmt.Errorf("open skills folder: %w", err)
	}
	defer func() { _ = base.Close() }()
	name := filepath.Base(in.Path)
	if err = base.Mkdir(name, 0755); err != nil {
		return nil, fmt.Errorf("create skill folder: %w", err)
	}
	root, err := base.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open new skill folder: %w", err)
	}
	defer func() { _ = root.Close() }()
	files := slices.Clone(in.Skill.Files)
	slices.SortStableFunc(files, func(a, b skillcatalog.File) int {
		if a.Path == b.Path {
			return 0
		}
		if a.Path == "SKILL.md" {
			return 1
		}
		if b.Path == "SKILL.md" {
			return -1
		}
		return strings.Compare(a.Path, b.Path)
	})
	var created []skillcatalog.File
	var dirs []string
	cleanup := func(root *os.Root) error {
		if err := checkSkillFiles(root, created); err != nil {
			return err
		}
		for _, f := range slices.Backward(created) {
			if err := root.Remove(filepath.FromSlash(f.Path)); err != nil {
				return err
			}
		}
		for _, d := range slices.Backward(dirs) {
			if err := root.Remove(filepath.FromSlash(d)); err != nil {
				return err
			}
		}
		return nil
	}
	for _, f := range files {
		if err = ctx.Err(); err != nil {
			break
		}
		parts := strings.Split(path.Dir(f.Path), "/")
		for i := range parts {
			d := strings.Join(parts[:i+1], "/")
			if d == "." {
				continue
			}
			if e := root.Mkdir(filepath.FromSlash(d), 0755); e == nil {
				dirs = append(dirs, d)
			} else if !errors.Is(e, os.ErrExist) {
				err = e
				break
			}
		}
		if err != nil {
			break
		}
		mode := os.FileMode(0644)
		if f.Executable {
			mode = 0755
		}
		var out *os.File
		out, err = root.OpenFile(filepath.FromSlash(f.Path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
		if err != nil {
			break
		}
		_, err = out.Write(f.Data)
		err = errors.Join(err, out.Close())
		if err != nil {
			_ = root.Remove(filepath.FromSlash(f.Path))
			break
		}
		created = append(created, f)
	}
	if err != nil {
		cleanErr := cleanup(root)
		if cleanErr == nil {
			_ = root.Close()
			cleanErr = base.Remove(name)
		}
		return nil, fmt.Errorf("install skill: %w", errors.Join(err, cleanErr))
	}
	links := in.allLinks()
	var madeLinks []string
	for _, link := range links {
		linkErr := os.MkdirAll(filepath.Dir(link), 0755)
		if linkErr == nil {
			result, err := (run.Exec{}).Run(ctx, run.Cmd{Args: linkCommand(in.Path, link), Timeout: time.Minute})
			linkErr = err
			if linkErr == nil && result.ExitCode != 0 {
				linkErr = fmt.Errorf("create skill link: exit %d: %s", result.ExitCode, result.Stderr)
			}
		}
		if linkErr != nil {
			var rollbackErr error
			for _, made := range slices.Backward(madeLinks) {
				rollbackErr = errors.Join(rollbackErr, os.Remove(made))
			}
			cleanErr := cleanup(root)
			if cleanErr == nil {
				_ = root.Close()
				cleanErr = base.Remove(name)
			}
			return nil, errors.Join(linkErr, rollbackErr, cleanErr)
		}
		madeLinks = append(madeLinks, link)
	}
	undo := func() error {
		root, err := os.OpenRoot(in.Path)
		if err != nil {
			return err
		}
		defer func() { _ = root.Close() }()
		for _, link := range links {
			if !sameSkillPath(link, in.Path) {
				return ErrChanged
			}
		}
		if err = cleanup(root); err != nil {
			return err
		}
		if err = root.Close(); err != nil {
			return err
		}
		for _, link := range slices.Backward(links) {
			if e := os.Remove(link); e != nil {
				return e
			}
		}
		return os.Remove(in.Path)
	}
	return undo, nil
}
