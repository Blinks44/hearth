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
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Repository is a skill repository source. Public GitHub repositories fetch a
// commit-pinned archive without credentials; GitLab, SSH, and private/self-hosted
// repositories clone over git using the user's own SSH keys or credential helper
// with GIT_TERMINAL_PROMPT=0 (Hearth never stores tokens or passwords).
// Root is the folder that holds one directory per skill (empty inspects root
// and skills/). Ref optionally pins a branch, tag, or commit; empty uses main
// for built-in repositories or the repository's default branch.
type Repository struct {
	Name, Slug, Root, Ref string
	// Host is the repository host (empty implies github.com for legacy GitHub repos).
	Host string
	// CloneURL, when non-empty, loads the repository via local git rather than
	// the public GitHub archive API.
	CloneURL string
	// WebURL is the optional HTTPS web root used to build browser links.
	WebURL string
	// Provider is "github", "gitlab", or "git" (empty implies "github").
	Provider string
}

// IsGit reports whether the repository should be fetched with local git.
func (r Repository) IsGit() bool { return r.CloneURL != "" }

// Key uniquely identifies a repository source for caching.
func (r Repository) Key() string {
	if r.CloneURL != "" {
		return r.CloneURL + "@" + r.Ref + ":" + r.Root
	}
	return r.Slug + "@" + r.Ref + ":" + r.Root
}

// SourceLabel returns a short provider/host badge for UI headers.
func (r Repository) SourceLabel() string {
	switch r.Provider {
	case "", "github":
		return "GitHub"
	case "gitlab":
		if r.Host != "" && r.Host != "gitlab.com" {
			return r.Host
		}
		return "GitLab"
	default:
		if r.Host != "" {
			return r.Host
		}
		return "Git"
	}
}

// CanonicalURL returns a user-facing URL string that ParseSource can parse back
// into the same repository, suitable for saving in user settings.
func (r Repository) CanonicalURL() string {
	if r.CloneURL != "" {
		if r.Ref == "" && r.Root == "" {
			return r.CloneURL
		}
		if r.WebURL != "" {
			ref := r.Ref
			if ref == "" {
				ref = "HEAD"
			}
			sep := "/tree/"
			if r.Provider == "gitlab" {
				sep = "/-/tree/"
			}
			u := strings.TrimRight(r.WebURL, "/") + sep + ref
			if r.Root != "" {
				u += "/" + r.Root
			}
			return u
		}
		return r.CloneURL
	}
	u := "https://github.com/" + r.Slug
	if r.Ref != "" {
		u += "/tree/" + r.Ref
		if r.Root != "" {
			u += "/" + r.Root
		}
	}
	return u
}

func (r Repository) treeBase(commit string) string {
	if commit == "" {
		return ""
	}
	switch r.Provider {
	case "", "github":
		base := r.WebURL
		if base == "" && r.Slug != "" {
			base = "https://github.com/" + r.Slug
		}
		if base == "" {
			return ""
		}
		return strings.TrimRight(base, "/") + "/tree/" + commit
	case "gitlab":
		if r.WebURL == "" {
			return ""
		}
		return strings.TrimRight(r.WebURL, "/") + "/-/tree/" + commit
	default:
		if r.WebURL == "" {
			return ""
		}
		return strings.TrimRight(r.WebURL, "/")
	}
}

// Repositories returns the sources offered in the catalog. OpenAI keeps its
// installable skills in .curated; .system ships with Codex itself.
func Repositories() []Repository {
	return []Repository{
		{Name: "Anthropic", Slug: "anthropics/skills", Root: "skills"},
		{Name: "OpenAI", Slug: "openai/skills", Root: "skills/.curated"},
		{Name: "Vercel", Slug: "vercel-labs/agent-skills", Root: "skills"},
	}
}

// ParseGitHubURL parses a public GitHub repository reference:
// "owner/repo", "https://github.com/owner/repo", or ".../tree/<ref>[/<subpath>]".
func ParseGitHubURL(raw string) (Repository, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Repository{}, errors.New("enter a GitHub repository URL or owner/repo")
	}
	if !strings.Contains(s, "://") {
		if strings.HasPrefix(strings.ToLower(s), "github.com/") || strings.HasPrefix(strings.ToLower(s), "www.github.com/") {
			s = "https://" + s
		} else {
			s = "https://github.com/" + strings.TrimPrefix(s, "/")
		}
	}
	u, err := url.Parse(s)
	if err != nil {
		return Repository{}, errors.New("invalid GitHub repository URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return Repository{}, errors.New("only GitHub HTTPS URLs are supported")
	}
	host := strings.ToLower(u.Hostname())
	if (host != "github.com" && host != "www.github.com") || u.Port() != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Repository{}, errors.New("only public github.com repository URLs are supported")
	}
	p := strings.Trim(u.Path, "/")
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return Repository{}, errors.New("repository must be owner/repo")
	}
	owner, repo := parts[0], parts[1]
	if len(parts) == 2 {
		repo = strings.TrimSuffix(repo, ".git")
	}
	if !validGitHubSegment(owner) || !validGitHubSegment(repo) {
		return Repository{}, errors.New("invalid GitHub owner or repository name")
	}
	var ref, root string
	switch {
	case len(parts) == 2:
	case len(parts) >= 4 && parts[2] == "tree":
		ref = parts[3]
		if !validRef(ref) {
			return Repository{}, errors.New("invalid git ref in URL")
		}
		if len(parts) > 4 {
			root = strings.Join(parts[4:], "/")
			if !ValidPath(root) {
				return Repository{}, errors.New("invalid subpath in URL")
			}
		}
	default:
		return Repository{}, errors.New("use github.com/owner/repo or github.com/owner/repo/tree/<branch>[/<path>]")
	}
	slug := owner + "/" + repo
	return Repository{Name: slug, Slug: slug, Root: root, Ref: ref}, nil
}

func validGitHubSegment(s string) bool {
	if s == "" || len(s) > 100 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func validSlug(slug string) bool {
	owner, repo, ok := strings.Cut(slug, "/")
	return ok && !strings.Contains(repo, "/") && validGitHubSegment(owner) && validGitHubSegment(repo)
}

func validRef(ref string) bool {
	if ref == "" || len(ref) > 200 || strings.Contains(ref, "..") {
		return false
	}
	return !strings.ContainsFunc(ref, func(r rune) bool {
		return r <= 32 || r == '?' || r == '#' || r == '\\' || r == '%'
	})
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
	WebBase                                          string `json:"webBase,omitempty"`
	Files                                            []File
}

// URL links to this immutable version of the skill, or "" when no web URL is known.
func (s Skill) URL() string {
	base := s.WebBase
	if base == "" {
		if !validSlug(s.Repository) || s.Commit == "" {
			return ""
		}
		base = "https://github.com/" + s.Repository + "/tree/" + s.Commit
	}
	if !strings.Contains(base, "/tree/") {
		return base
	}
	switch s.Root {
	case ".":
		return base
	case "":
		return base + "/" + s.Dir
	default:
		return base + "/" + s.Root + "/" + s.Dir
	}
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
	if !validSlug(repo.Slug) || (repo.Root != "" && !ValidPath(repo.Root)) || (repo.Ref != "" && !validRef(repo.Ref)) {
		return nil, errors.New("unknown skill repository")
	}
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	ref := repo.Ref
	if ref == "" {
		if slices.Contains(Repositories(), repo) {
			ref = "main"
		} else {
			meta, err := get(ctx, client, "https://api.github.com/repos/"+repo.Slug, 1<<20)
			if err != nil {
				return nil, err
			}
			var info struct {
				DefaultBranch string `json:"default_branch"`
			}
			if err := json.Unmarshal(meta, &info); err != nil {
				return nil, fmt.Errorf("read repository metadata: %w", err)
			}
			ref = info.DefaultBranch
			if ref == "" {
				ref = "main"
			}
			if !validRef(ref) {
				return nil, errors.New("invalid repository default branch")
			}
		}
	}
	data, err := get(ctx, client, "https://api.github.com/repos/"+repo.Slug+"/commits/"+url.PathEscape(ref), 1<<20)
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

type archiveGroup struct {
	root    string
	dir     string
	files   []File
	blocked bool
}

type rawEntry struct {
	rel string
	f   File
	bad bool
}

func readArchive(ctx context.Context, data []byte, repo, root, commit string) ([]Skill, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open skill archive: %w", err)
	}
	defer func() { _ = gz.Close() }()
	limited := &io.LimitedReader{R: gz, N: maxExpanded + 1}
	tr := tar.NewReader(limited)

	var entries []rawEntry
	total, count := int64(0), 0
	prefix := ""
	if root != "" {
		prefix = root + "/"
	}
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
		if rel == "" || h.Typeflag == tar.TypeDir {
			continue
		}
		inside := rel
		if prefix != "" {
			var ok bool
			inside, ok = strings.CutPrefix(rel, prefix)
			if !ok || inside == "" {
				continue
			}
		}
		if !ValidPath(inside) || h.Typeflag != tar.TypeReg || h.Size > maxFile {
			entries = append(entries, rawEntry{rel: inside, bad: true})
			continue
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read skill file: %w", err)
		}
		entries = append(entries, rawEntry{rel: inside, f: File{Path: inside, Data: body, Executable: h.Mode&0111 != 0}})
	}
	return buildSkills(entries, repo, root, commit, "")
}

func buildSkills(entries []rawEntry, repo, root, commit, webBase string) ([]Skill, error) {
	hasRootSkill := slices.ContainsFunc(entries, func(e rawEntry) bool { return !e.bad && e.rel == "SKILL.md" })
	groups := map[string]*archiveGroup{}
	add := func(key, gRoot, gDir, filePath string, e rawEntry) {
		g := groups[key]
		if g == nil {
			g = &archiveGroup{root: gRoot, dir: gDir}
			groups[key] = g
		}
		if e.bad || !ValidPath(gDir) || !ValidPath(filePath) {
			g.blocked = true
			return
		}
		f := e.f
		f.Path = filePath
		g.files = append(g.files, f)
	}

	switch {
	case hasRootSkill:
		gRoot, gDir := ".", ""
		if root != "" {
			gDir = path.Base(root)
			if d := path.Dir(root); d != "." {
				gRoot = d
			} else {
				gRoot = ""
			}
		} else {
			gDir = strings.TrimSuffix(path.Base(strings.TrimRight(repo, "/")), ".git")
		}
		for _, e := range entries {
			add(gDir, gRoot, gDir, e.rel, e)
		}
	case root != "":
		for _, e := range entries {
			dir, file, ok := strings.Cut(e.rel, "/")
			if !ok || file == "" {
				continue
			}
			add(root+"\x00"+dir, root, dir, file, e)
		}
	default:
		// Discover every directory containing a SKILL.md so community and
		// skills.sh repositories using skills/, .agents/skills/, .claude/skills/,
		// or plugins/*/skills/ layouts all load cleanly.
		var skillDirs []string
		seenDir := map[string]bool{}
		for _, e := range entries {
			if e.bad || !strings.HasSuffix(e.rel, "/SKILL.md") {
				continue
			}
			d := path.Dir(e.rel)
			if !seenDir[d] {
				seenDir[d] = true
				skillDirs = append(skillDirs, d)
			}
		}
		// Sort shorter paths first so nested fixture SKILL.md files inside a
		// skill stay part of the outer skill rather than becoming separate skills.
		slices.SortFunc(skillDirs, func(a, b string) int {
			if da, db := strings.Count(a, "/"), strings.Count(b, "/"); da != db {
				return da - db
			}
			return strings.Compare(a, b)
		})
		var roots []string
		for _, d := range skillDirs {
			if slices.ContainsFunc(roots, func(parent string) bool { return strings.HasPrefix(d, parent+"/") }) {
				continue
			}
			roots = append(roots, d)
		}
		for _, e := range entries {
			for _, d := range roots {
				if file, ok := strings.CutPrefix(e.rel, d+"/"); ok && file != "" {
					gRoot := path.Dir(d)
					if gRoot == "." {
						gRoot = ""
					}
					add(d, gRoot, path.Base(d), file, e)
					break
				}
			}
		}
	}

	byDir := map[string]Skill{}
	for _, g := range groups {
		if g.blocked {
			continue
		}
		seen := map[string]bool{}
		for _, f := range g.files {
			key := strings.ToLower(f.Path)
			if seen[key] {
				return nil, errors.New("duplicate file in skill archive")
			}
			seen[key] = true
		}
		s := Skill{Dir: g.dir, Repository: repo, Root: g.root, Commit: commit, WebBase: webBase, Files: g.files}
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
		key := strings.ToLower(s.Dir)
		if prev, exists := byDir[key]; !exists || preferSkillRoot(s.Root, prev.Root) {
			byDir[key] = s
		}
	}
	out := make([]Skill, 0, len(byDir))
	for _, s := range byDir {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	if len(out) == 0 {
		return nil, errors.New("repository contains no installable skills")
	}
	return out, nil
}

func skillRootRank(root string) int {
	switch root {
	case "skills":
		return 0
	case ".agents/skills":
		return 1
	case "":
		return 2
	case ".claude/skills":
		return 3
	default:
		return 4
	}
}

func preferSkillRoot(a, b string) bool {
	ra, rb := skillRootRank(a), skillRootRank(b)
	if ra != rb {
		return ra < rb
	}
	return a < b
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
