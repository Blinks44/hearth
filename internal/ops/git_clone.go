package ops

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const gitCloneTimeout = 5 * time.Minute

// ParseGitCloneURL validates a remote Git repository URL (SSH SCP-style,
// ssh://, https://, http://, git://, or bare host.tld/org/repo shorthand) and
// returns the normalized clone URL together with the suggested folder name.
// Local paths, file://, ext::, leading dashes, and whitespace are rejected.
func ParseGitCloneURL(raw string) (cloneURL, defaultName string, err error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "-") {
		return "", "", errors.New(locales.GitCloneErrURL())
	}
	for _, r := range s {
		if unicode.IsSpace(r) || r < 0x20 || r == 0x7f {
			return "", "", errors.New(locales.GitCloneErrURL())
		}
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "ext::") || strings.HasPrefix(lower, "fd::") || strings.HasPrefix(lower, "file:") {
		return "", "", errors.New(locales.GitCloneErrURL())
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "\\") || strings.HasPrefix(s, ".") {
		return "", "", errors.New(locales.GitCloneErrURL())
	}

	var repoPath string
	switch {
	case strings.Contains(s, "://"):
		u, parseErr := url.Parse(s)
		if parseErr != nil || u.Host == "" {
			return "", "", errors.New(locales.GitCloneErrURL())
		}
		switch strings.ToLower(u.Scheme) {
		case "https", "http", "ssh", "git":
		default:
			return "", "", errors.New(locales.GitCloneErrURL())
		}
		repoPath = u.Path
		cloneURL = s
	case isSCPGitURL(s):
		_, pathPart, _ := strings.Cut(s, ":")
		if pathPart == "" || strings.HasPrefix(pathPart, "/") {
			return "", "", errors.New(locales.GitCloneErrURL())
		}
		repoPath = pathPart
		cloneURL = s
	default:
		// Bare host shorthand, e.g. github.com/org/repo or gitlab.company.internal/g/sub/repo.
		host, rest, ok := strings.Cut(s, "/")
		if !ok || !strings.Contains(host, ".") || strings.ContainsAny(host, `:\`) || rest == "" {
			return "", "", errors.New(locales.GitCloneErrURL())
		}
		u, parseErr := url.Parse("https://" + s)
		if parseErr != nil || u.Host == "" {
			return "", "", errors.New(locales.GitCloneErrURL())
		}
		repoPath = u.Path
		cloneURL = "https://" + s
	}

	defaultName = extractCloneFolderName(repoPath)
	if ValidateCloneFolderName(defaultName) != nil {
		return "", "", errors.New(locales.GitCloneErrURL())
	}
	return cloneURL, defaultName, nil
}

// isSCPGitURL reports whether s matches the SCP-style SSH syntax [user@]host:path.
func isSCPGitURL(s string) bool {
	if strings.Contains(s, "://") {
		return false
	}
	left, right, ok := strings.Cut(s, ":")
	if !ok || left == "" || right == "" {
		return false
	}
	// Reject Windows drive letters like C:\repo or C:/repo.
	if len(left) == 1 && ((left[0] >= 'a' && left[0] <= 'z') || (left[0] >= 'A' && left[0] <= 'Z')) {
		return false
	}
	if strings.ContainsAny(left, `/\`) {
		return false
	}
	// Require user@host:path or a dotted/qualified hostname so random "foo:bar"
	// strings are not misclassified as SSH URLs.
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

func extractCloneFolderName(repoPath string) string {
	trimmed := strings.Trim(repoPath, "/")
	if trimmed == "" {
		return ""
	}
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	last = strings.TrimSuffix(last, ".git")
	return last
}

// ValidateCloneFolderName checks that name is a safe single directory name.
func ValidateCloneFolderName(name string) error {
	if name == "" || strings.TrimSpace(name) != name {
		return errors.New(locales.GitCloneErrFolder())
	}
	if name == "." || name == ".." {
		return errors.New(locales.GitCloneErrFolder())
	}
	for _, r := range name {
		if r == '/' || r == '\\' || r == ':' || r < 0x20 || r == 0x7f {
			return errors.New(locales.GitCloneErrFolder())
		}
	}
	return nil
}

// ExpandCloneParent resolves a leading ~/ against home.
func ExpandCloneParent(parent, home string) string {
	parent = strings.TrimSpace(parent)
	if home == "" || parent == "" {
		return parent
	}
	if parent == "~" {
		return home
	}
	if strings.HasPrefix(parent, "~/") || strings.HasPrefix(parent, `~\`) {
		return filepath.Join(home, parent[2:])
	}
	return parent
}

func resolveCloneTarget(repoURL, parentDir, folderName string, env sysenv.Env) (cloneURL, parent, folder, target string, err error) {
	cloneURL, defaultName, err := ParseGitCloneURL(repoURL)
	if err != nil {
		return "", "", "", "", err
	}
	folder = strings.TrimSpace(folderName)
	if folder == "" {
		folder = defaultName
	}
	if err := ValidateCloneFolderName(folder); err != nil {
		return "", "", "", "", err
	}
	home := env.Get("HOME")
	if home == "" {
		home = env.Get("USERPROFILE")
	}
	parent = ExpandCloneParent(parentDir, home)
	if parent == "" || !filepath.IsAbs(parent) {
		return "", "", "", "", errors.New(locales.GitCloneErrParent())
	}
	parent = filepath.Clean(parent)
	target = filepath.Join(parent, folder)
	return cloneURL, parent, folder, target, nil
}

func checkCloneTarget(parent, target string, env sysenv.Env) error {
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return errors.New(locales.GitCloneErrParent())
	}
	if _, err := os.Lstat(target); err == nil {
		return errors.New(locales.GitCloneErrExists(tildeIn(target, env)))
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check destination %s: %w", target, err)
	}
	return nil
}

func verifyGitClone(target, folder string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) {
		if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
			return "", errors.New(locales.GitCloneErrMissingGitDir())
		}
		return locales.GitCloneDone(folder), nil
	}
}

// GitClonePlan builds a plan that clones repoURL into filepath.Join(parentDir, folderName)
// using an existing Git binary, with GIT_TERMINAL_PROMPT=0 so it never hangs waiting for input.
func GitClonePlan(gitPath, repoURL, parentDir, folderName string, env sysenv.Env) (Plan, error) {
	cloneURL, parent, folder, target, err := resolveCloneTarget(repoURL, parentDir, folderName, env)
	if err != nil {
		return Plan{}, err
	}
	if err := checkCloneTarget(parent, target, env); err != nil {
		return Plan{}, err
	}
	exe := strings.TrimSpace(gitPath)
	if exe == "" {
		exe = "git"
	}
	return Plan{
		Title:  locales.GitClonePlanTitle(folder),
		Why:    locales.GitClonePlanWhy(cloneURL, tildeIn(target, env)),
		Action: locales.GitCloneConfirm(),
		Env:    slices.Clone(env),
		Before: func(context.Context) error {
			return checkCloneTarget(parent, target, env)
		},
		Steps: []Step{{
			Argv:    []string{exe, "clone", "--", cloneURL, target},
			Env:     []string{"GIT_TERMINAL_PROMPT=0"},
			Timeout: gitCloneTimeout,
		}},
		Changes: []string{"+ " + tildeIn(target, env)},
		Verify:  verifyGitClone(target, folder),
	}, nil
}

// GitCloneWithInstallPlan builds a composite plan that installs Git via installPlan
// and then clones repoURL into filepath.Join(parentDir, folderName). Parts keeps the
// Git installation separate from the clone step so a clone failure does not roll back Git.
func GitCloneWithInstallPlan(installPlan Plan, repoURL, parentDir, folderName string, env sysenv.Env) (Plan, error) {
	cloneURL, parent, folder, target, err := resolveCloneTarget(repoURL, parentDir, folderName, env)
	if err != nil {
		return Plan{}, err
	}
	if err := checkCloneTarget(parent, target, env); err != nil {
		return Plan{}, err
	}
	steps := append(slices.Clone(installPlan.Steps), Step{
		Argv:    []string{"git", "clone", "--", cloneURL, target},
		Env:     cloneStepEnvWithFallback(env),
		Timeout: gitCloneTimeout,
	})
	var parts []int
	if len(installPlan.Parts) > 0 {
		parts = slices.Clone(installPlan.Parts)
	} else if len(installPlan.Steps) > 0 {
		parts = []int{0}
	}
	if len(installPlan.Steps) > 0 {
		parts = append(parts, len(installPlan.Steps))
	}
	planEnv := installPlan.Env
	if len(planEnv) == 0 {
		planEnv = slices.Clone(env)
	}
	prevBefore := installPlan.Before
	return Plan{
		Title:  locales.GitCloneInstallPlanTitle(folder),
		Why:    locales.GitCloneInstallPlanWhy(cloneURL, tildeIn(target, env)),
		Action: locales.GitCloneInstallConfirm(),
		Env:    planEnv,
		Notes:  slices.Clone(installPlan.Notes),
		Before: func(ctx context.Context) error {
			if err := checkCloneTarget(parent, target, env); err != nil {
				return err
			}
			if prevBefore != nil {
				return prevBefore(ctx)
			}
			return nil
		},
		Steps:           steps,
		Parts:           parts,
		RestartTerminal: true,
		Changes:         append(slices.Clone(installPlan.Changes), "+ "+tildeIn(target, env)),
		Verify:          verifyGitClone(target, folder),
	}, nil
}

// cloneStepEnvWithFallback returns per-step env entries for the clone step after
// a fresh Git install: disables interactive prompts and adds standard OS Git
// installation directories to PATH so ops.Execute finds the newly installed git.
func cloneStepEnvWithFallback(env sysenv.Env) []string {
	out := []string{"GIT_TERMINAL_PROMPT=0"}
	cur := env.Get("PATH")
	extra := gitFallbackPaths(env)
	if len(extra) == 0 {
		return out
	}
	existing := filepath.SplitList(cur)
	seen := make(map[string]bool, len(existing))
	for _, d := range existing {
		seen[filepath.Clean(d)] = true
	}
	var prepend []string
	for _, d := range extra {
		if d == "" {
			continue
		}
		c := filepath.Clean(d)
		if !seen[c] {
			seen[c] = true
			prepend = append(prepend, c)
		}
	}
	if len(prepend) == 0 {
		return out
	}
	parts := append(prepend, existing...)
	return append(out, "PATH="+strings.Join(parts, string(filepath.ListSeparator)))
}
