// Package skillcatalog reads public skill repositories without running their code.
package skillcatalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Repository is a supported public source. Requests never carry credentials.
// Root is the folder that holds one directory per skill.
type Repository struct{ Name, Slug, Root string }

// Repositories returns the sources offered in the catalog. OpenAI keeps its
// installable skills in .curated; .system ships with Codex itself.
func Repositories() []Repository {
	return []Repository{
		{"Anthropic", "anthropics/skills", "skills"},
		{"OpenAI", "openai/skills", "skills/.curated"},
		{"Vercel", "vercel-labs/agent-skills", "skills"},
	}
}

// File is a repository file relative to a skill's directory.
type File struct {
	Path       string
	Data       []byte
	Executable bool
}

// Skill holds the exact files shown in the preview and later installed.
type Skill struct {
	Name, Description, Dir, Repository, Root, Commit string
	Files                                            []File
}

// URL links to this immutable version of the skill.
func (s Skill) URL() string {
	return "https://github.com/" + s.Repository + "/tree/" + s.Commit + "/" + s.Root + "/" + s.Dir
}

// Text returns SKILL.md, whose encoding is checked during loading.
func (s Skill) Text() string {
	for _, f := range s.Files {
		if f.Path == "SKILL.md" {
			return string(f.Data)
		}
	}
	return ""
}

const maxArchive = 32 << 20
const maxExpanded = 64 << 20
const maxFile = 8 << 20

// Fetch loads a bounded archive pinned to a commit. The caller owns the timeout.
func Fetch(ctx context.Context, client *http.Client, repo Repository) ([]Skill, error) {
	if !slices.Contains(Repositories(), repo) {
		return nil, errors.New("unknown skill repository")
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	data, err := get(ctx, client, "https://api.github.com/repos/"+repo.Slug+"/commits/main", 1<<20)
	if err != nil {
		return nil, err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err = json.Unmarshal(data, &commit); err != nil {
		return nil, fmt.Errorf("read repository commit: %w", err)
	}
	if len(commit.SHA) != 40 || strings.ContainsFunc(commit.SHA, func(r rune) bool { return (r < '0' || r > '9') && (r < 'a' || r > 'f') }) {
		return nil, errors.New("invalid repository commit")
	}
	data, err = get(ctx, client, "https://codeload.github.com/"+repo.Slug+"/tar.gz/"+commit.SHA, maxArchive)
	if err != nil {
		return nil, err
	}
	return readArchive(ctx, data, repo.Slug, repo.Root, commit.SHA)
}

func get(ctx context.Context, client *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("request repository: %w", err)
	}
	req.Header.Set("User-Agent", "Hearth")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download repository: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download repository: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read repository: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, errors.New("repository download exceeds size limit")
	}
	return data, nil
}

// ValidPath accepts portable relative file names, including on Windows.
func ValidPath(name string) bool {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, `\\:`) {
		return false
	}
	for part := range strings.SplitSeq(name, "/") {
		if strings.TrimRight(part, ". ") != part || strings.ContainsAny(part, "<>\"|?*") || strings.ContainsFunc(part, func(r rune) bool { return r < 32 }) {
			return false
		}
		stem, _, _ := strings.Cut(strings.ToUpper(part), ".")
		if slices.Contains([]string{"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$"}, stem) || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
			return false
		}
	}
	return true
}

func readArchive(ctx context.Context, data []byte, repo, root, commit string) ([]Skill, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open skill archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	limited := &io.LimitedReader{R: gz, N: maxExpanded + 1}
	tr := tar.NewReader(limited)
	groups := map[string][]File{}
	blocked := map[string]bool{}
	seen := map[string]bool{}
	total, count := int64(0), 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			if limited.N <= 0 {
				return nil, errors.New("skill archive exceeds size limit")
			}
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read skill archive: %w", err)
		}
		count++
		total += h.Size
		if count > 10000 || total > maxExpanded {
			return nil, errors.New("skill archive exceeds size limit")
		}
		_, rel, _ := strings.Cut(h.Name, "/")
		inside, ok := strings.CutPrefix(rel, root+"/")
		if !ok {
			continue
		}
		dir, file, ok := strings.Cut(inside, "/")
		if !ok || file == "" || h.Typeflag == tar.TypeDir {
			continue
		}
		if !ValidPath(dir) || !ValidPath(file) || h.Typeflag != tar.TypeReg || h.Size > maxFile {
			blocked[dir] = true
			continue
		}
		key := strings.ToLower(dir + "/" + file)
		if seen[key] {
			return nil, errors.New("duplicate file in skill archive")
		}
		seen[key] = true
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read skill file: %w", err)
		}
		groups[dir] = append(groups[dir], File{file, body, h.Mode&0111 != 0})
	}
	var out []Skill
	for dir, files := range groups {
		if blocked[dir] {
			continue
		}
		s := Skill{Dir: dir, Repository: repo, Root: root, Commit: commit, Files: files}
		text := strings.ReplaceAll(strings.TrimPrefix(s.Text(), "\ufeff"), "\r\n", "\n")
		rest, ok := strings.CutPrefix(text, "---\n")
		front, _, closed := strings.Cut(rest, "\n---")
		if !ok || !closed || !utf8.ValidString(text) {
			continue
		}
		var meta struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}
		if yaml.Unmarshal([]byte(front), &meta) != nil || strings.TrimSpace(meta.Name) == "" {
			continue
		}
		s.Name, s.Description = meta.Name, meta.Description
		slices.SortFunc(s.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	if len(out) == 0 {
		return nil, errors.New("repository contains no installable skills")
	}
	return out, nil
}

// Validate checks a preview again before the operation accepts it.
func (s Skill) Validate() error {
	if !ValidPath(s.Dir) || strings.Contains(s.Dir, "/") || s.Text() == "" || len(s.Files) == 0 || len(s.Files) > 2048 {
		return errors.New("invalid skill bundle")
	}
	seen := map[string]bool{}
	total := 0
	for _, f := range s.Files {
		key := strings.ToLower(f.Path)
		if !ValidPath(f.Path) || seen[key] || len(f.Data) > maxFile {
			return errors.New("invalid skill file")
		}
		seen[key] = true
		total += len(f.Data)
	}
	if total > maxExpanded {
		return errors.New("skill exceeds size limit")
	}
	return nil
}
