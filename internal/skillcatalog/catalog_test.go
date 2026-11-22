package skillcatalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func archive(t *testing.T, entries []File, special string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range entries {
		typ := byte(tar.TypeReg)
		if f.Path == special {
			typ = tar.TypeSymlink
		}
		if err := tw.WriteHeader(&tar.Header{Name: "repo-sha/skills/" + f.Path, Mode: 0644, Size: int64(len(f.Data)), Typeflag: typ}); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			if _, err := tw.Write(f.Data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadArchive(t *testing.T) {
	md := []byte("---\nname: example\ndescription: |\n  Useful skill\n---\n# Example\n")
	for _, tc := range []struct {
		name    string
		files   []File
		special string
		ok      bool
	}{
		{"files and metadata", []File{{"example/SKILL.md", md, false}, {"example/scripts/a.py", []byte{0, 1, 2}, false}}, "", true},
		{"symlink", []File{{"example/SKILL.md", md, false}, {"example/linked", nil, false}}, "example/linked", false},
		{"traversal", []File{{"example/SKILL.md", md, false}, {"example/../../escape", nil, false}}, "", false},
		{"windows path", []File{{"example/SKILL.md", md, false}, {"example/C:escape", nil, false}}, "", false},
		{"duplicate case", []File{{"example/SKILL.md", md, false}, {"example/skill.md", md, false}}, "", false},
		{"bad frontmatter", []File{{"example/SKILL.md", []byte("no frontmatter"), false}}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readArchive(t.Context(), archive(t, tc.files, tc.special), "owner/repo", "skills", "sha")
			if (err == nil) != tc.ok {
				t.Fatalf("skills=%v error=%v", got, err)
			}
			if tc.ok {
				if len(got) != 1 || got[0].Name != "example" || got[0].Description != "Useful skill" || len(got[0].Files) != 2 {
					t.Fatalf("got %+v", got)
				}
				if err := got[0].Validate(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetch(t *testing.T) {
	sha := strings.Repeat("a", 40)
	data := archive(t, []File{{"example/SKILL.md", []byte("---\nname: example\n---\nText"), false}}, "")
	calls := 0
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		body := []byte(fmt.Sprintf(`{"sha":%q}`, sha))
		if calls == 2 {
			if r.URL.String() != "https://codeload.github.com/anthropics/skills/tar.gz/"+sha {
				t.Fatal(r.URL)
			}
			body = data
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatal("unexpected credentials")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})}
	skills, err := Fetch(t.Context(), client, Repositories()[0])
	if err != nil || len(skills) != 1 || calls != 2 {
		t.Fatalf("skills %v calls %d err %v", skills, calls, err)
	}
	if skills[0].Commit != sha {
		t.Fatal("unpinned preview")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readArchive(ctx, data, "repo", "skills", sha); err == nil {
		t.Fatal("cancellation ignored")
	}
	client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limit"))}, nil
	})
	if _, err := Fetch(t.Context(), client, Repositories()[0]); err == nil {
		t.Fatal("HTTP error ignored")
	}
}

func TestValidPath(t *testing.T) {
	for _, name := range []string{"../a", "/a", "a/../b", "a\\b", "x:ads", "NUL.txt", "x/COM1.py", "x.", "x ", "a//b"} {
		t.Run(name, func(t *testing.T) {
			if ValidPath(name) {
				t.Fatal("unsafe path accepted")
			}
		})
	}
	if !ValidPath("references/hello.md") {
		t.Fatal("ordinary path refused")
	}
}

func TestDownloadLimit(t *testing.T) {
	client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("123456"))}, nil
	})}
	if _, err := get(t.Context(), client, "https://example.test/archive", 5); err == nil {
		t.Fatal("oversize download accepted")
	}
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []File
	}{
		{"duplicate", []File{{Path: "SKILL.md", Data: []byte("text")}, {Path: "skill.md"}}},
		{"traversal", []File{{Path: "SKILL.md", Data: []byte("text")}, {Path: "../outside"}}},
		{"missing instructions", []File{{Path: "readme.md"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := (Skill{Dir: "example", Files: tc.files}).Validate(); err == nil {
				t.Fatal("invalid bundle accepted")
			}
		})
	}
}

func TestReadArchiveNestedRoot(t *testing.T) {
	md := []byte("---\nname: gh-fix-ci\ndescription: Fix CI\n---\n")
	data := archive(t, []File{{".curated/gh-fix-ci/SKILL.md", md, false}, {".system/skill-installer/SKILL.md", md, false}}, "")
	got, err := readArchive(t.Context(), data, "openai/skills", "skills/.curated", "sha")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Dir != "gh-fix-ci" || got[0].URL() != "https://github.com/openai/skills/tree/sha/skills/.curated/gh-fix-ci" {
		t.Fatalf("got %+v", got)
	}
}

func archiveRaw(t *testing.T, entries []File) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range entries {
		if err := tw.WriteHeader(&tar.Header{Name: "repo-sha/" + f.Path, Mode: 0644, Size: int64(len(f.Data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(f.Data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseGitHubURL(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Repository
		ok   bool
	}{
		{"owner/repo", Repository{Name: "owner/repo", Slug: "owner/repo"}, true},
		{"https://github.com/owner/repo", Repository{Name: "owner/repo", Slug: "owner/repo"}, true},
		{"https://github.com/owner/repo.git/", Repository{Name: "owner/repo", Slug: "owner/repo"}, true},
		{"github.com/owner/repo/tree/dev/custom/skills", Repository{Name: "owner/repo", Slug: "owner/repo", Ref: "dev", Root: "custom/skills"}, true},
		{"https://github.com/owner/repo/tree/v1.2.0", Repository{Name: "owner/repo", Slug: "owner/repo", Ref: "v1.2.0"}, true},
		{"", Repository{}, false},
		{"https://gitlab.com/owner/repo", Repository{}, false},
		{"https://user:pass@github.com/owner/repo", Repository{}, false},
		{"https://github.com/owner/repo?tab=readme", Repository{}, false},
		{"https://github.com/owner/repo/blob/main/SKILL.md", Repository{}, false},
		{"https://github.com/owner/repo/tree/main/../escape", Repository{}, false},
	} {
		t.Run(tc.in, func(t *testing.T) {
			got, err := ParseGitHubURL(tc.in)
			if (err == nil) != tc.ok {
				t.Fatalf("got %+v err %v", got, err)
			}
			if tc.ok && got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestFetchCustomRepoAndLayouts(t *testing.T) {
	sha := strings.Repeat("b", 40)
	md := []byte("---\nname: solo\ndescription: Single skill repo\n---\n# Solo\n")
	rawData := archiveRaw(t, []File{{"SKILL.md", md, false}, {"scripts/run.sh", []byte("echo hi\n"), true}})
	var urls []string
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		urls = append(urls, r.URL.String())
		var body []byte
		switch r.URL.String() {
		case "https://api.github.com/repos/acme/solo-skill":
			body = []byte(`{"default_branch":"master"}`)
		case "https://api.github.com/repos/acme/solo-skill/commits/master":
			body = []byte(fmt.Sprintf(`{"sha":%q}`, sha))
		case "https://codeload.github.com/acme/solo-skill/tar.gz/" + sha:
			body = rawData
		default:
			t.Fatalf("unexpected URL %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})}
	repo, err := ParseGitHubURL("https://github.com/acme/solo-skill")
	if err != nil {
		t.Fatal(err)
	}
	skills, err := Fetch(t.Context(), client, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 1 || skills[0].Name != "solo" || skills[0].Dir != "solo-skill" || skills[0].URL() != "https://github.com/acme/solo-skill/tree/"+sha {
		t.Fatalf("skills = %+v", skills)
	}
	if len(urls) != 3 {
		t.Fatalf("urls = %v", urls)
	}

	// Root without skills/ prefix: both top-level dir and skills/<dir>.
	multi := archiveRaw(t, []File{
		{"alpha/SKILL.md", []byte("---\nname: alpha\n---\n"), false},
		{"skills/beta/SKILL.md", []byte("---\nname: beta\n---\n"), false},
	})
	got, err := readArchive(t.Context(), multi, "acme/pack", "", sha)
	if err != nil || len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("multi got %+v err %v", got, err)
	}
	if got[0].URL() != "https://github.com/acme/pack/tree/"+sha+"/alpha" || got[1].URL() != "https://github.com/acme/pack/tree/"+sha+"/skills/beta" {
		t.Fatalf("urls: %s, %s", got[0].URL(), got[1].URL())
	}
}

func TestCache(t *testing.T) {
	sha := strings.Repeat("c", 40)
	data := archive(t, []File{{"example/SKILL.md", []byte("---\nname: example\n---\nText"), false}}, "")
	calls := 0
	fail := false
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		calls++
		if fail {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("rate limit"))}, nil
		}
		body := []byte(fmt.Sprintf(`{"sha":%q}`, sha))
		if strings.Contains(r.URL.Host, "codeload") {
			body = data
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})}
	now := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	cache := &Cache{Dir: t.TempDir(), TTL: time.Hour, Now: func() time.Time { return now }}
	repo := Repositories()[0]

	// First fetch hits network and writes cache.
	got, err := cache.Fetch(t.Context(), client, repo, false)
	if err != nil || len(got) != 1 || calls != 2 {
		t.Fatalf("first fetch: len=%d calls=%d err=%v", len(got), calls, err)
	}
	// Fresh fetch uses cache with zero network calls.
	got, err = cache.Fetch(t.Context(), client, repo, false)
	if err != nil || len(got) != 1 || calls != 2 {
		t.Fatalf("cached fetch: len=%d calls=%d err=%v", len(got), calls, err)
	}
	// Expired cache with network failure falls back to stale entry.
	now = now.Add(2 * time.Hour)
	fail = true
	got, err = cache.Fetch(t.Context(), client, repo, false)
	if err != nil || len(got) != 1 {
		t.Fatalf("stale fallback: len=%d err=%v", len(got), err)
	}
	// Force refresh surfaces network error instead of stale cache.
	if _, err := cache.Fetch(t.Context(), client, repo, true); err == nil {
		t.Fatal("forced refresh ignored network error")
	}
	// Clear removes cache so next fetch hits network.
	fail = false
	before := calls
	if err := cache.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Fetch(t.Context(), client, repo, false); err != nil {
		t.Fatal(err)
	}
	if calls == before {
		t.Fatal("cache was not cleared")
	}
}
