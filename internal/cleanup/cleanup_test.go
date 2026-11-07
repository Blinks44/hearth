package cleanup

import (
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
			"/fnm/v20.18.0":              {Bytes: 200e6},
			"/nvm/v16.20.2":              {Bytes: 300e6},
			"/nvm/v14.21.3":              {Bytes: 400e6},
			"/brew/Cellar/node/23.6.0":   {Bytes: 250e6},
			"/brew/Cellar/oniguruma/6.9": {Bytes: 10e6},
			"/npm-cache":                 {Bytes: 5000e6},
			"/pnpm-store":                {Bytes: 9000e6, Shared: 9000e6},
			"/yarn-cache":                {Bytes: 16384},
			"/old/node_modules":          {Bytes: 900e6},
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
		"project-installs node_modules /old/node_modules unchanged,reinstalled",
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
