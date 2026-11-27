package skillcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestParseRepoURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Repository
		ok   bool
	}{
		{
			name: "github public shorthand",
			raw:  "owner/repo",
			want: Repository{Name: "owner/repo", Slug: "owner/repo"},
			ok:   true,
		},
		{
			name: "github blob SKILL.md",
			raw:  "https://github.com/owner/repo/blob/main/skills/review/SKILL.md",
			want: Repository{Name: "owner/repo", Slug: "owner/repo", Ref: "main", Root: "skills/review"},
			ok:   true,
		},
		{
			name: "github ssh scp",
			raw:  "git@github.com:private-org/agent-skills.git",
			want: Repository{
				Name:     "private-org/agent-skills",
				Slug:     "private-org/agent-skills",
				Host:     "github.com",
				CloneURL: "git@github.com:private-org/agent-skills.git",
				WebURL:   "https://github.com/private-org/agent-skills",
				Provider: "github",
			},
			ok: true,
		},
		{
			name: "gitlab nested subgroups https",
			raw:  "https://gitlab.com/acme/platform/ai/skills",
			want: Repository{
				Name:     "acme/platform/ai/skills",
				Slug:     "acme/platform/ai/skills",
				Host:     "gitlab.com",
				CloneURL: "https://gitlab.com/acme/platform/ai/skills.git",
				WebURL:   "https://gitlab.com/acme/platform/ai/skills",
				Provider: "gitlab",
			},
			ok: true,
		},
		{
			name: "self-hosted gitlab tree with ref and subpath",
			raw:  "https://gitlab.company.internal/team/sub/skills/-/tree/release-1/skills/go",
			want: Repository{
				Name:     "team/sub/skills",
				Slug:     "team/sub/skills",
				Ref:      "release-1",
				Root:     "skills/go",
				Host:     "gitlab.company.internal",
				CloneURL: "https://gitlab.company.internal/team/sub/skills.git",
				WebURL:   "https://gitlab.company.internal/team/sub/skills",
				Provider: "gitlab",
			},
			ok: true,
		},
		{
			name: "self-hosted gitlab blob SKILL.md",
			raw:  "https://gitlab.company.internal/team/skills/-/blob/main/my-skill/SKILL.md",
			want: Repository{
				Name:     "team/skills",
				Slug:     "team/skills",
				Ref:      "main",
				Root:     "my-skill",
				Host:     "gitlab.company.internal",
				CloneURL: "https://gitlab.company.internal/team/skills.git",
				WebURL:   "https://gitlab.company.internal/team/skills",
				Provider: "gitlab",
			},
			ok: true,
		},
		{
			name: "self-hosted gitlab scp ssh",
			raw:  "git@gitlab.company.internal:team/sub/skills.git",
			want: Repository{
				Name:     "team/sub/skills",
				Slug:     "team/sub/skills",
				Host:     "gitlab.company.internal",
				CloneURL: "git@gitlab.company.internal:team/sub/skills.git",
				WebURL:   "https://gitlab.company.internal/team/sub/skills",
				Provider: "gitlab",
			},
			ok: true,
		},
		{
			name: "corporate ssh scheme with custom port",
			raw:  "ssh://git@git.corp.local:2222/eng/skills.git",
			want: Repository{
				Name:     "eng/skills",
				Slug:     "eng/skills",
				Host:     "git.corp.local",
				CloneURL: "ssh://git@git.corp.local:2222/eng/skills.git",
				Provider: "git",
			},
			ok: true,
		},
		{
			name: "bare gitlab host shorthand",
			raw:  "gitlab.com/team/sub/skills",
			want: Repository{
				Name:     "team/sub/skills",
				Slug:     "team/sub/skills",
				Host:     "gitlab.com",
				CloneURL: "https://gitlab.com/team/sub/skills.git",
				WebURL:   "https://gitlab.com/team/sub/skills",
				Provider: "gitlab",
			},
			ok: true,
		},
		{name: "rejects embedded credentials", raw: "https://user:secret@gitlab.com/org/skills", ok: false},
		{name: "rejects file scheme", raw: "file:///tmp/skills", ok: false},
		{name: "rejects ext helper", raw: "ext::sh -c id", ok: false},
		{name: "rejects leading dash", raw: "--upload-pack=sh", ok: false},
		{name: "rejects local path", raw: "/Users/dev/skills", ok: false},
		{name: "rejects query string", raw: "https://gitlab.com/org/skills?private_token=123", ok: false},
		{name: "rejects non-SKILL.md blob", raw: "https://gitlab.com/org/skills/-/blob/main/README.md", ok: false},
		{name: "rejects path traversal subpath", raw: "https://gitlab.com/org/skills/-/tree/main/../secret", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseRepoURL(tc.raw)
			if (err == nil) != tc.ok {
				t.Fatalf("ParseRepoURL(%q) = (%+v, %v), want ok=%v", tc.raw, got, err, tc.ok)
			}
			if tc.ok && got != tc.want {
				t.Fatalf("ParseRepoURL(%q) =\n  %+v\nwant:\n  %+v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestFetchGitLocalRepo(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}

	origin := t.TempDir()
	mustWrite := func(rel, content string) {
		t.Helper()
		full := filepath.Join(origin, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Safe corporate deploy checklist\n---\n# Deploy\n")
	mustWrite("skills/deploy/checklist.md", "1. Verify staging\n")
	mustWrite("skills/symlinked/SKILL.md", "---\nname: symlinked\ndescription: Must be skipped because it contains a symlink\n---\n")
	if err := os.Symlink("/etc/hosts", filepath.Join(origin, "skills", "symlinked", "leak.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command(gitPath, args...)
		cmd.Dir = origin
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	gitRun("init", "-b", "main")
	gitRun("add", ".")
	gitRun("commit", "-m", "add skills")

	env := sysenv.Env{"PATH=" + filepath.Dir(gitPath)}
	repo := Repository{
		Name:     "acme/platform/skills",
		Slug:     "acme/platform/skills",
		Host:     "gitlab.company.internal",
		CloneURL: origin,
		WebURL:   "https://gitlab.company.internal/acme/platform/skills",
		Provider: "gitlab",
	}

	skills, err := FetchGit(context.Background(), run.Exec{}, env, repo)
	if err != nil {
		t.Fatalf("FetchGit error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills (%+v), want 1 (symlinked skill must be excluded)", len(skills), skills)
	}
	sk := skills[0]
	if sk.Name != "deploy" || sk.Dir != "deploy" {
		t.Fatalf("unexpected skill: %+v", sk)
	}
	if err := sk.Validate(); err != nil {
		t.Fatalf("skill.Validate failed: %v", err)
	}
	if !strings.HasPrefix(sk.URL(), "https://gitlab.company.internal/acme/platform/skills/-/tree/") || !strings.HasSuffix(sk.URL(), "/skills/deploy") {
		t.Fatalf("skill.URL() = %q, want gitlab tree URL", sk.URL())
	}

	// Subpath loading directly targeting a single skill folder.
	subRepo := repo
	subRepo.Root = "skills/deploy"
	subRepo.Ref = "main"
	subSkills, err := FetchGit(context.Background(), run.Exec{}, env, subRepo)
	if err != nil {
		t.Fatalf("FetchGit subpath error: %v", err)
	}
	if len(subSkills) != 1 || subSkills[0].Name != "deploy" {
		t.Fatalf("subpath skills = %+v, want [deploy]", subSkills)
	}
}

func TestFetchWithPrivateGitHubFallback(t *testing.T) {
	// Return HTTP 404 for the public GitHub archive check so FetchWith falls back to git.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := &http.Client{Transport: rewriteTransport{base: srv.URL, next: srv.Client().Transport}}
	fakeBin := t.TempDir()
	fakeGit := filepath.Join(fakeBin, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var calledGit bool
	r := gitTestRunner(func(c run.Cmd) (run.Result, error) {
		if len(c.Args) >= 2 && c.Args[1] == "clone" {
			calledGit = true
			dir := c.Args[len(c.Args)-1]
			skillDir := filepath.Join(dir, "skills", "private-skill")
			if err := os.MkdirAll(skillDir, 0o755); err != nil {
				return run.Result{}, err
			}
			if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: private-skill\ndescription: Internal guide\n---\n# Guide\n"), 0o644); err != nil {
				return run.Result{}, err
			}
			return run.Result{ExitCode: 0}, nil
		}
		if len(c.Args) >= 4 && c.Args[3] == "rev-parse" {
			return run.Result{ExitCode: 0, Stdout: []byte(strings.Repeat("a", 40) + "\n")}, nil
		}
		return run.Result{ExitCode: 0}, nil
	})
	repo := Repository{Name: "acme/private-skills", Slug: "acme/private-skills"}
	skills, err := FetchWith(context.Background(), client, r, sysenv.Env{"PATH=" + fakeBin}, repo)
	if err != nil {
		t.Fatalf("FetchWith fallback error: %v", err)
	}
	if !calledGit {
		t.Fatal("expected git fallback on HTTP 404")
	}
	if len(skills) != 1 || skills[0].Name != "private-skill" {
		t.Fatalf("got skills %+v, want [private-skill]", skills)
	}
}

type gitTestRunner func(run.Cmd) (run.Result, error)

func (f gitTestRunner) Run(_ context.Context, c run.Cmd) (run.Result, error) { return f(c) }

type rewriteTransport struct {
	base string
	next http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	u := clone.URL
	parsed, err := http.NewRequestWithContext(req.Context(), req.Method, rt.base+u.Path, req.Body)
	if err != nil {
		return nil, err
	}
	return rt.next.RoundTrip(parsed)
}
