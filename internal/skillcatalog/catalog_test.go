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
