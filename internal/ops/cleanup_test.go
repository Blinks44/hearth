package ops

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/disk"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// pathWith is an environment whose PATH has programs of these names.
func pathWith(t *testing.T, names ...string) sysenv.Env {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n+runtimeExe), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return sysenv.Env{"PATH=" + dir}
}

// A cache goes through its tool's command when the terminal finds it, or
// else as its folder; Docker's through prune in the context counted;
// what the plan does not remove yet is left out.
func TestCleanupPlan(t *testing.T) {
	gb := inventory.Usage{Bytes: 2e9}
	cands := []cleanup.Candidate{
		{Kind: cleanup.Cache, ID: "pnpm", Paths: []string{"/c/pnpm"}, Usage: gb},
		{Kind: cleanup.Cache, ID: "npm", Paths: []string{"/c/npm"}, Usage: gb},
		{Kind: cleanup.Cache, ID: "cargo", Paths: []string{"/c/cargo"}, Usage: gb},
		{Kind: cleanup.Cache, ID: "pub", Paths: []string{"/c/pub"}, Usage: gb},
		{Kind: cleanup.BuildCache, Usage: gb},
		{Kind: cleanup.UnusedImages, Usage: gb},
		{Kind: cleanup.StoppedContainers, Usage: gb},
		{Kind: cleanup.ExtraCopy, ID: "node", Paths: []string{"/c/node"}, Usage: gb},
		{Kind: cleanup.ProjectInstalls, ID: "node_modules", Paths: []string{"/p/node_modules"}, Usage: gb},
	}
	titles := []string{"pnpm cache", "npm cache", "Cargo cache", "pub cache", "Build Cache", "Images", "Containers", "Node", "old: node_modules"}
	env := pathWith(t, "pnpm", "flutter")
	p, err := CleanupPlan(cands, titles, env, DockerTarget{CLI: "/usr/local/bin/docker", Context: "orbstack"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range p.Steps {
		if s.Remove != "" {
			got = append(got, "remove "+s.Remove)
		} else {
			got = append(got, Command(s.Argv))
		}
	}
	want := []string{
		"pnpm store prune",
		"remove /c/npm",
		"remove /c/cargo",
		"flutter pub cache clean --force",
		"/usr/local/bin/docker --context orbstack builder prune --force",
		"/usr/local/bin/docker --context orbstack image prune --all --force",
		"/usr/local/bin/docker --context orbstack container prune --force",
	}
	if !slices.Equal(got, want) {
		t.Errorf("steps\n%q\nwant\n%q", got, want)
	}
	if len(p.Changes) != 7 || p.Changes[0] != "− pnpm cache · 2.0 GB" {
		t.Errorf("changes %q", p.Changes)
	}
	if len(p.Notes) != 2 {
		t.Errorf("notes %+v, want the folders' and pub's", p.Notes)
	}
	// Without an engine Docker's are left out; with nothing left, no plan.
	p, err = CleanupPlan(cands[4:5], titles[4:5], env, DockerTarget{}, nil)
	if err == nil {
		t.Errorf("plan %+v without anything to remove", p)
	}
}

// The check says what was freed: a folder gone frees all it took, one
// cleaned some of it; Docker's, what it can prune less than before.
func TestCleanupFreed(t *testing.T) {
	gone, kept := filepath.Join(t.TempDir(), "gone"), t.TempDir()
	if err := os.WriteFile(filepath.Join(kept, "f"), make([]byte, 10_000), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := disk.Measure(t.Context(), kept)
	if err != nil {
		t.Fatal(err)
	}
	cands := []cleanup.Candidate{
		{Kind: cleanup.Cache, ID: "cargo", Paths: []string{gone}, Usage: inventory.Usage{Bytes: 3e9}},
		{Kind: cleanup.Cache, ID: "gradle", Paths: []string{kept}, Usage: inventory.Usage{Bytes: 1e9 + after.Bytes}},
		{Kind: cleanup.BuildCache, Usage: inventory.Usage{Bytes: 5e8}},
	}
	docker := func(context.Context) ([]inventory.DockerUsage, error) {
		return []inventory.DockerUsage{{Type: "Build Cache", Reclaimable: 0}, {Type: "Images", Reclaimable: 7e9}}, nil
	}
	p, err := CleanupPlan(cands, []string{"a", "b", "c"}, nil, DockerTarget{CLI: "docker", Context: "default"}, docker)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := p.Verify(t.Context()); err != nil || got != "Freed 4.5 GB" {
		t.Errorf("verify %q, %v", got, err)
	}
}

// A step that removes a folder removes all in it, read-only folders as
// Go's module cache too; a relative path is refused.
func TestExecuteRemove(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mod")
	ro := filepath.Join(dir, "golang.org", "x@v1")
	if err := os.MkdirAll(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ro, "go.mod"), nil, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	p := Plan{Title: "Clean up", Steps: []Step{{Remove: dir}}}
	journal := &strings.Builder{}
	got := Execute(t.Context(), &run.Fake{}, p, journal, func(State) {})
	if got.Phase != Succeeded {
		t.Fatalf("phase %d, err %q", got.Phase, got.Err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("%s is still there: %v", dir, err)
	}
	if !strings.Contains(journal.String(), "remove "+dir) {
		t.Errorf("journal %q", journal)
	}
	p.Steps[0].Remove = "relative/cache"
	if got := Execute(t.Context(), &run.Fake{}, p, journal, func(State) {}); got.Phase != Failed {
		t.Errorf("relative path: phase %d", got.Phase)
	}
}
