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
	"github.com/Blinks44/hearth/locales"
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
// projects' installs and extensions' old versions as their folders.
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
		{Kind: cleanup.OldExtensions, ID: "VS Code", Paths: []string{"/e/go-0.1", "/e/go-0.2"}, Usage: gb},
	}
	titles := []string{"pnpm cache", "npm cache", "Cargo cache", "pub cache", "Build Cache", "Images", "Containers", "Node", "old: node_modules", "VS Code"}
	env := pathWith(t, "pnpm", "flutter")
	p, err := CleanupPlan(cands, titles, nil, env, DockerTarget{CLI: "/usr/local/bin/docker", Context: "orbstack"}, nil)
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
		"remove /p/node_modules",
		"remove /e/go-0.1",
		"remove /e/go-0.2",
	}
	if !slices.Equal(got, want) {
		t.Errorf("steps\n%q\nwant\n%q", got, want)
	}
	if len(p.Changes) != 0 {
		t.Errorf("changes %q", p.Changes)
	}
	if len(p.Notes) != 4 {
		t.Errorf("notes %+v, want the folders', pnpm's, pub's and the projects'", p.Notes)
	}
	// Without an engine Docker's are left out; with nothing left, no plan.
	p, err = CleanupPlan(cands[4:5], titles[4:5], nil, env, DockerTarget{}, nil)
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
	p, err := CleanupPlan(cands, []string{"a", "b", "c"}, nil, nil, DockerTarget{CLI: "docker", Context: "default"}, docker)
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

// A copy goes through its version manager, or as its folder when the
// terminal does not find the manager; a leftover's versions one by root;
// Homebrew's copies and packages through one brew uninstall, but not a
// formula with another version installed. What no source removes is
// left out.
func TestCleanupCopies(t *testing.T) {
	vm := func(tool, manager, root, version string) inventory.Installation {
		return inventory.Installation{Tool: tool, Version: version, Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: manager, Root: root, Version: version}}
	}
	keg := func(tool, root string) inventory.Installation {
		return inventory.Installation{Tool: tool, Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew", Root: root}}
	}
	gb := inventory.Usage{Bytes: 1e9}
	s := &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "node", Managers: []inventory.Manager{{Name: "fnm"}, {Name: "n", Admin: true}}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew", Packages: []inventory.SystemPackage{
			{Name: "python@3.12", Kind: "formula", Path: "/opt/homebrew/Cellar/python@3.12/3.12.8"},
			{Name: "libyaml", Kind: "formula", Path: "/opt/homebrew/Cellar/libyaml/0.2.5", Dependency: true},
			{Name: "node", Kind: "formula", Path: "/opt/homebrew/Cellar/node/23.3.0"},
			{Name: "node", Kind: "formula", Path: "/opt/homebrew/Cellar/node/24.1.0"},
		}}}},
	}}
	nvm1, nvm2 := "/h/.nvm/versions/node/v18.20.4", "/h/.nvm/versions/node/v20.18.0"
	cands := []cleanup.Candidate{
		{Kind: cleanup.ExtraCopy, ID: "node", Paths: []string{"/fnm/v20.1.0"}, Copies: []inventory.Installation{vm("node", "fnm", "/fnm/v20.1.0", "20.1.0")}, Usage: gb},
		{Kind: cleanup.ExtraCopy, ID: "python", Paths: []string{"/h/.pyenv/versions/3.11.9"}, Copies: []inventory.Installation{vm("python", "pyenv", "/h/.pyenv/versions/3.11.9", "3.11.9")}, Usage: gb},
		{Kind: cleanup.ExtraCopy, ID: "node", Paths: []string{"/n/versions/node/16.0.0"}, Copies: []inventory.Installation{vm("node", "n", "/n/versions/node/16.0.0", "16.0.0")}, Usage: gb},
		{Kind: cleanup.Leftover, ID: "nvm", Paths: []string{nvm1, nvm2}, Copies: []inventory.Installation{
			vm("node", "nvm", nvm1, "18.20.4"), vm("npm", "nvm", nvm1, "10.8.2"), vm("node", "nvm", nvm2, "20.18.0"),
		}, Usage: gb},
		{Kind: cleanup.ExtraCopy, ID: "python", Paths: []string{"/opt/homebrew/Cellar/python@3.12/3.12.8"}, Copies: []inventory.Installation{keg("python", "/opt/homebrew/Cellar/python@3.12/3.12.8")}, Usage: gb},
		{Kind: cleanup.UnneededPackage, ID: "libyaml", Manager: "brew", Paths: []string{"/opt/homebrew/Cellar/libyaml/0.2.5"}, Usage: gb},
		{Kind: cleanup.ExtraCopy, ID: "node", Paths: []string{"/opt/homebrew/Cellar/node/23.3.0"}, Copies: []inventory.Installation{keg("node", "/opt/homebrew/Cellar/node/23.3.0")}, Usage: gb},
		{Kind: cleanup.ExtraCopy, ID: "node", Paths: []string{"/h/.volta/tools/image/node/22.11.0"}, Copies: []inventory.Installation{vm("node", "volta", "/h/.volta/tools/image/node/22.11.0", "22.11.0")}, Usage: gb},
		{Kind: cleanup.ExtraCopy, ID: "rust", Paths: []string{"/h/.rustup/toolchains/nightly"}, Copies: []inventory.Installation{vm("rust", "rustup", "/h/.rustup/toolchains/nightly", "nightly")}, Usage: gb},
	}
	titles := []string{"Node 20.1.0 · fnm", "Python 3.11.9 · pyenv", "Node 16.0.0 · n", "nvm", "Python · brew", "libyaml · brew", "Node 23.3.0 · brew", "Node · Volta", "Rust · rustup"}
	for i, c := range cands {
		if want := i < 6; Cleans(c, s) != want {
			t.Errorf("%s: Cleans %v, want %v", titles[i], !want, want)
		}
	}
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".nvm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".nvm", "nvm.sh"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	env := append(pathWith(t, "fnm", "n"), "HOME="+home)
	p, err := CleanupPlan(cands, titles, s, env, DockerTarget{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, st := range p.Steps {
		switch {
		case st.Remove != "":
			got = append(got, "remove "+st.Remove)
		case st.Admin:
			got = append(got, "admin "+Command(st.Shown()))
		default:
			got = append(got, Command(st.Shown()))
		}
	}
	want := []string{
		"/opt/homebrew/bin/brew uninstall --formula python@3.12 libyaml",
		"fnm uninstall 20.1.0",
		"remove /h/.pyenv/versions/3.11.9",
		"admin n rm 16.0.0",
		"nvm uninstall 18.20.4",
		"nvm uninstall 20.18.0",
	}
	if !slices.Equal(got, want) {
		t.Errorf("steps\n%q\nwant\n%q", got, want)
	}
	if len(p.Changes) != 0 {
		t.Errorf("changes %q", p.Changes)
	}
	var notes []string
	for _, n := range p.Notes {
		notes = append(notes, n.Title)
	}
	if len(notes) != 4 {
		t.Errorf("notes %q, want brew's, the folders', the versions' and the administrator's", notes)
	}
	// nvm removed leaves its versions: they go as folders.
	p, err = CleanupPlan(cands[3:4], titles[3:4], s, env[:1], DockerTarget{}, nil)
	if err != nil || len(p.Steps) != 2 || p.Steps[0].Remove != nvm1 || p.Steps[1].Remove != nvm2 {
		t.Errorf("without nvm.sh: %+v, %v", p.Steps, err)
	}
}

// A copy's check counts all its folders: those gone free what they took.
func TestCleanupFreedCopies(t *testing.T) {
	root := t.TempDir()
	gone, kept := filepath.Join(root, "gone"), filepath.Join(root, "kept")
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kept, "f"), make([]byte, 10_000), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := disk.Measure(t.Context(), kept)
	if err != nil {
		t.Fatal(err)
	}
	c := cleanup.Candidate{Kind: cleanup.Leftover, Paths: []string{gone, kept}, Usage: inventory.Usage{Bytes: 2e9 + after.Bytes}}
	if got := freed(t.Context(), []cleanup.Candidate{c}, nil); got != 2e9 {
		t.Errorf("freed %d, want 2e9", got)
	}
}

// Ollama's models go through ollama rm when the terminal has it, LM
// Studio's as their folders. Ollama's frees all it took once ollama rm
// succeeded, LM Studio's once its folder is gone.
func TestCleanupModels(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "qwen")
	cands := []cleanup.Candidate{
		{Kind: cleanup.Model, ID: "llama3.2:latest", Manager: "Ollama", Paths: []string{"/o/manifests/llama3.2/latest"}, Usage: inventory.Usage{Bytes: 2e9}},
		{Kind: cleanup.Model, ID: "qwen", Manager: "LM Studio", Paths: []string{folder}, Usage: inventory.Usage{Bytes: 3e9}},
	}
	withOllama := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "models", Models: &inventory.ModelsState{Ollama: &inventory.ModelApp{Path: "/usr/local/bin/ollama"}}}}}
	if Cleans(cands[0], nil) || !Cleans(cands[0], withOllama) || !Cleans(cands[1], nil) {
		t.Errorf("cleans: Ollama's only with ollama, LM Studio's always")
	}
	p, err := CleanupPlan(cands, []string{"llama3.2", "qwen"}, withOllama, nil, DockerTarget{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 2 || Command(p.Steps[0].Argv) != "ollama rm llama3.2:latest" || p.Steps[1].Remove != folder {
		t.Errorf("steps %+v", p.Steps)
	}
	if got := freed(t.Context(), cands, nil); got != 5e9 {
		t.Errorf("freed %d, want 5e9", got)
	}
}

// An agent's old sessions go as their files and folders, with a note
// that they leave Usage and cannot be resumed.
func TestCleanupPlanHistory(t *testing.T) {
	c := cleanup.Candidate{Kind: cleanup.AgentHistory, ID: "claude", Count: 2, Usage: inventory.Usage{Bytes: 5e6},
		Paths: []string{"/h/.claude/projects/p/a.jsonl", "/h/.claude/projects/p/a", "/h/.claude/projects/p/b.jsonl"}}
	p, err := CleanupPlan([]cleanup.Candidate{c}, []string{"Claude Code: old sessions"}, nil, nil, DockerTarget{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var removed []string
	for _, s := range p.Steps {
		removed = append(removed, s.Remove)
	}
	if !slices.Equal(removed, c.Paths) {
		t.Errorf("removes %q", removed)
	}
	if !slices.ContainsFunc(p.Notes, func(n Note) bool { return n.Title == locales.CleanupHistoryNote() }) {
		t.Errorf("notes %+v", p.Notes)
	}
}

// A successful prune can keep the whole store. Its outcome must not claim
// that the estimated candidate size was actually removed.
func TestCleanupPruneKeepsUsedPackages(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "used"), make([]byte, 10000), 0o600); err != nil {
		t.Fatal(err)
	}
	usage, err := disk.Measure(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	c := cleanup.Candidate{Kind: cleanup.Cache, ID: "pnpm", Paths: []string{dir}, Usage: usage}
	p, err := CleanupPlan([]cleanup.Candidate{c}, []string{"pnpm cache"}, nil, pathWith(t, "pnpm"), DockerTarget{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := p.Verify(t.Context()); err != nil || got != locales.CleanupFreedNone() {
		t.Fatalf("verify %q, %v", got, err)
	}
	if len(p.Changes) != 0 {
		t.Fatalf("estimated removals presented as actual: %v", p.Changes)
	}
	if !slices.ContainsFunc(p.Notes, func(n Note) bool { return n.Title == locales.CleanupPnpmNote() }) {
		t.Fatal("missing prune explanation")
	}
}
