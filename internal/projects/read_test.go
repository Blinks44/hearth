package projects

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	tests := []struct {
		dir     string
		want    []Requirement
		wantAI  []string
		wantErr string
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
		{dir: "package-json-plain"}, // engines of other tools only
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
			{Tool: "go", Want: "1.23.2", File: ".tool-versions", Key: "golang"},
		}},
		{dir: "mise-toml", want: []Requirement{
			{Tool: "python", Want: "3.12", File: "mise.toml", Key: "tools.python"},
			{Tool: "node", Want: "22", File: "mise.toml", Key: "tools.node"},
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
			dir:     "broken", // the other files still count
			want:    []Requirement{{Tool: "node", Want: "22", File: ".nvmrc"}},
			wantErr: "read package.json",
		},
		{dir: "empty"},
		{dir: "missing"},
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
