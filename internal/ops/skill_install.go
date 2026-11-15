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

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/skillcatalog"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// SkillInstall creates a new skill directory from the exact previewed files.
// No file from the bundle is executed, and an existing directory is never reused.
type SkillInstall struct {
	Path  string
	Skill skillcatalog.Skill
}

// SkillInstallPlan reads the destination without creating it.
func SkillInstallPlan(sk skillcatalog.Skill, id string, env sysenv.Env, names func(string) string) (Plan, error) {
	if err := sk.Validate(); err != nil {
		return Plan{}, fmt.Errorf("prepare skill: %w", err)
	}
	dir, readers := ai.SkillsDir(id, env, "")
	if dir == "" || !filepath.IsAbs(dir) {
		return Plan{}, errors.New(locales.SkillNoDir(names(id)))
	}
	dest := filepath.Join(dir, sk.Dir)
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
	p := Plan{Title: locales.SkillInstallTitle(sk.Name, names(id)), Why: locales.SkillInstallWhy(), Action: locales.SkillCatalogInstall(), Steps: []Step{{Skill: &SkillInstall{dest, sk}}}, Changes: []string{dest}}
	p.Notes = []Note{{Title: locales.SkillInstallSource(), Text: sk.Repository + " · " + sk.Commit, URL: sk.URL()}, {Title: locales.FilesSection(), Text: strings.Join(files, "\n")}, {Title: locales.SkillInstallReview(), Text: locales.SkillInstallReviewText()}}
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
	undo := func() error {
		root, err := os.OpenRoot(in.Path)
		if err != nil {
			return err
		}
		defer func() { _ = root.Close() }()
		if err = cleanup(root); err != nil {
			return err
		}
		if err = root.Close(); err != nil {
			return err
		}
		return os.Remove(in.Path)
	}
	return undo, nil
}
