package projects

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	tests := []struct {
		dir     string
		want    []Requirement
		wantAI  []string
		wantErr string
		// wantFiles are the files found, when they are not the files of
		// the requirements.
		wantFiles []string
	}{
		{dir: "nvmrc", want: []Requirement{{Tool: "node", Want: "lts/iron", File: ".nvmrc"}}},
		{dir: "node-version", want: []Requirement{{Tool: "node", Want: "v20.11.0", File: ".node-version"}}},
		{dir: "package-json", want: []Requirement{
			{Tool: "node", Want: ">=18 <21", File: "package.json", Key: "engines.node"},
			{Tool: "pnpm", Want: "^9", File: "package.json", Key: "engines.pnpm"},
			{Tool: "node", Want: "20.11.0", File: "package.json", Key: "volta.node"},
			{Tool: "pnpm", Want: "9.12.0", File: "package.json", Key: "packageManager"},
			{Tool: "node", Want: "^20.10", File: "package.json", Key: "devEngines.runtime"},
			{Tool: "pnpm", Want: "9.x", File: "package.json", Key: "devEngines.packageManager"},
		}},
		{dir: "package-json-plain", wantFiles: []string{"package.json"}}, // engines of other tools only
		{dir: "lockfiles", want: []Requirement{
			{Tool: "npm", File: "package-lock.json"},
			{Tool: "pnpm", File: "pnpm-lock.yaml"},
			{Tool: "yarn", File: "yarn.lock"},
			{Tool: "bun", File: "bun.lock"},
			{Tool: "bun", File: "bun.lockb"},
		}},
		{dir: "tool-versions", want: []Requirement{
			{Tool: "node", Want: "22.11.0", File: ".tool-versions", Key: "nodejs"},
			{Tool: "python", Want: "3.12.4", File: ".tool-versions", Key: "python"},
			{Tool: "terraform", Want: "1.9.0", File: ".tool-versions", Key: "terraform"},
			{Tool: "go", Want: "1.23.2", File: ".tool-versions", Key: "golang"},
		}},
		{dir: "mise-toml", want: []Requirement{
			{Tool: "python", Want: "3.12", File: "mise.toml", Key: "tools.python"},
			{Tool: "node", Want: "22", File: "mise.toml", Key: "tools.node"},
			{Tool: "terraform", Want: "1.9", File: "mise.toml", Key: "tools.terraform"},
			{Tool: "rust", Want: "1.80", File: "mise.toml", Key: "tools.rust"},
		}},
		{dir: "dot-mise-toml", want: []Requirement{{Tool: "bun", Want: "latest", File: ".mise.toml", Key: "tools.bun"}}},
		{dir: "python-version", want: []Requirement{{Tool: "python", Want: "3.12", File: ".python-version"}}},
		{dir: "pyproject", want: []Requirement{
			{Tool: "python", Want: ">=3.12", File: "pyproject.toml", Key: "project.requires-python"},
			{Tool: "uv", File: "uv.lock"},
		}},
		{dir: "rust-toolchain-toml", want: []Requirement{{Tool: "rust", Want: "1.80.0", File: "rust-toolchain.toml", Key: "toolchain.channel"}}},
		{dir: "rust-toolchain", want: []Requirement{{Tool: "rust", Want: "nightly-2024-08-01", File: "rust-toolchain"}}},
		{dir: "cargo", want: []Requirement{{Tool: "rust", Want: "1.74", File: "Cargo.toml", Key: "package.rust-version"}}},
		{dir: "go-mod", want: []Requirement{
			{Tool: "go", Want: "1.23.2", File: "go.mod", Key: "go"},
			{Tool: "go", Want: "1.24.0", File: "go.mod", Key: "toolchain"},
		}},
		{dir: "folder-named-file", wantFiles: []string{}}, // .nvmrc is a folder
		{dir: "composer", want: []Requirement{
			{Tool: "php", Want: "^8.2", File: "composer.json", Key: "require.php"},
			{Tool: "composer", File: "composer.json"},
		}},
		{dir: "bun", want: []Requirement{
			{Tool: "bun", Want: "1.1.30", File: ".bun-version"},
			{Tool: "bun", File: "bunfig.toml"},
		}},
		{dir: "docker", want: []Requirement{
			{Tool: "docker", File: "compose.yaml"},
			{Tool: "docker", File: "Dockerfile"},
		}},
		{dir: "docker-compose", want: []Requirement{{Tool: "docker", File: "docker-compose.yml"}}},
		{dir: "ai", wantAI: []string{"AGENTS.md", "CLAUDE.md", ".claude", ".cursor", ".mcp.json"}},
		{
			dir:       "broken", // the other files still count
			want:      []Requirement{{Tool: "node", Want: "22", File: ".nvmrc"}},
			wantErr:   "read package.json",
			wantFiles: []string{".nvmrc", "package.json"},
		},
		{dir: "empty"},
		{dir: "missing", wantErr: "missing"},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			dir := filepath.Join("testdata", tt.dir)
			p := Read(dir)
			if p.Dir != dir {
				t.Errorf("Dir %q, want %q", p.Dir, dir)
			}
			if !reflect.DeepEqual(p.Requirements, tt.want) {
				t.Errorf("requirements\n got %+v\nwant %+v", p.Requirements, tt.want)
			}
			files := tt.wantFiles
			for _, r := range p.Requirements {
				if tt.wantFiles == nil && !slices.Contains(files, r.File) {
					files = append(files, r.File)
				}
			}
			if !slices.Equal(p.Files, files) {
				t.Errorf("files %q, want %q", p.Files, files)
			}
			if !reflect.DeepEqual(p.AI, tt.wantAI) {
				t.Errorf("AI %q, want %q", p.AI, tt.wantAI)
			}
			switch {
			case tt.wantErr == "" && len(p.Errs) > 0:
				t.Errorf("errors %v", p.Errs)
			case tt.wantErr != "" && (len(p.Errs) != 1 || !strings.Contains(p.Errs[0].Error(), tt.wantErr)):
				t.Errorf("errors %v, want one with %q", p.Errs, tt.wantErr)
			}
		})
	}
}

func TestFind(t *testing.T) {
	dir := filepath.Join("testdata", "monorepo")
	root, nested := Find(dir)
	if want := []string{".nvmrc", "package.json"}; !reflect.DeepEqual(root.Files, want) {
		t.Errorf("root files %q, want %q", root.Files, want)
	}
	var got []string
	for _, p := range nested {
		rel, _ := filepath.Rel(dir, p.Dir)
		got = append(got, filepath.ToSlash(rel))
	}
	// Not node_modules, hidden folders, or deeper than three levels.
	if want := []string{"apps/docs", "apps/web", "packages/ui"}; !reflect.DeepEqual(got, want) {
		t.Errorf("nested %q, want %q", got, want)
	}
	if v := nested[1].Versions(); len(v) != 1 || v[0].Want != "20" {
		t.Errorf("apps/web versions %+v, want .nvmrc 20", v)
	}
	if v := nested[0].Versions(); len(v) != 0 {
		t.Errorf("apps/docs versions %+v, want none", v)
	}
}
