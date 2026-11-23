package skillcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const skillsShSearchBase = "https://skills.sh/api/search"

// SearchResult is one public skill entry from skills.sh.
// Installing always fetches the pinned GitHub repository archive via Fetch.
type SearchResult struct {
	ID       string `json:"id"`
	SkillID  string `json:"skillId"`
	Name     string `json:"name"`
	Source   string `json:"source"`
	Installs int    `json:"installs"`
}

// Repository returns the GitHub repository reference for this search result.
func (r SearchResult) Repository() Repository {
	return Repository{Name: r.Source, Slug: r.Source}
}

// TargetSkill returns the skill slug to preselect once the repository loads.
func (r SearchResult) TargetSkill() string {
	if r.SkillID != "" {
		return r.SkillID
	}
	return r.Name
}

// URL links to this skill's page on skills.sh.
func (r SearchResult) URL() string {
	if r.SkillID != "" {
		return "https://skills.sh/" + r.Source + "/" + r.SkillID
	}
	return "https://skills.sh/" + r.Source
}

// SearchSkillsSh queries the public skills.sh directory API without credentials.
func SearchSkillsSh(ctx context.Context, client *http.Client, query string, limit int) ([]SearchResult, error) {
	return searchSkillsShAt(ctx, client, skillsShSearchBase, query, limit)
}

func searchSkillsShAt(ctx context.Context, client *http.Client, endpoint, query string, limit int) ([]SearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	reqURL := endpoint + "?q=" + url.QueryEscape(q) + "&limit=" + strconv.Itoa(limit)
	data, err := get(ctx, client, reqURL, 2<<20)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Skills []SearchResult `json:"skills"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("read skills.sh response: %w", err)
	}
	out := make([]SearchResult, 0, len(payload.Skills))
	for _, r := range payload.Skills {
		r.Source = strings.TrimSpace(r.Source)
		r.Name = strings.TrimSpace(r.Name)
		r.SkillID = strings.TrimSpace(r.SkillID)
		if !validSlug(r.Source) || r.Name == "" {
			continue
		}
		if r.SkillID == "" {
			r.SkillID = r.Name
		}
		out = append(out, r)
	}
	return out, nil
}

// ParseSource parses a GitHub URL, owner/repo, skills.sh URL, or
// "npx skills add <source> [--skill <name>]" command. It returns the
// GitHub repository and an optional skill name/slug to preselect.
func ParseSource(raw string) (Repository, string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Repository{}, "", errors.New("enter a GitHub URL, skills.sh URL, or npx skills add command")
	}
	if repo, skill, matched, err := parseSkillsCLI(s); matched {
		return repo, skill, err
	}
	if repo, skill, matched, err := parseSkillsShURL(s); matched {
		return repo, skill, err
	}
	repo, err := ParseGitHubURL(s)
	return repo, "", err
}

func parseSkillsCLI(raw string) (Repository, string, bool, error) {
	fields := splitShellLike(raw)
	if len(fields) < 2 {
		return Repository{}, "", false, nil
	}
	// Strip leading runner prefixes: npx [-y|--yes], pnpm dlx, bunx, yarn dlx.
	i := 0
	switch strings.ToLower(fields[0]) {
	case "npx":
		i = 1
		for i < len(fields) && (fields[i] == "-y" || fields[i] == "--yes") {
			i++
		}
	case "bunx":
		i = 1
	case "pnpm", "yarn":
		if len(fields) >= 2 && strings.ToLower(fields[1]) == "dlx" {
			i = 2
		} else {
			return Repository{}, "", false, nil
		}
	case "skills":
		i = 0
	default:
		return Repository{}, "", false, nil
	}
	if i >= len(fields) {
		return Repository{}, "", false, nil
	}
	pkg := strings.ToLower(fields[i])
	if pkg != "skills" && !strings.HasPrefix(pkg, "skills@") {
		return Repository{}, "", false, nil
	}
	i++
	if i >= len(fields) || strings.ToLower(fields[i]) != "add" {
		return Repository{}, "", true, errors.New("expected: npx skills add <owner/repo> [--skill <name>]")
	}
	i++
	var source, skill string
	for i < len(fields) {
		tok := fields[i]
		switch {
		case tok == "--skill" || tok == "-s":
			if i+1 >= len(fields) {
				return Repository{}, "", true, errors.New("missing skill name after --skill")
			}
			skill = strings.TrimSpace(fields[i+1])
			i += 2
		case strings.HasPrefix(tok, "--skill="):
			skill = strings.TrimSpace(strings.TrimPrefix(tok, "--skill="))
			i++
		case strings.HasPrefix(tok, "-"):
			// Skip known flags with separate values (e.g. -a claude-code, --agent claude-code).
			if (tok == "-a" || tok == "--agent") && i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "-") {
				i += 2
				continue
			}
			i++
		default:
			if source == "" {
				source = tok
			}
			i++
		}
	}
	if source == "" {
		return Repository{}, "", true, errors.New("missing repository in skills add command")
	}
	if skill != "" && (!ValidPath(skill) || strings.Contains(skill, "/")) {
		return Repository{}, "", true, errors.New("invalid skill name")
	}
	if repo, urlSkill, matched, err := parseSkillsShURL(source); matched {
		if err != nil {
			return Repository{}, "", true, err
		}
		if skill == "" {
			skill = urlSkill
		}
		return repo, skill, true, nil
	}
	repo, err := ParseGitHubURL(source)
	if err != nil {
		return Repository{}, "", true, err
	}
	return repo, skill, true, nil
}

func parseSkillsShURL(raw string) (Repository, string, bool, error) {
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "https://skills.sh/") &&
		!strings.HasPrefix(lower, "http://skills.sh/") &&
		!strings.HasPrefix(lower, "https://www.skills.sh/") &&
		!strings.HasPrefix(lower, "http://www.skills.sh/") &&
		!strings.HasPrefix(lower, "skills.sh/") &&
		!strings.HasPrefix(lower, "www.skills.sh/") {
		return Repository{}, "", false, nil
	}
	s := raw
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return Repository{}, "", true, errors.New("invalid skills.sh URL")
	}
	host := strings.ToLower(u.Hostname())
	if (host != "skills.sh" && host != "www.skills.sh") || u.Port() != "" || u.User != nil {
		return Repository{}, "", true, errors.New("invalid skills.sh URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || len(parts) > 3 {
		return Repository{}, "", true, errors.New("use skills.sh/<owner>/<repo> or skills.sh/<owner>/<repo>/<skill>")
	}
	owner, repo := parts[0], parts[1]
	if !validGitHubSegment(owner) || !validGitHubSegment(repo) {
		return Repository{}, "", true, errors.New("invalid owner or repository in skills.sh URL")
	}
	var skill string
	if len(parts) == 3 {
		skill = parts[2]
		if !ValidPath(skill) || strings.Contains(skill, "/") {
			return Repository{}, "", true, errors.New("invalid skill name in skills.sh URL")
		}
	}
	slug := owner + "/" + repo
	return Repository{Name: slug, Slug: slug}, skill, true, nil
}

// splitShellLike splits a pasted CLI command on whitespace while respecting
// simple single- or double-quoted tokens (e.g. --skill 'grill-me').
func splitShellLike(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}
