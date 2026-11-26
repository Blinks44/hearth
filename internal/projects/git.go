package projects

import (
	"bufio"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// GitInfo is the read-only Git repository summary for a project directory.
type GitInfo struct {
	Branch       string
	Detached     bool
	Dirty        bool
	Modified     int
	Remote       string
	RemoteURL    string
	LastCommit   string
	LastCommitAt time.Time
}

const gitReadTimeout = 1500 * time.Millisecond

// readGitInfo reads basic repository metadata for dir without mutating the
// repository or touching Git index locks.
func readGitInfo(dir string, env sysenv.Env) *GitInfo {
	gitDir, commonDir, ok := findGitDirs(dir)
	if !ok {
		return nil
	}
	info := &GitInfo{}
	info.Branch, info.Detached = readGitHead(gitDir)
	if raw := readGitOriginURL(commonDir); raw != "" {
		info.Remote, info.RemoteURL = parseGitRemote(raw)
	}
	if len(env) > 0 {
		if bin := findRealGit(env); bin != "" {
			enrichGitWithCLI(context.Background(), run.Exec{}, bin, dir, env, info)
		}
	}
	if info.Branch == "" && info.Remote == "" && info.LastCommit == "" && !info.Dirty {
		return nil
	}
	return info
}

func findRealGit(env sysenv.Env) string {
	for _, p := range sysenv.LookPathAll("git", env) {
		if !isGitStub(p) {
			return p
		}
	}
	return ""
}

func enrichGitWithCLI(ctx context.Context, r run.Runner, bin, dir string, env sysenv.Env, info *GitInfo) {
	gitEnv := append(append([]string(nil), env...), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	if res, err := r.Run(ctx, run.Cmd{
		Args:    []string{bin, "--no-optional-locks", "-C", dir, "status", "--porcelain=v1", "-b"},
		Env:     gitEnv,
		Timeout: gitReadTimeout,
	}); err == nil && res.ExitCode == 0 {
		parseGitStatus(string(res.Stdout), info)
	}
	if res, err := r.Run(ctx, run.Cmd{
		Args:    []string{bin, "--no-optional-locks", "-C", dir, "log", "-1", "--format=%ct%x00%s"},
		Env:     gitEnv,
		Timeout: gitReadTimeout,
	}); err == nil && res.ExitCode == 0 {
		parseGitLastCommit(string(res.Stdout), info)
	}
}

func parseGitStatus(out string, info *GitInfo) {
	modified := 0
	for i, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		if i == 0 && strings.HasPrefix(line, "## ") {
			header := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			switch {
			case strings.HasPrefix(header, "HEAD (no branch)"):
				info.Detached = true
			case strings.HasPrefix(header, "No commits yet on "):
				info.Branch = strings.TrimSpace(strings.TrimPrefix(header, "No commits yet on "))
				info.Detached = false
			case strings.HasPrefix(header, "Initial commit on "):
				info.Branch = strings.TrimSpace(strings.TrimPrefix(header, "Initial commit on "))
				info.Detached = false
			default:
				branch, _, _ := strings.Cut(header, "...")
				if branch != "" {
					info.Branch = branch
					info.Detached = false
				}
			}
			continue
		}
		modified++
	}
	info.Modified = modified
	info.Dirty = modified > 0
}

func parseGitLastCommit(out string, info *GitInfo) {
	line := strings.TrimRight(out, "\r\n")
	ts, subject, ok := strings.Cut(line, "\x00")
	if !ok {
		return
	}
	if sec, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64); err == nil && sec > 0 {
		info.LastCommitAt = time.Unix(sec, 0)
	}
	info.LastCommit = strings.TrimSpace(subject)
}

// findGitDirs locates the .git directory and common directory (for worktrees)
// starting at dir and walking up to 4 parent directories.
func findGitDirs(dir string) (gitDir, commonDir string, ok bool) {
	cur := filepath.Clean(dir)
	for range 5 {
		dot := filepath.Join(cur, ".git")
		st, err := os.Stat(dot)
		if err == nil {
			if st.IsDir() {
				return dot, dot, true
			}
			data, rerr := os.ReadFile(dot)
			if rerr != nil {
				return "", "", false
			}
			line := strings.TrimSpace(string(data))
			rest, has := strings.CutPrefix(line, "gitdir:")
			if !has {
				return "", "", false
			}
			target := strings.TrimSpace(rest)
			if !filepath.IsAbs(target) {
				target = filepath.Join(cur, target)
			}
			target = filepath.Clean(target)
			common := target
			if cdata, cerr := os.ReadFile(filepath.Join(target, "commondir")); cerr == nil {
				c := strings.TrimSpace(string(cdata))
				if c != "" {
					if !filepath.IsAbs(c) {
						c = filepath.Join(target, c)
					}
					common = filepath.Clean(c)
				}
			}
			return target, common, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", "", false
}

func readGitHead(gitDir string) (branch string, detached bool) {
	data, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(line, "ref:"); ok {
		ref = strings.TrimSpace(ref)
		ref = strings.TrimPrefix(ref, "refs/heads/")
		return ref, false
	}
	if len(line) >= 7 {
		return line[:7], true
	}
	return line, line != ""
}

func readGitOriginURL(commonDir string) string {
	f, err := os.Open(filepath.Join(commonDir, "config"))
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	inOrigin := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.TrimSpace(line[1 : len(line)-1])
			inOrigin = strings.EqualFold(section, `remote "origin"`)
			continue
		}
		if !inOrigin {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.EqualFold(strings.TrimSpace(k), "url") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// parseGitRemote extracts a credential-free display label and safe browser URL
// (https or http) from a Git remote URL.
func parseGitRemote(raw string) (display, browserURL string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	// SCP-like syntax: git@github.com:org/repo.git
	if !strings.Contains(raw, "://") {
		if at := strings.IndexByte(raw, '@'); at >= 0 {
			rest := raw[at+1:]
			host, path, ok := strings.Cut(rest, ":")
			if ok && host != "" && path != "" && !strings.ContainsAny(host, "/ \t") {
				path = strings.TrimPrefix(strings.TrimSuffix(path, ".git"), "/")
				if path == "" {
					return "", ""
				}
				return host + "/" + path, "https://" + host + "/" + path
			}
		}
		return "", ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ""
	}
	scheme := strings.ToLower(u.Scheme)
	host := u.Host
	if scheme == "ssh" || scheme == "git" || scheme == "git+ssh" {
		host = u.Hostname()
		scheme = "https"
	}
	if scheme != "https" && scheme != "http" {
		return "", ""
	}
	path := strings.TrimPrefix(strings.TrimSuffix(u.Path, ".git"), "/")
	if path == "" || host == "" {
		return "", ""
	}
	return host + "/" + path, scheme + "://" + host + "/" + path
}
