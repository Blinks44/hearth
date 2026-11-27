package skillcatalog

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const gitFetchTimeout = 90 * time.Second

// ParseRepoURL parses a GitHub, GitLab (gitlab.com or self-hosted, including
// nested subgroups), or generic SSH/HTTPS Git repository URL into a Repository.
// Embedded credentials (user:token@host), file://, ext::, and local paths are rejected.
func ParseRepoURL(raw string) (Repository, error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "-") {
		return Repository{}, errors.New("enter a GitHub, GitLab, or Git repository URL")
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r < 0x20 || r == 0x7f {
			return Repository{}, errors.New("invalid Git repository URL")
		}
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "file:") || strings.HasPrefix(lower, "ext::") || strings.HasPrefix(lower, "fd::") {
		return Repository{}, errors.New("local and helper Git URLs are not supported")
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "\\") || strings.HasPrefix(s, ".") {
		return Repository{}, errors.New("local paths are not supported")
	}

	switch {
	case strings.Contains(s, "://"):
		return parseSchemeRepoURL(s)
	case isSCPRepoURL(s):
		return parseSCPRepoURL(s)
	default:
		first, rest, ok := strings.Cut(s, "/")
		if !ok || rest == "" {
			return Repository{}, errors.New("enter owner/repo or a full Git repository URL")
		}
		if strings.Contains(first, ".") && !strings.ContainsAny(first, `:\`) {
			return parseSchemeRepoURL("https://" + s)
		}
		return ParseGitHubURL(s)
	}
}

func parseSchemeRepoURL(raw string) (Repository, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return Repository{}, errors.New("invalid Git repository URL")
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	hostPort := strings.ToLower(u.Host)

	switch scheme {
	case "ssh":
		if u.RawQuery != "" || u.Fragment != "" {
			return Repository{}, errors.New("invalid SSH repository URL")
		}
		repoPath := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
		if !validRepoPath(repoPath) {
			return Repository{}, errors.New("invalid repository path in SSH URL")
		}
		provider := detectGitProvider(host, false)
		webURL := ""
		if provider == "github" || provider == "gitlab" {
			webURL = "https://" + host + "/" + repoPath
		}
		return Repository{
			Name:     repoPath,
			Slug:     repoPath,
			Host:     host,
			CloneURL: raw,
			WebURL:   webURL,
			Provider: provider,
		}, nil

	case "https", "http":
		if u.User != nil {
			return Repository{}, errors.New("do not include tokens or passwords in the URL; use SSH keys or your git credential helper")
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return Repository{}, errors.New("repository URL must not include query parameters or fragments")
		}
		return parseHTTPWebRepoURL(scheme, host, hostPort, u.Path)

	default:
		return Repository{}, errors.New("only HTTPS and SSH Git repository URLs are supported")
	}
}

func parseHTTPWebRepoURL(scheme, host, hostPort, rawPath string) (Repository, error) {
	p := strings.Trim(rawPath, "/")
	if p == "" {
		return Repository{}, errors.New("repository path is required")
	}
	parts := strings.Split(p, "/")

	// Fast path for public github.com URLs (keeps the commit-pinned tarball flow,
	// with automatic git fallback if the repository is private).
	if (host == "github.com" || host == "www.github.com") && host == hostPort {
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
		case len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob"):
			var err error
			ref, root, err = parseRefAndRoot(parts[2], parts[3:])
			if err != nil {
				return Repository{}, err
			}
		default:
			return Repository{}, errors.New("use github.com/owner/repo or github.com/owner/repo/tree/<branch>[/<path>]")
		}
		slug := owner + "/" + repo
		return Repository{Name: slug, Slug: slug, Root: root, Ref: ref}, nil
	}

	// GitLab (gitlab.com or self-hosted) and generic HTTPS Git hosts.
	var (
		repoParts    []string
		ref, root    string
		gitlabMarker bool
	)
	if idx := findGitLabMarker(parts); idx >= 2 {
		gitlabMarker = true
		repoParts = parts[:idx]
		var err error
		ref, root, err = parseRefAndRoot(parts[idx+1], parts[idx+2:])
		if err != nil {
			return Repository{}, err
		}
	} else if idx := findTreeBlobMarker(parts); idx >= 2 {
		repoParts = parts[:idx]
		var err error
		ref, root, err = parseRefAndRoot(parts[idx], parts[idx+1:])
		if err != nil {
			return Repository{}, err
		}
	} else {
		repoParts = parts
	}

	if len(repoParts) < 2 {
		return Repository{}, errors.New("repository path must include at least group/repo")
	}
	lastIdx := len(repoParts) - 1
	trimmed := append([]string(nil), repoParts...)
	trimmed[lastIdx] = strings.TrimSuffix(trimmed[lastIdx], ".git")
	repoPath := strings.Join(trimmed, "/")
	if !validRepoPath(repoPath) {
		return Repository{}, errors.New("invalid repository path")
	}
	provider := detectGitProvider(host, gitlabMarker)
	webURL := fmt.Sprintf("%s://%s/%s", scheme, hostPort, repoPath)
	cloneURL := webURL + ".git"
	return Repository{
		Name:     repoPath,
		Slug:     repoPath,
		Root:     root,
		Ref:      ref,
		Host:     host,
		CloneURL: cloneURL,
		WebURL:   webURL,
		Provider: provider,
	}, nil
}

func parseSCPRepoURL(raw string) (Repository, error) {
	left, right, _ := strings.Cut(raw, ":")
	if right == "" || strings.HasPrefix(right, "/") {
		return Repository{}, errors.New("invalid SSH repository URL")
	}
	host := left
	if _, h, ok := strings.Cut(left, "@"); ok {
		host = h
	}
	host = strings.ToLower(host)
	repoPath := strings.TrimSuffix(strings.Trim(right, "/"), ".git")
	if !validRepoPath(repoPath) {
		return Repository{}, errors.New("invalid repository path in SSH URL")
	}
	provider := detectGitProvider(host, false)
	webURL := ""
	if provider == "github" || provider == "gitlab" {
		webURL = "https://" + host + "/" + repoPath
	}
	return Repository{
		Name:     repoPath,
		Slug:     repoPath,
		Host:     host,
		CloneURL: raw,
		WebURL:   webURL,
		Provider: provider,
	}, nil
}

func isSCPRepoURL(s string) bool {
	if strings.Contains(s, "://") {
		return false
	}
	left, right, ok := strings.Cut(s, ":")
	if !ok || left == "" || right == "" {
		return false
	}
	if len(left) == 1 && ((left[0] >= 'a' && left[0] <= 'z') || (left[0] >= 'A' && left[0] <= 'Z')) {
		return false
	}
	if strings.ContainsAny(left, `/\`) {
		return false
	}
	host := left
	if user, h, hasAt := strings.Cut(left, "@"); hasAt {
		if user == "" || h == "" {
			return false
		}
		host = h
	} else if !strings.Contains(host, ".") {
		return false
	}
	return !strings.ContainsAny(host, "@")
}

func findGitLabMarker(parts []string) int {
	for i := 2; i+2 < len(parts); i++ {
		if parts[i] == "-" && (parts[i+1] == "tree" || parts[i+1] == "blob") {
			return i
		}
	}
	return -1
}

func findTreeBlobMarker(parts []string) int {
	for i := 2; i+1 < len(parts); i++ {
		if parts[i] == "tree" || parts[i] == "blob" {
			return i
		}
	}
	return -1
}

func parseRefAndRoot(kind string, tail []string) (ref, root string, err error) {
	if len(tail) == 0 {
		return "", "", errors.New("missing git ref in URL")
	}
	ref = tail[0]
	if !validRef(ref) {
		return "", "", errors.New("invalid git ref in URL")
	}
	rest := tail[1:]
	if kind == "blob" {
		if len(rest) == 0 || rest[len(rest)-1] != "SKILL.md" {
			return "", "", errors.New("blob URLs must point to SKILL.md")
		}
		rest = rest[:len(rest)-1]
	}
	if len(rest) > 0 {
		root = strings.Join(rest, "/")
		if !ValidPath(root) {
			return "", "", errors.New("invalid subpath in URL")
		}
	}
	return ref, root, nil
}

func validRepoPath(p string) bool {
	if p == "" {
		return false
	}
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return false
	}
	for _, seg := range parts {
		if !validGitHubSegment(seg) {
			return false
		}
	}
	return true
}

func detectGitProvider(host string, gitlabMarker bool) string {
	switch {
	case host == "github.com" || host == "www.github.com":
		return "github"
	case gitlabMarker || host == "gitlab.com" || strings.Contains(host, "gitlab"):
		return "gitlab"
	default:
		return "git"
	}
}

// FetchWith loads skills from repo, using the public GitHub archive fast path
// when applicable and falling back to local git (with the user's SSH keys or
// credential helper and GIT_TERMINAL_PROMPT=0) for GitLab, SSH, self-hosted,
// or private GitHub repositories.
func FetchWith(ctx context.Context, client *http.Client, runner run.Runner, env sysenv.Env, repo Repository) ([]Skill, error) {
	if repo.IsGit() {
		return FetchGit(ctx, runner, env, repo)
	}
	skills, err := Fetch(ctx, client, repo)
	if err == nil {
		return skills, nil
	}
	if runner == nil || !shouldFallbackToGit(err) || !validSlug(repo.Slug) {
		return nil, err
	}
	gitRepo := repo
	gitRepo.Host = "github.com"
	gitRepo.Provider = "github"
	gitRepo.WebURL = "https://github.com/" + repo.Slug
	gitRepo.CloneURL = gitRepo.WebURL + ".git"
	gitSkills, gitErr := FetchGit(ctx, runner, env, gitRepo)
	if gitErr == nil {
		return gitSkills, nil
	}
	return nil, err
}

func shouldFallbackToGit(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "HTTP 401") || strings.Contains(msg, "HTTP 403")
}

// FetchGit clones repo into a temporary directory using local git through runner,
// never prompting for credentials (GIT_TERMINAL_PROMPT=0), and validates every
// file through the same safety checks as public archive loads.
func FetchGit(ctx context.Context, runner run.Runner, env sysenv.Env, repo Repository) ([]Skill, error) {
	if runner == nil {
		return nil, errors.New("git transport is not configured")
	}
	if strings.TrimSpace(repo.CloneURL) == "" || strings.HasPrefix(strings.TrimSpace(repo.CloneURL), "-") {
		return nil, errors.New("unknown git skill repository")
	}
	if (repo.Root != "" && !ValidPath(repo.Root)) || (repo.Ref != "" && !validRef(repo.Ref)) {
		return nil, errors.New("invalid repository ref or subpath")
	}
	gitExe := findGitExecutable(env)
	if gitExe == "" {
		return nil, errors.New("git is not installed")
	}

	tmp, err := os.MkdirTemp("", "hearth-skill-git-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir for skill clone: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	cloneDir := filepath.Join(tmp, "repo")
	gitEnv := append(env.Without("GIT_TERMINAL_PROMPT"), "GIT_TERMINAL_PROMPT=0")

	if err := cloneSkillRepo(ctx, runner, gitExe, gitEnv, repo, cloneDir); err != nil {
		return nil, err
	}

	rev, err := runner.Run(ctx, run.Cmd{
		Args:    []string{gitExe, "-C", cloneDir, "rev-parse", "HEAD"},
		Env:     gitEnv,
		Timeout: 15 * time.Second,
	})
	if err != nil || rev.ExitCode != 0 {
		return nil, fmt.Errorf("read cloned skill commit: %s", gitErrorSummary(rev, err))
	}
	commit := strings.TrimSpace(string(rev.Stdout))
	if len(commit) < 7 || strings.ContainsFunc(commit, func(r rune) bool { return (r < '0' || r > '9') && (r < 'a' || r > 'f') }) {
		return nil, errors.New("invalid cloned repository commit")
	}

	return readGitDir(ctx, cloneDir, repo, commit)
}

func findGitExecutable(env sysenv.Env) string {
	found := sysenv.LookPathAll("git", env)
	if len(found) > 0 {
		return found[0]
	}
	return ""
}

func cloneSkillRepo(ctx context.Context, runner run.Runner, gitExe string, env sysenv.Env, repo Repository, cloneDir string) error {
	args := []string{gitExe, "clone", "--depth", "1", "--single-branch"}
	if repo.Ref != "" && repo.Ref != "HEAD" {
		args = append(args, "--branch", repo.Ref)
	}
	args = append(args, "--", repo.CloneURL, cloneDir)
	res, err := runner.Run(ctx, run.Cmd{Args: args, Env: env, Timeout: gitFetchTimeout})
	if err == nil && res.ExitCode == 0 {
		return nil
	}
	// If ref is a commit SHA or non-branch ref, git clone --branch fails.
	// Retry with a shallow clone of the default branch followed by fetch+checkout.
	if repo.Ref != "" && repo.Ref != "HEAD" && ctx.Err() == nil {
		_ = os.RemoveAll(cloneDir)
		res2, err2 := runner.Run(ctx, run.Cmd{
			Args:    []string{gitExe, "clone", "--depth", "1", "--", repo.CloneURL, cloneDir},
			Env:     env,
			Timeout: gitFetchTimeout,
		})
		if err2 == nil && res2.ExitCode == 0 {
			fres, ferr := runner.Run(ctx, run.Cmd{
				Args:    []string{gitExe, "-C", cloneDir, "fetch", "--depth", "1", "origin", "--", repo.Ref},
				Env:     env,
				Timeout: gitFetchTimeout,
			})
			if ferr == nil && fres.ExitCode == 0 {
				cres, cerr := runner.Run(ctx, run.Cmd{
					Args:    []string{gitExe, "-C", cloneDir, "checkout", "--detach", "FETCH_HEAD"},
					Env:     env,
					Timeout: 30 * time.Second,
				})
				if cerr == nil && cres.ExitCode == 0 {
					return nil
				}
				return fmt.Errorf("checkout ref %s: %s", repo.Ref, gitErrorSummary(cres, cerr))
			}
		}
	}
	return fmt.Errorf("git clone failed: %s", gitErrorSummary(res, err))
}

func gitErrorSummary(res run.Result, err error) string {
	if err != nil {
		return err.Error()
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(res.Stderr)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 200 {
				line = line[:200] + "…"
			}
			return line
		}
	}
	return fmt.Sprintf("exit %d", res.ExitCode)
}

func readGitDir(ctx context.Context, cloneDir string, repo Repository, commit string) ([]Skill, error) {
	walkRoot := cloneDir
	if repo.Root != "" {
		walkRoot = filepath.Join(cloneDir, filepath.FromSlash(repo.Root))
		info, err := os.Lstat(walkRoot)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("subpath %q not found in repository", repo.Root)
		}
	}
	var entries []rawEntry
	total, count := int64(0), 0
	err := filepath.WalkDir(walkRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == walkRoot {
			return nil
		}
		name := d.Name()
		if name == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		relFS, err := filepath.Rel(walkRoot, p)
		if err != nil {
			return err
		}
		inside := filepath.ToSlash(relFS)
		// Never follow symlinks: mark them bad so any skill containing symlinks is skipped.
		if d.Type()&fs.ModeSymlink != 0 {
			entries = append(entries, rawEntry{rel: inside, bad: true})
			return nil
		}
		if d.IsDir() {
			return nil
		}
		count++
		if count > 10000 {
			return errors.New("skill repository exceeds file count limit")
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		size := info.Size()
		total += size
		if total > maxExpanded {
			return errors.New("skill repository exceeds size limit")
		}
		if !ValidPath(inside) || !info.Mode().IsRegular() || size > maxFile {
			entries = append(entries, rawEntry{rel: inside, bad: true})
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read skill file: %w", err)
		}
		entries = append(entries, rawEntry{
			rel: inside,
			f: File{
				Path:       inside,
				Data:       body,
				Executable: info.Mode()&0o111 != 0,
			},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	label := repo.Slug
	if repo.Host != "" && repo.Host != "github.com" {
		label = repo.Host + "/" + repo.Slug
	}
	return buildSkills(entries, label, repo.Root, commit, repo.treeBase(commit))
}
