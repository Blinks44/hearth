package projects

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// monorepoMaxUp limits how many parent folders Read walks when looking for
// inherited version configs outside a git repository.
const monorepoMaxUp = 6

// inheritedVersionFiles are version-manager pin files whose lookup walks up
// the directory tree in monorepos (global.json is handled separately by
// readInheritedGlobalJSON).
var inheritedVersionFiles = []string{
	".nvmrc",
	".node-version",
	".tool-versions",
	"mise.toml",
	".mise.toml",
	".python-version",
	".ruby-version",
	".java-version",
	".sdkmanrc",
	".go-version",
	".php-version",
	".bun-version",
	"rust-toolchain.toml",
	"rust-toolchain",
	".terraform-version",
	".opentofu-version",
	".tofu-version",
}

// readInheritedVersionFiles looks upward in parent directories for version
// config files of tools the project uses that are not already pinned locally.
// It never turns an empty folder into a project, stops at the repository root
// (.git), and never reads global user configs in $HOME.
func readInheritedVersionFiles(p *Project) {
	if len(p.Files) == 0 || hasGitMarker(p.Dir) {
		return
	}
	relevant := projectTools(p)
	if len(relevant) == 0 {
		return
	}
	pinned := localPinnedTools(p)
	// If every relevant tool already has a local version pin, nothing to inherit.
	needAny := false
	for tool := range relevant {
		if !pinned[tool] {
			needAny = true
			break
		}
	}
	if !needAny {
		return
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		home = filepath.Clean(home)
	}
	dir := filepath.Dir(p.Dir)
	for range monorepoMaxUp {
		clean := filepath.Clean(dir)
		if clean == filepath.Clean(p.Dir) || (home != "" && clean == home) {
			return
		}
		foundHere := map[string]bool{}
		for _, r := range readers {
			if !slices.Contains(inheritedVersionFiles, r.file) {
				continue
			}
			path := filepath.Join(dir, r.file)
			data, err := os.ReadFile(path)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if info, serr := os.Stat(path); err != nil && serr == nil && info.IsDir() {
				continue
			}
			if err != nil {
				continue
			}
			reqs, parseErr := r.read(data)
			if parseErr != nil {
				continue
			}
			var matching []Requirement
			for _, req := range reqs {
				if req.Want == "" || !relevant[req.Tool] || pinned[req.Tool] {
					continue
				}
				matching = append(matching, req)
			}
			if len(matching) == 0 {
				continue
			}
			rel, relErr := filepath.Rel(p.Dir, path)
			if relErr != nil {
				continue
			}
			if !slices.Contains(p.Files, rel) {
				p.Files = append(p.Files, rel)
			}
			for _, req := range matching {
				req.File = rel
				p.Requirements = append(p.Requirements, req)
				foundHere[req.Tool] = true
			}
		}
		for tool := range foundHere {
			pinned[tool] = true
		}
		if hasGitMarker(dir) {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func hasGitMarker(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// localPinnedTools returns the tools that already have a local version-pinning
// file in dir (not a manifest like package.json engines or go.mod).
func localPinnedTools(p *Project) map[string]bool {
	out := map[string]bool{}
	for _, r := range p.Requirements {
		if !r.Inherited() && r.Want != "" && (slices.Contains(inheritedVersionFiles, r.File) || r.File == "global.json" || r.SDK != nil) {
			out[r.Tool] = true
		}
	}
	return out
}

// projectTools infers the tools relevant to p from its requirements and local
// manifests so a Go service in a polyglot monorepo does not inherit Node pins.
func projectTools(p *Project) map[string]bool {
	out := map[string]bool{}
	for _, r := range p.Requirements {
		out[r.Tool] = true
	}
	for _, f := range p.Files {
		switch f {
		case "package.json":
			for _, t := range []string{"node", "npm", "pnpm", "yarn", "bun"} {
				out[t] = true
			}
		case "pyproject.toml", "Pipfile", "poetry.lock", "uv.lock":
			out["python"] = true
			out["uv"] = true
		case "go.mod":
			out["go"] = true
		case "Cargo.toml":
			out["rust"] = true
		case "Gemfile":
			out["ruby"] = true
		case "composer.json":
			out["php"] = true
		case "pom.xml", "build.gradle", "build.gradle.kts":
			out["java"] = true
		case "deno.json", "deno.jsonc":
			out["deno"] = true
		default:
			switch strings.ToLower(filepath.Ext(f)) {
			case ".csproj":
				out["dotnet"] = true
			case ".tf":
				out["terraform"] = true
				out["tofu"] = true
			case ".tofu":
				out["tofu"] = true
			}
		}
	}
	return out
}
