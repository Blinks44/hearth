// Package projects reads what a project folder asks of the computer: tool
// versions from its files, and its AI settings; and checks those against
// what a scan found.
package projects

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Requirement is what one project file asks of a tool.
type Requirement struct {
	// Tool is the tool's ID, as "node", "pnpm", "python", "docker".
	Tool string
	// Want is the version as the file writes it: "22", "lts/*", "^20.10";
	// empty when the file only tells the tool is needed, as a lockfile.
	Want string
	// File is the file's name in the project folder, Key the field in it
	// when it has several, as "engines.node".
	File, Key string
}

// Project is what Read found in a project folder.
type Project struct {
	Dir string
	// Requirements come in the order of the files, as reader lists them.
	Requirements []Requirement
	// AI are the AI agents' files and folders the project has, as
	// "CLAUDE.md", ".claude".
	AI []string
	// Errs say which files could not be read; the others still count.
	Errs []error
}

// aiFiles are the AI settings a project may have, in the order shown.
var aiFiles = []string{"AGENTS.md", "CLAUDE.md", ".agents", ".claude", ".cursor", ".mcp.json"}

// readers read the project files Hearth knows, in the order shown. Each
// gets the file's contents and returns its requirements.
var readers = []struct {
	file string
	read func(data []byte) ([]Requirement, error)
}{
	{".nvmrc", firstLine("node")},
	{".node-version", firstLine("node")},
	{"package.json", readPackageJSON},
	{"package-lock.json", needs("npm")},
	{"pnpm-lock.yaml", needs("pnpm")},
	{"yarn.lock", needs("yarn")},
	{"bun.lock", needs("bun")},
	{"bun.lockb", needs("bun")},
	{".tool-versions", readToolVersions},
	{"mise.toml", readMiseTOML},
	{".mise.toml", readMiseTOML},
	{".python-version", firstLine("python")},
	{"pyproject.toml", readPyproject},
	{"uv.lock", needs("uv")},
	{"rust-toolchain.toml", readRustToolchain},
	{"rust-toolchain", firstLine("rust")},
	{"Cargo.toml", readCargoTOML},
	{"composer.json", readComposerJSON},
	{".bun-version", firstLine("bun")},
	{"bunfig.toml", needs("bun")},
	{"compose.yaml", needs("docker")},
	{"compose.yml", needs("docker")},
	{"docker-compose.yml", needs("docker")},
	{"docker-compose.yaml", needs("docker")},
	{"Dockerfile", needs("docker")},
}

// Read reads the project files in dir. A file that is missing is skipped;
// one that cannot be read or parsed goes to Errs.
func Read(dir string) Project {
	p := Project{Dir: dir}
	for _, r := range readers {
		data, err := os.ReadFile(filepath.Join(dir, r.file))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		var reqs []Requirement
		if err == nil {
			reqs, err = r.read(data)
		}
		if err != nil {
			p.Errs = append(p.Errs, fmt.Errorf("read %s: %w", r.file, err))
			continue
		}
		for _, req := range reqs {
			req.File = r.file
			p.Requirements = append(p.Requirements, req)
		}
	}
	for _, name := range aiFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			p.AI = append(p.AI, name)
		}
	}
	return p
}

// needs is a file that only tells the tool is needed.
func needs(tool string) func([]byte) ([]Requirement, error) {
	return func([]byte) ([]Requirement, error) {
		return []Requirement{{Tool: tool}}, nil
	}
}

// firstLine is a file that holds the version alone, as .nvmrc: its first
// line that is not empty or a comment.
func firstLine(tool string) func([]byte) ([]Requirement, error) {
	return func(data []byte) ([]Requirement, error) {
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			line, _, _ := strings.Cut(sc.Text(), "#")
			if line = strings.TrimSpace(line); line != "" {
				return []Requirement{{Tool: tool, Want: line}}, nil
			}
		}
		return []Requirement{{Tool: tool}}, sc.Err()
	}
}

// packageTools are the tools package.json's engines and volta may name.
var packageTools = []string{"node", "npm", "pnpm", "yarn", "bun"}

func readPackageJSON(data []byte) ([]Requirement, error) {
	var pkg struct {
		Engines        map[string]any `json:"engines"`
		Volta          map[string]any `json:"volta"`
		PackageManager string         `json:"packageManager"`
		DevEngines     struct {
			Runtime        json.RawMessage `json:"runtime"`
			PackageManager json.RawMessage `json:"packageManager"`
		} `json:"devEngines"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	var reqs []Requirement
	for _, field := range []struct {
		key string
		m   map[string]any
	}{{"engines", pkg.Engines}, {"volta", pkg.Volta}} {
		for _, tool := range packageTools {
			if want, ok := field.m[tool].(string); ok {
				reqs = append(reqs, Requirement{Tool: tool, Want: strings.TrimSpace(want), Key: field.key + "." + tool})
			}
		}
	}
	// "pnpm@9.12.0+sha512.abc": the version, without the hash.
	if name, version, ok := strings.Cut(pkg.PackageManager, "@"); ok {
		version, _, _ = strings.Cut(version, "+")
		reqs = append(reqs, Requirement{Tool: name, Want: version, Key: "packageManager"})
	}
	for _, field := range []struct {
		key string
		raw json.RawMessage
	}{{"devEngines.runtime", pkg.DevEngines.Runtime}, {"devEngines.packageManager", pkg.DevEngines.PackageManager}} {
		engines, err := devEngines(field.raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", field.key, err)
		}
		for _, e := range engines {
			reqs = append(reqs, Requirement{Tool: e.Name, Want: e.Version, Key: field.key})
		}
	}
	return reqs, nil
}

type devEngine struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// devEngines reads a field of devEngines: one engine or a list of them.
func devEngines(raw json.RawMessage) ([]devEngine, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil
	}
	var list []devEngine
	if raw[0] != '[' {
		raw = append(append([]byte{'['}, raw...), ']')
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// toolNames maps the names asdf plugins and mise give tools to Hearth's
// IDs. A tool not here is not one Hearth knows, and is skipped.
var toolNames = map[string]string{
	"node": "node", "nodejs": "node",
	"npm": "npm", "pnpm": "pnpm", "yarn": "yarn", "bun": "bun", "deno": "deno",
	"python": "python", "uv": "uv",
	"rust": "rust",
	"go":   "go", "golang": "go",
	"java":   "java",
	"ruby":   "ruby",
	"php":    "php",
	"dotnet": "dotnet", "dotnet-core": "dotnet",
}

// readToolVersions reads asdf's .tool-versions: "nodejs 22.11.0", and with
// several versions the first one is used.
func readToolVersions(data []byte) ([]Requirement, error) {
	var reqs []Requirement
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		tool, ok := toolNames[fields[0]]
		if !ok {
			continue
		}
		req := Requirement{Tool: tool, Key: fields[0]}
		if len(fields) > 1 {
			req.Want = fields[1]
		}
		reqs = append(reqs, req)
	}
	return reqs, sc.Err()
}

// readMiseTOML reads the [tools] of mise.toml: node = "22",
// python = ["3.12", "3.11"] or node = { version = "22" }.
func readMiseTOML(data []byte) ([]Requirement, error) {
	var cfg struct {
		Tools map[string]any `toml:"tools"`
	}
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, err
	}
	var reqs []Requirement
	// The keys keep the file's order, the map does not.
	for _, key := range md.Keys() {
		if len(key) != 2 || key[0] != "tools" {
			continue
		}
		name := key[1]
		tool, ok := toolNames[name]
		if !ok {
			continue
		}
		req := Requirement{Tool: tool, Key: "tools." + name}
		switch v := cfg.Tools[name].(type) {
		case string:
			req.Want = v
		case []any:
			if len(v) > 0 {
				req.Want, _ = v[0].(string)
			}
		case map[string]any:
			req.Want, _ = v["version"].(string)
		}
		reqs = append(reqs, req)
	}
	return reqs, nil
}

func readPyproject(data []byte) ([]Requirement, error) {
	var cfg struct {
		Project struct {
			RequiresPython string `toml:"requires-python"`
		} `toml:"project"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return []Requirement{{Tool: "python", Want: cfg.Project.RequiresPython, Key: "project.requires-python"}}, nil
}

func readRustToolchain(data []byte) ([]Requirement, error) {
	var cfg struct {
		Toolchain struct {
			Channel string `toml:"channel"`
		} `toml:"toolchain"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return []Requirement{{Tool: "rust", Want: cfg.Toolchain.Channel, Key: "toolchain.channel"}}, nil
}

func readCargoTOML(data []byte) ([]Requirement, error) {
	var cfg struct {
		Package struct {
			RustVersion string `toml:"rust-version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return []Requirement{{Tool: "rust", Want: cfg.Package.RustVersion, Key: "package.rust-version"}}, nil
}

func readComposerJSON(data []byte) ([]Requirement, error) {
	var cfg struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return []Requirement{
		{Tool: "php", Want: cfg.Require["php"], Key: "require.php"},
		{Tool: "composer"},
	}, nil
}
