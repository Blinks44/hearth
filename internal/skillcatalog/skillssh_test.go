package skillcatalog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchSkillsSh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected authorization header")
		}
		if got := r.URL.Query().Get("q"); got != "grill me" {
			t.Errorf("q = %q, want %q", got, "grill me")
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("limit = %q, want %q", got, "10")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"skills": [
				{"id": "mattpocock/skills/grill-me", "skillId": "grill-me", "name": "grill-me", "installs": 41200, "source": "mattpocock/skills"},
				{"id": "bad", "skillId": "evil", "name": "evil", "installs": 99, "source": "../bad/slug"},
				{"id": "empty", "skillId": "", "name": "", "installs": 1, "source": "acme/repo"}
			]
		}`))
	}))
	defer srv.Close()

	got, err := searchSkillsShAt(t.Context(), srv.Client(), srv.URL, "  grill me  ", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(got), got)
	}
	if got[0].Source != "mattpocock/skills" || got[0].TargetSkill() != "grill-me" || got[0].Installs != 41200 {
		t.Fatalf("unexpected result: %+v", got[0])
	}
	if got[0].Repository() != (Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}) {
		t.Fatalf("unexpected repo: %+v", got[0].Repository())
	}
	if got[0].URL() != "https://skills.sh/mattpocock/skills/grill-me" {
		t.Fatalf("unexpected url: %s", got[0].URL())
	}

	// Empty query returns nil without hitting the server.
	empty, err := searchSkillsShAt(t.Context(), srv.Client(), "http://127.0.0.1:0", "   ", 10)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty query: %+v, err=%v", empty, err)
	}
}

func TestParseSource(t *testing.T) {
	for _, tc := range []struct {
		in        string
		wantRepo  Repository
		wantSkill string
		ok        bool
	}{
		{"mattpocock/skills", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "", true},
		{"https://github.com/mattpocock/skills", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "", true},
		{"https://skills.sh/mattpocock/skills", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "", true},
		{"https://skills.sh/mattpocock/skills/grill-me", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "grill-me", true},
		{"skills.sh/vercel-labs/agent-skills/react-best-practices", Repository{Name: "vercel-labs/agent-skills", Slug: "vercel-labs/agent-skills"}, "react-best-practices", true},
		{"npx skills add mattpocock/skills --skill grill-me", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "grill-me", true},
		{"npx -y skills@latest add mattpocock/skills -s 'grill-me' -a claude-code", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "grill-me", true},
		{"pnpm dlx skills add https://github.com/mattpocock/skills --skill=grill-me", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "grill-me", true},
		{"bunx skills add https://skills.sh/mattpocock/skills/grill-me", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "grill-me", true},
		{"skills add mattpocock/skills", Repository{Name: "mattpocock/skills", Slug: "mattpocock/skills"}, "", true},
		{"", Repository{}, "", false},
		{"npx skills remove mattpocock/skills", Repository{}, "", false},
		{"npx skills add mattpocock/skills --skill ../escape", Repository{}, "", false},
		{"https://skills.sh/only-owner", Repository{}, "", false},
		{"https://skills.sh/owner/repo/a/b", Repository{}, "", false},
		{"https://gitlab.com/owner/repo", Repository{}, "", false},
	} {
		t.Run(tc.in, func(t *testing.T) {
			repo, skill, err := ParseSource(tc.in)
			if (err == nil) != tc.ok {
				t.Fatalf("got repo=%+v skill=%q err=%v", repo, skill, err)
			}
			if tc.ok && (repo != tc.wantRepo || skill != tc.wantSkill) {
				t.Fatalf("got (%+v, %q), want (%+v, %q)", repo, skill, tc.wantRepo, tc.wantSkill)
			}
		})
	}
}

func TestReadArchiveNestedCommunityLayouts(t *testing.T) {
	sha := strings.Repeat("e", 40)
	data := archiveRaw(t, []File{
		{".agents/skills/grill-me/SKILL.md", []byte("---\nname: grill-me\ndescription: Stress-test plans\n---\n# Grill Me\n"), false},
		{".claude/skills/grill-me/SKILL.md", []byte("---\nname: grill-me\ndescription: Mirror copy\n---\n# Mirror\n"), false},
		{"plugins/dev/skills/ship-it/SKILL.md", []byte("---\nname: ship-it\ndescription: Release helper\n---\n# Ship\n"), false},
	})
	got, err := readArchive(t.Context(), data, "mattpocock/skills", "", sha)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d skills, want 2: %+v", len(got), got)
	}
	// Deduplicated mirror prefers .agents/skills over .claude/skills.
	if got[0].Name != "grill-me" || got[0].Root != ".agents/skills" || got[0].Description != "Stress-test plans" {
		t.Fatalf("grill-me = %+v", got[0])
	}
	if got[1].Name != "ship-it" || got[1].Root != "plugins/dev/skills" {
		t.Fatalf("ship-it = %+v", got[1])
	}
}
