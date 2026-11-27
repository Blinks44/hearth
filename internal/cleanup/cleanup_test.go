package cleanup

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
)

var now = time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)

func copyOf(tool, version, manager string, kind inventory.Kind, root string, term inventory.PathRank) inventory.Installation {
	return inventory.Installation{Tool: tool, Version: version, Path: root + "/bin/" + tool, OnPath: term,
		Source: inventory.Source{Kind: kind, Manager: manager, Root: root, Version: version}}
}

func TestFind(t *testing.T) {
	const vm, pm, sys = inventory.KindVersionManager, inventory.KindPackageManager, inventory.KindSystem
	fnm22 := copyOf("node", "22.11.0", "fnm", vm, "/fnm/v22.11.0", 1)
	fnm22.Default = true
	brewNode := copyOf("node", "23.6.0", "brew", pm, "/brew/Cellar/node/23.6.0", 2)
	brewNPM := copyOf("npm", "10.9.0", "brew", pm, "/brew/Cellar/node/23.6.0", 0)
	mise := inventory.Installation{Tool: "python", Path: "/mise/shims/python", OnPath: 1, Source: inventory.Source{Kind: vm, Manager: "mise", Root: "/mise", Shim: true}}
	mise312 := copyOf("python", "3.12.7", "mise", vm, "/mise/installs/python/3.12.7", 0)
	mise312.Default = true
	brewPython := copyOf("python", "3.13.1", "brew", pm, "/brew/Cellar/python@3.13/3.13.1", 2)
	s := &inventory.Snapshot{Tools: []inventory.ToolState{
		{Tool: "node", Installations: []inventory.Installation{
			fnm22, brewNode,
			copyOf("node", "20.18.0", "fnm", vm, "/fnm/v20.18.0", 0),
			copyOf("node", "18.20.4", "fnm", vm, "/fnm/v18.20.4", 0),
			copyOf("node", "16.20.2", "nvm", vm, "/nvm/v16.20.2", 0),
			copyOf("node", "14.21.3", "nvm", vm, "/nvm/v14.21.3", 0),
		}},
		// npm in brew's Node folder goes with it, though not its own copy.
		{Tool: "npm", Installations: []inventory.Installation{copyOf("npm", "10.9.1", "fnm", vm, "/fnm/v22.11.0", 1), brewNPM}},
		{Tool: "python", Installations: []inventory.Installation{
			mise, brewPython, mise312,
			copyOf("python", "3.11.10", "mise", vm, "/mise/installs/python/3.11.10", 0),
			copyOf("python", "3.9.6", "", sys, "/usr", 3),
			copyOf("python", "3.13.1", "uv", vm, "/uv/python/cpython-3.13.1", 0),
		}},
		// mise's tools without a page: zig's default stays, its older
		// version goes.
		{Tool: "mise", MiseTools: []inventory.MiseTool{{Name: "zig", Installations: []inventory.Installation{
			func() inventory.Installation {
				in := copyOf("zig", "0.13.0", "mise", vm, "/mise/installs/zig/0.13.0", 0)
				in.Default = true
				return in
			}(),
			copyOf("zig", "0.12.0", "mise", vm, "/mise/installs/zig/0.12.0", 0),
		}}}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", ReasonsKnown: true, Packages: []inventory.SystemPackage{
			{Name: "node", Path: "/brew/Cellar/node/23.6.0", Dependencies: []string{"icu4c"}},
			{Name: "icu4c", Path: "/brew/Cellar/icu4c/76", Dependency: true},
			{Name: "oniguruma", Path: "/brew/Cellar/oniguruma/6.9", Dependency: true},
			{Name: "python@3.13", Path: "/brew/Cellar/python@3.13/3.13.1", Dependency: true},
		}}}},
	}}
	sizes := &inventory.Sizes{
		Paths: map[string]inventory.Usage{
			"/fnm/v20.18.0":                         {Bytes: 200e6},
			"/nvm/v16.20.2":                         {Bytes: 300e6},
			"/nvm/v14.21.3":                         {Bytes: 400e6},
			"/brew/Cellar/node/23.6.0":              {Bytes: 250e6},
			"/brew/Cellar/oniguruma/6.9":            {Bytes: 10e6},
			"/npm-cache":                            {Bytes: 5000e6},
			"/pnpm-store":                           {Bytes: 9000e6, Shared: 9000e6},
			"/yarn-cache":                           {Bytes: 16384},
			filepath.Join("/old", "node_modules"):   {Bytes: 900e6},
			filepath.Join("/h/try", "node_modules"): {Bytes: 500e6},
			filepath.Join("/h/app", "node_modules"): {Bytes: 700e6},
		},
		// Found under home: the known /old again, an unknown project not
		// changed for long, and one changed lately.
		Installs: []inventory.ProjectInstall{
			{Project: "/old", Name: "node_modules", Changed: now.AddDate(-1, 0, 0)},
			{Project: "/h/try", Name: "node_modules", Changed: now.AddDate(-2, 0, 0)},
			{Project: "/h/app", Name: "node_modules", Changed: now.AddDate(0, 0, -3)},
		},
		Caches: []inventory.Cache{{ID: "npm", Path: "/npm-cache"}, {ID: "pnpm", Path: "/pnpm-store"}, {ID: "yarn", Path: "/yarn-cache"}},
		Docker: []inventory.DockerUsage{
			{Type: "Images", Count: 16, Active: 14, Bytes: 11000e6, Reclaimable: 1600e6},
			{Type: "Containers", Count: 18, Active: 2, Bytes: 770e6, Reclaimable: 760e6},
			{Type: "Local Volumes", Count: 29, Active: 25, Bytes: 5300e6, Reclaimable: 400e6},
			// An image of a stopped container: reclaimable to Docker, yet
			// not without its container.
			{Type: "Images", Count: 1, Active: 1, Bytes: 400e6, Reclaimable: 400e6},
			{Type: "Build Cache", Count: 3, Bytes: 16384, Reclaimable: 16384},
		},
	}
	ps := []projects.Project{
		{Dir: "/legacy", Requirements: []projects.Requirement{{Tool: "node", Want: "18", File: ".nvmrc"}}, Changed: now.Add(-time.Hour)},
		{Dir: "/old", Installs: []string{"node_modules"}, Changed: now.AddDate(-1, 0, 0)},
		{Dir: "/fresh", Installs: []string{"node_modules", ".venv"}, Changed: now.AddDate(0, -1, 0)},
	}
	got := Find(s, sizes, ps, nil, now)
	var summary []string
	for _, c := range got {
		var reasons []string
		for _, r := range c.Reasons {
			reasons = append(reasons, string(r))
		}
		line := string(c.Kind) + " " + c.ID + " " + strings.Join(c.Paths, ",") + " " + strings.Join(reasons, ",")
		if c.Selected {
			line += " selected"
		}
		summary = append(summary, line)
	}
	want := []string{
		// Caches first, chosen at once; the pnpm store frees nothing, its
		// files are all in projects' node_modules too, and Yarn's next to
		// nothing.
		"cache npm /npm-cache restorable selected",
		// Copies: nvm is not set up, so its copies come as one leftover;
		// fnm's 18 is the project's and stays; brew's Node is shadowed,
		// with its npm. Brew's Python is shadowed by mise's shim, its
		// default; the system's Python and uv's, which venvs use by path,
		// stay.
		"leftover nvm /nvm/v16.20.2,/nvm/v14.21.3 not-set-up,no-project",
		"extra-copy node /brew/Cellar/node/23.6.0 shadowed,no-project",
		"extra-copy node /fnm/v20.18.0 not-default,not-on-path,no-project",
		"unneeded-package oniguruma /brew/Cellar/oniguruma/6.9 nothing-needs",
		"extra-copy python /brew/Cellar/python@3.13/3.13.1 shadowed,no-project",
		"extra-copy python /mise/installs/python/3.11.10 not-default,not-on-path,no-project",
		"extra-copy zig /mise/installs/zig/0.12.0 not-default,not-on-path,no-project",
		// Docker: no volumes, and nothing for a build cache this small.
		"docker-images   pulled",
		"docker-containers   stopped",
		// Projects not changed for long are chosen at once, one changed
		// lately is offered; the known project's comes once.
		"project-installs node_modules " + filepath.Join("/old", "node_modules") + " unchanged,reinstalled selected",
		"project-installs node_modules " + filepath.Join("/h/app", "node_modules") + " reinstalled",
		"project-installs node_modules " + filepath.Join("/h/try", "node_modules") + " unchanged,reinstalled selected",
	}
	if !slices.Equal(summary, want) {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(summary, "\n"), strings.Join(want, "\n"))
	}
	if c := got[1]; c.Usage.Bytes != 700e6 || !c.Known || len(c.Copies) != 2 {
		t.Errorf("leftover %+v", c)
	}
	if c := got[2]; len(c.Copies) != 2 {
		t.Errorf("brew's Node goes without its npm: %+v", c.Copies)
	}
	if c := got[6]; c.Known {
		t.Errorf("a size not counted is known: %+v", c)
	}
	if c := got[8]; c.Count != 2 || c.Frees() != 1600e6 {
		t.Errorf("images %+v", c)
	}
	if Find(nil, sizes, ps, nil, now) != nil {
		t.Error("candidates before a scan")
	}
}

// Models of a gigabyte or more are offered, not chosen at once, with what
// they free; not those in memory, those an MCP server of the user's or a
// project's names, nor small ones.
func TestModels(t *testing.T) {
	const gb = 1_000_000_000
	s := &inventory.Snapshot{
		Tools: []inventory.ToolState{{Tool: "models", Models: &inventory.ModelsState{
			Ollama: &inventory.ModelApp{Models: []inventory.Model{
				{Name: "llama3.2:3b", Path: "/o/3b", Size: 2 * gb, Frees: 2*gb - 5},
				{Name: "qwen2.5-coder:32b", Path: "/o/32b", Size: 19 * gb, Frees: 19 * gb, Loaded: true},
				{Name: "nomic-embed-text:latest", Path: "/o/nomic", Size: 3 * gb, Frees: 3 * gb},
				{Name: "mxbai-embed-large:latest", Path: "/o/mxbai", Size: 2 * gb, Frees: 2 * gb},
				{Name: "all-minilm:latest", Path: "/o/minilm", Size: gb / 20, Frees: gb / 20},
			}},
			LMStudio: &inventory.ModelApp{Models: []inventory.Model{{Name: "Mistral-Nemo-12B-GGUF", Path: "/l/mistral", Size: 7 * gb, Frees: 7 * gb}}},
		}}},
		MCP: []inventory.MCPServer{{Name: "memory", Agent: "claude", Args: []string{"-y", "mcp-memory", "--embed-model=nomic-embed-text"}}},
	}
	ps := []projects.Project{{Dir: "/p", MCP: []inventory.MCPServer{{Name: "rag", Args: []string{"--model", "ollama/mxbai-embed-large"}}}}}
	var got []string
	for _, c := range Find(s, nil, ps, nil, now) {
		if c.Kind != Model {
			continue
		}
		got = append(got, c.Manager+" "+c.ID)
		if c.Selected || !c.Known || c.Kind.Group() != Models {
			t.Errorf("%s: %+v", c.ID, c)
		}
		if c.ID == "llama3.2:3b" && (c.Frees() != 2*gb-5 || c.Paths[0] != "/o/3b") {
			t.Errorf("3b frees %d", c.Frees())
		}
	}
	if want := []string{"LM Studio Mistral-Nemo-12B-GGUF", "Ollama llama3.2:3b"}; !slices.Equal(got, want) {
		t.Errorf("models %q, want %q", got, want)
	}
}

func TestOldExtensions(t *testing.T) {
	old := []string{"/h/.vscode/extensions/go-0.45.0", "/h/.vscode/extensions/go-0.44.0"}
	s := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "editors", Editors: &inventory.EditorsState{Editors: []inventory.Editor{
		{Name: "VS Code", Superseded: old}, {Name: "Cursor"},
	}}}}}
	sizes := &inventory.Sizes{Paths: map[string]inventory.Usage{old[0]: {Bytes: 30}, old[1]: {Bytes: 20}}}
	var got []Candidate
	for _, c := range Find(s, sizes, nil, nil, now) {
		if c.Kind == OldExtensions {
			got = append(got, c)
		}
	}
	if len(got) != 1 || got[0].ID != "VS Code" || got[0].Count != 2 || got[0].Frees() != 50 || got[0].Selected || got[0].Kind.Group() != Copies {
		t.Fatalf("%+v", got)
	}
}

func TestHistory(t *testing.T) {
	old, recent := now.Add(-HistoryAfter-time.Hour), now.Add(-HistoryAfter+time.Hour)
	s := &inventory.Snapshot{Sessions: []inventory.AgentSession{
		{Agent: "claude", Files: []string{"/h/.claude/projects/p/a.jsonl", "/h/.claude/projects/p/a"}, Size: 3_000_000, Modified: old},
		{Agent: "claude", Files: []string{"/h/.claude/projects/p/b.jsonl"}, Size: 2_000_000, Modified: old},
		// Resumed lately: it stays.
		{Agent: "claude", Files: []string{"/h/.claude/projects/p/c.jsonl"}, Size: 9_000_000, Modified: recent},
		// Too little to be worth a look.
		{Agent: "codex", Files: []string{"/h/.codex/sessions/r.jsonl"}, Size: 500, Modified: old},
		// Shared SQLite DB session without standalone Files must never be offered.
		{Agent: "opencode", Files: nil, Size: 50_000_000, Modified: old},
	}}
	var got []Candidate
	for _, c := range Find(s, nil, nil, nil, now) {
		if c.Kind == AgentHistory {
			got = append(got, c)
		}
	}
	want := []string{"/h/.claude/projects/p/a.jsonl", "/h/.claude/projects/p/a", "/h/.claude/projects/p/b.jsonl"}
	if len(got) != 1 || got[0].ID != "claude" || got[0].Count != 2 || got[0].Frees() != 5_000_000 || got[0].Selected ||
		!slices.Equal(got[0].Paths, want) || got[0].Kind.Group() != History {
		t.Fatalf("%+v", got)
	}
}

func TestOrphanPackages(t *testing.T) {
	s := &inventory.Snapshot{Tools: []inventory.ToolState{{
		Tool: "packages",
		PackageManagers: []inventory.PackageManager{
			{ID: "apt", Name: "APT", Path: "/usr/bin/apt", Packages: []inventory.SystemPackage{
				{Name: "git", Size: 10_000_000},
				{Name: "libfoo1", Dependency: true, Orphan: true, Size: 4_000_000},
			}},
			{ID: "pacman", Name: "Pacman", Path: "/usr/bin/pacman", Packages: []inventory.SystemPackage{
				{Name: "pcre-old", Dependency: true, Orphan: true, Size: 2_000_000},
			}},
		},
	}}}
	var got []Candidate
	for _, c := range Find(s, nil, nil, nil, now) {
		if c.Kind == UnneededPackage {
			got = append(got, c)
		}
	}
	if len(got) != 2 || got[0].ID != "libfoo1" || got[0].Manager != "apt" || got[0].Frees() != 4_000_000 || got[0].Selected || !got[0].Known ||
		got[1].ID != "pcre-old" || got[1].Manager != "pacman" || got[1].Frees() != 2_000_000 || got[1].Selected {
		t.Fatalf("orphans = %+v", got)
	}
}

func TestMultiPathAndOptInCaches(t *testing.T) {
	s := &inventory.Snapshot{}
	sizes := &inventory.Sizes{
		Paths: map[string]inventory.Usage{
			"/h/.rustup/downloads": {Bytes: 300_000_000},
			"/h/.rustup/tmp":       {Bytes: 50_000_000},
			"/h/.codex/worktrees":  {Bytes: 400_000_000},
			"/h/.claude/debug":     {Bytes: 20_000_000},
			"/h/.claude/telemetry": {Bytes: 10_000_000},
		},
		Caches: []inventory.Cache{
			{ID: "rustup", Paths: []string{"/h/.rustup/downloads", "/h/.rustup/tmp"}},
			{ID: "codex-worktrees", Path: "/h/.codex/worktrees", OptIn: true},
			{ID: "claude-cache", Paths: []string{"/h/.claude/debug", "/h/.claude/telemetry"}},
		},
	}
	got := Find(s, sizes, nil, nil, now)
	byID := map[string]Candidate{}
	for _, c := range got {
		byID[c.ID] = c
	}
	rustup, ok := byID["rustup"]
	if !ok || !rustup.Selected || rustup.Frees() != 350_000_000 || len(rustup.Paths) != 2 {
		t.Fatalf("rustup = %+v (ok=%v)", rustup, ok)
	}
	wt, ok := byID["codex-worktrees"]
	if !ok || wt.Selected || wt.Frees() != 400_000_000 || !slices.Contains(wt.Reasons, AbandonedWorktrees) {
		t.Fatalf("codex-worktrees = %+v (ok=%v)", wt, ok)
	}
	claude, ok := byID["claude-cache"]
	if !ok || !claude.Selected || claude.Frees() != 30_000_000 || len(claude.Paths) != 2 {
		t.Fatalf("claude-cache = %+v (ok=%v)", claude, ok)
	}
}

func TestHistory30DaysAndAntigravity(t *testing.T) {
	if HistoryAfter != 30*24*time.Hour {
		t.Fatalf("HistoryAfter = %v, want 30d", HistoryAfter)
	}
	s := &inventory.Snapshot{Sessions: []inventory.AgentSession{
		{Agent: "antigravity", Files: []string{"/h/.gemini/antigravity-cli/conversations/old.db", "/h/.gemini/antigravity-cli/conversations/old.db-wal"}, Size: 4_000_000, Modified: now.Add(-45 * 24 * time.Hour)},
		{Agent: "antigravity", Files: []string{"/h/.gemini/antigravity-cli/conversations/new.db"}, Size: 8_000_000, Modified: now.Add(-15 * 24 * time.Hour)},
	}}
	var got []Candidate
	for _, c := range Find(s, nil, nil, nil, now) {
		if c.Kind == AgentHistory {
			got = append(got, c)
		}
	}
	wantFiles := []string{"/h/.gemini/antigravity-cli/conversations/old.db", "/h/.gemini/antigravity-cli/conversations/old.db-wal"}
	if len(got) != 1 || got[0].ID != "antigravity" || got[0].Selected || got[0].Count != 1 || got[0].Frees() != 4_000_000 || !slices.Equal(got[0].Paths, wantFiles) {
		t.Fatalf("antigravity history = %+v", got)
	}
}

func TestFindWithOptions(t *testing.T) {
	sizes := &inventory.Sizes{
		Paths: map[string]inventory.Usage{
			"/cache/npm":                            {Bytes: 20_000_000},
			"/cache/pip":                            {Bytes: 120_000_000},
			filepath.Join("/p/45d", "node_modules"): {Bytes: 80_000_000},
		},
		Installs: []inventory.ProjectInstall{
			{Project: "/p/45d", Name: "node_modules", Changed: now.Add(-45 * 24 * time.Hour)},
		},
		Caches: []inventory.Cache{
			{ID: "npm", Path: "/cache/npm"},
			{ID: "pip", Path: "/cache/pip"},
		},
	}
	s := &inventory.Snapshot{}
	ps := []projects.Project{{Dir: "/known/45d", Installs: []string{".venv"}, Changed: now.Add(-45 * 24 * time.Hour)}}
	sizes.Paths[filepath.Join("/known/45d", ".venv")] = inventory.Usage{Bytes: 90_000_000}

	// Default options (180d stale, 1 MB min): both caches appear, 45d home install is unselected, 45d known project is skipped.
	def := FindWithOptions(s, sizes, ps, nil, now, Options{})
	find := func(cands []Candidate, id, proj string) (Candidate, bool) {
		for _, c := range cands {
			if c.ID == id && c.Project == proj {
				return c, true
			}
		}
		return Candidate{}, false
	}
	if _, ok := find(def, "npm", ""); !ok {
		t.Fatal("default options should include 20 MB npm cache")
	}
	if _, ok := find(def, "pip", ""); !ok {
		t.Fatal("default options should include 120 MB pip cache")
	}
	if c, ok := find(def, "node_modules", "/p/45d"); !ok || c.Selected {
		t.Fatalf("default 45d home install = (%+v, %v), want present & unselected", c, ok)
	}
	if _, ok := find(def, ".venv", "/known/45d"); ok {
		t.Fatal("default options should not include 45d known project")
	}

	// Custom options (30d stale, 50 MB min): 45d projects are stale/selected, 20 MB npm cache is filtered out.
	custom := FindWithOptions(s, sizes, ps, nil, now, Options{
		StaleAfter: 30 * 24 * time.Hour,
		MinFrees:   50_000_000,
	})
	if _, ok := find(custom, "npm", ""); ok {
		t.Fatal("50 MB min should filter out 20 MB npm cache")
	}
	if _, ok := find(custom, "pip", ""); !ok {
		t.Fatal("50 MB min should keep 120 MB pip cache")
	}
	if c, ok := find(custom, "node_modules", "/p/45d"); !ok || !c.Selected {
		t.Fatalf("30d custom 45d home install = (%+v, %v), want present & selected", c, ok)
	}
	if c, ok := find(custom, ".venv", "/known/45d"); !ok || !c.Selected {
		t.Fatalf("30d custom 45d known project = (%+v, %v), want present & selected", c, ok)
	}
}

func TestProjectInstallsMinFreesAndRole(t *testing.T) {
	sizes := &inventory.Sizes{
		Paths: map[string]inventory.Usage{
			filepath.Join("/p/tiny", "node_modules"): {Bytes: 50_000},
			filepath.Join("/p/big", "node_modules"):  {Bytes: 5_000_000},
			filepath.Join("/p/big", ".next"):         {Bytes: 12_000_000},
		},
		Installs: []inventory.ProjectInstall{
			{Project: "/p/tiny", Name: "node_modules", Role: inventory.InstallRoleDeps, Changed: now.AddDate(-1, 0, 0)},
			{Project: "/p/big", Name: "node_modules", Role: inventory.InstallRoleDeps, Changed: now.AddDate(0, -1, 0)},
			{Project: "/p/big", Name: ".next", Role: inventory.InstallRoleBuild, Changed: now.AddDate(0, -1, 0)},
		},
	}
	got := Find(&inventory.Snapshot{}, sizes, nil, nil, now)
	byPath := map[string]Candidate{}
	for _, c := range got {
		if c.Kind == ProjectInstalls && len(c.Paths) > 0 {
			byPath[c.Paths[0]] = c
		}
	}
	if _, ok := byPath[filepath.Join("/p/tiny", "node_modules")]; ok {
		t.Error("measured ProjectInstalls below default 1 MB minFrees should be filtered out")
	}
	if c, ok := byPath[filepath.Join("/p/big", "node_modules")]; !ok || c.Role != inventory.InstallRoleDeps {
		t.Errorf("node_modules candidate = (%+v, %v), want role=deps", c, ok)
	}
	if c, ok := byPath[filepath.Join("/p/big", ".next")]; !ok || c.Role != inventory.InstallRoleBuild {
		t.Errorf(".next candidate = (%+v, %v), want role=build", c, ok)
	}
}

func TestProjectInstallsMonorepoRootGrouping(t *testing.T) {
	root := filepath.Join("/dev", "moduix")
	solid := filepath.Join(root, "packages", "solid")
	solidNM := filepath.Join(solid, "node_modules")
	solidDist := filepath.Join(solid, "dist")
	rootNM := filepath.Join(root, "node_modules")

	sizes := &inventory.Sizes{
		Paths: map[string]inventory.Usage{
			rootNM:    {Bytes: 500_000_000},
			solidNM:   {Bytes: 200_000_000},
			solidDist: {Bytes: 15_000_000},
		},
		Installs: []inventory.ProjectInstall{
			{Project: root, Name: "node_modules", Role: inventory.InstallRoleDeps, Changed: now.AddDate(0, 0, -5)},
			{Project: solid, Root: root, Name: "node_modules", Role: inventory.InstallRoleDeps, Changed: now.AddDate(0, 0, -5)},
			{Project: solid, Root: root, Name: "dist", Role: inventory.InstallRoleBuild, Changed: now.AddDate(0, 0, -5)},
		},
	}

	got := Find(&inventory.Snapshot{}, sizes, nil, nil, now)
	byPath := map[string]Candidate{}
	for _, c := range got {
		if c.Kind == ProjectInstalls && len(c.Paths) > 0 {
			byPath[c.Paths[0]] = c
		}
	}

	if c, ok := byPath[rootNM]; !ok || c.Project != root || c.ID != "node_modules" {
		t.Errorf("root node_modules = (%+v, %v), want Project=%q ID=node_modules", c, ok, root)
	}
	if c, ok := byPath[solidNM]; !ok || c.Project != root || c.ID != "packages/solid/node_modules" || !slices.Equal(c.Paths, []string{solidNM}) {
		t.Errorf("solid node_modules = (%+v, %v), want Project=%q ID=packages/solid/node_modules Path=%q", c, ok, root, solidNM)
	}
	if c, ok := byPath[solidDist]; !ok || c.Project != root || c.ID != "packages/solid/dist" || c.Role != inventory.InstallRoleBuild {
		t.Errorf("solid dist = (%+v, %v), want Project=%q ID=packages/solid/dist role=build", c, ok, root)
	}
}
