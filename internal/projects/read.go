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
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Requirement is what one project file asks of a tool.
type Requirement struct {
	// Tool is the tool's ID, as "node", "pnpm", "python", "docker".
	Tool string
	// SDK is the selection policy of global.json, when present.
	SDK *SDKPolicy
	// Want is the version as the file writes it: "22", "lts/*", "^20.10";
	// empty when the file only tells the tool is needed, as a lockfile.
	Want string
	// File is the file's name in the project folder, Key the field in it
	// when it has several, as "engines.node".
	File, Key string
}

// Inherited reports that the requirement came from a parent folder
// (such as a monorepo root) rather than the project directory itself.
func (r Requirement) Inherited() bool {
	return strings.HasPrefix(filepath.ToSlash(r.File), "../")
}

// Project is what Read found in a project folder.
type Project struct {
	Dir          string
	Instructions Instructions
	// Requirements come in the order of the files, as reader lists them.
	Requirements []Requirement
	// Files are the project files Read knows that the folder has, in the
	// order of the requirements.
	Files []string
	// AI are the AI agents' files and folders the project has, as
	// "CLAUDE.md", ".claude".
	AI []string
	// Skills are the skills in the agents' skills directories of the
	// project.
	Skills []inventory.Skill
	// MCP are the MCP servers in the agents' files of the project.
	MCP []inventory.MCPServer
	// Rules are what the agents' settings of the project let them do
	// without asking, make them ask for or forbid; Hooks the commands
	// they run on the agents' events.
	Rules []inventory.AgentRule
	Hooks []inventory.AgentHook
	// Databases are the local databases Compose and .env refer to.
	Databases []DatabaseReference
	// Kube are the Kubernetes files and folders it has, as "Chart.yaml"
	// and "k8s/": Hearth shows them and never applies them.
	Kube []string
	// Changed is when the project last changed, as its files and git's
	// index say; zero when none of them is there.
	Changed time.Time
	// Installs are the folders of dependencies its tools install and can
	// install again, as node_modules and .venv, that it has.
	Installs []string
	// Readme is the project's setup documentation file in Dir, when present
	// (such as README.md), shown only as a file link for the user to open.
	Readme string
	// Git holds read-only repository status (branch, dirty count, remote,
	// last commit) when Dir is inside a Git repository.
	Git *GitInfo
	// Texts are the version files Hearth may write, by name, as Read
	// found them: an edit starts from them.
	Texts map[string]string
	// Errs say which files could not be read; the others still count.
	Errs []error
}

// VersionFiles are the files that pin versions Hearth may write.
var VersionFiles = []string{
	".nvmrc", ".node-version", ".tool-versions", "mise.toml", ".mise.toml", "package.json",
	".python-version", ".ruby-version", ".go-version", ".java-version", ".sdkmanrc",
	"rust-toolchain.toml", "rust-toolchain", ".php-version", ".bun-version", "global.json",
	".terraform-version", ".opentofu-version", ".tofu-version",
}

// installDirs are the dependency folders tools put back from a project's
// files: npm and the others' node_modules, a Python virtual environment.
var installDirs = []string{"node_modules", ".venv"}

// aiFiles are the AI settings a project may have, in the order shown.
var aiFiles = []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md", ".github/copilot-instructions.md", ".agents", ".agent", "_agents", "_agent", ".claude", ".cursor", ".mcp.json"}

type fileReader struct {
	file string
	read func(data []byte) ([]Requirement, error)
}

// readers read the project files Hearth knows, in the order shown. Each
// gets the file's contents and returns its requirements.
var readers = []fileReader{
	{".nvmrc", firstLine("node")},
	{".node-version", firstLine("node")},
	{"package.json", readPackageJSON},
	{"package-lock.json", needs("npm")},
	{"pnpm-lock.yaml", needs("pnpm")},
	{"yarn.lock", needs("yarn")},
	{"bun.lock", needs("bun")},
	{"bun.lockb", needs("bun")},
	{"deno.json", readDenoJSON},
	{"deno.jsonc", readDenoJSON},
	{".tool-versions", readToolVersions},
	{"mise.toml", readMiseTOML},
	{".mise.toml", readMiseTOML},
	{".ruby-version", readRubyVersion},
	{"Gemfile", readGemfile},
	{".java-version", firstLine("java")},
	{".sdkmanrc", readSDKMAN},
	{"pom.xml", readPomXML},
	{"build.gradle", readGradle},
	{"build.gradle.kts", readGradle},
	{".python-version", firstLine("python")},
	{"pyproject.toml", readPyproject},
	{"Pipfile", readPipfile},
	{"poetry.lock", readPoetryLock},
	{"uv.lock", needs("uv")},
	{"rust-toolchain.toml", readRustToolchain},
	{"rust-toolchain", firstLine("rust")},
	{"Cargo.toml", readCargoTOML},
	{".php-version", readPHPVersion},
	{"composer.json", readComposerJSON},
	{".go-version", readGoVersion},
	{"go.mod", readGoMod},
	{"global.json", readGlobalJSON},
	{".bun-version", firstLine("bun")},
	{"bunfig.toml", needs("bun")},
	{"compose.yaml", needs("docker")},
	{"compose.yml", needs("docker")},
	{"docker-compose.yml", needs("docker")},
	{"docker-compose.yaml", needs("docker")},
	{"Dockerfile", needs("docker")},
	{".terraform-version", firstLine("terraform")},
	{".opentofu-version", firstLine("tofu")},
	{".tofu-version", firstLine("tofu")},
}

// Read reads the project files in dir. A file that is missing is skipped;
// one that cannot be read or parsed goes to Errs, as does dir when it is
// gone.
func Read(dir string) Project {
	return ReadWithEnv(dir, nil)
}

// ReadWithEnv reads the project files in dir, using env's PATH to verify
// project MCP server commands when env is non-nil.
func ReadWithEnv(dir string, env sysenv.Env) Project {
	p := Project{Dir: dir}
	if _, err := os.Stat(dir); err != nil {
		p.Errs = append(p.Errs, err)
		return p
	}
	for _, r := range readers {
		path := filepath.Join(dir, r.file)
		data, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if info, serr := os.Stat(path); err != nil && serr == nil && info.IsDir() {
			continue // a folder by that name, as testdata/rust-toolchain
		}
		p.Files = append(p.Files, r.file)
		if err == nil && slices.Contains(VersionFiles, r.file) {
			if p.Texts == nil {
				p.Texts = map[string]string{}
			}
			p.Texts[r.file] = string(data)
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
	p.Instructions = ReadInstructions(dir)
	readDatabases(&p)
	readKube(&p)
	readTerraform(&p)
	readInheritedGlobalJSON(&p)
	readCsproj(&p)
	readInheritedVersionFiles(&p)
	for _, name := range aiFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			p.AI = append(p.AI, name)
		}
	}
	for _, spec := range installSpecs {
		info, err := os.Stat(filepath.Join(dir, spec.name))
		if err != nil || !info.IsDir() {
			continue
		}
		if slices.Contains(installDirs, spec.name) || hasInstallManifest(dir, spec.manifests) {
			p.Installs = append(p.Installs, spec.name)
		}
	}
	p.Readme = findReadme(dir)
	p.Git = readGitInfo(dir, env)
	p.Changed = changed(p)
	p.Skills = ai.ProjectSkills(dir)
	mcp, errs := ai.ProjectMCP(dir, env)
	p.MCP, p.Errs = mcp, append(p.Errs, errs...)
	p.Rules, p.Hooks, errs = ai.ProjectAccess(dir)
	p.Errs = append(p.Errs, errs...)
	return p
}

var readmeFiles = []string{"README.md", "README.MD", "Readme.md", "README", "DEVELOPMENT.md", "CONTRIBUTING.md"}

func findReadme(dir string) string {
	for _, name := range readmeFiles {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			return name
		}
	}
	return ""
}

// Names are the names of the files and folders in a project folder that
// Read reads: the project files and the AI settings.
func Names() []string {
	names := make([]string, 0, len(readers)+len(aiFiles))
	for _, r := range readers {
		names = append(names, r.file)
	}
	names = append(append(names, ".env"), aiFiles...)
	names = append(names, kubeFiles...)
	return append(names, kubeDirs...)
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

// packageTool normalizes a JS runtime or package manager name in package.json
// and rejects empty or unrecognized entries.
func packageTool(name string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "nodejs" {
		name = "node"
	}
	if name == "deno" || slices.Contains(packageTools, name) {
		return name, true
	}
	return "", false
}

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
		if tool, ok := packageTool(name); ok {
			reqs = append(reqs, Requirement{Tool: tool, Want: strings.TrimSpace(version), Key: "packageManager"})
		}
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
			if tool, ok := packageTool(e.Name); ok {
				reqs = append(reqs, Requirement{Tool: tool, Want: strings.TrimSpace(e.Version), Key: field.key})
			}
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
	"terraform": "terraform",
	"tofu":      "tofu", "opentofu": "tofu",
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
		Tool struct {
			Poetry struct {
				Dependencies map[string]any `toml:"dependencies"`
			} `toml:"poetry"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(cfg.Project.RequiresPython); v != "" {
		return []Requirement{{Tool: "python", Want: v, Key: "project.requires-python"}}, nil
	}
	if py, ok := cfg.Tool.Poetry.Dependencies["python"].(string); ok {
		if v := strings.TrimSpace(py); v != "" {
			return []Requirement{{Tool: "python", Want: v, Key: "tool.poetry.dependencies.python"}}, nil
		}
	}
	return []Requirement{{Tool: "python", Key: "project.requires-python"}}, nil
}

func readPipfile(data []byte) ([]Requirement, error) {
	var cfg struct {
		Requires struct {
			PythonFullVersion string `toml:"python_full_version"`
			PythonVersion     string `toml:"python_version"`
		} `toml:"requires"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(cfg.Requires.PythonFullVersion); v != "" {
		return []Requirement{{Tool: "python", Want: v, Key: "requires.python_full_version"}}, nil
	}
	if v := strings.TrimSpace(cfg.Requires.PythonVersion); v != "" {
		return []Requirement{{Tool: "python", Want: v, Key: "requires.python_version"}}, nil
	}
	return []Requirement{{Tool: "python"}}, nil
}

func readPoetryLock(data []byte) ([]Requirement, error) {
	var cfg struct {
		Metadata struct {
			PythonVersions string `toml:"python-versions"`
		} `toml:"metadata"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(cfg.Metadata.PythonVersions); v != "" {
		return []Requirement{{Tool: "python", Want: v, Key: "metadata.python-versions"}}, nil
	}
	return []Requirement{{Tool: "python"}}, nil
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

// readGoMod reads the go and toolchain lines of go.mod: "go 1.23.2",
// "toolchain go1.24.0".
func readGoMod(data []byte) ([]Requirement, error) {
	var reqs []Requirement
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "go":
			reqs = append(reqs, Requirement{Tool: "go", Want: fields[1], Key: "go"})
		case "toolchain":
			reqs = append(reqs, Requirement{Tool: "go", Want: strings.TrimPrefix(fields[1], "go"), Key: "toolchain"})
		}
	}
	if len(reqs) == 0 {
		reqs = []Requirement{{Tool: "go"}}
	}
	return reqs, sc.Err()
}

func readGoVersion(data []byte) ([]Requirement, error) {
	reqs, err := firstLine("go")(data)
	for i := range reqs {
		reqs[i].Want = strings.TrimPrefix(reqs[i].Want, "go")
	}
	return reqs, err
}

func readPHPVersion(data []byte) ([]Requirement, error) {
	reqs, err := firstLine("php")(data)
	for i := range reqs {
		reqs[i].Want = strings.TrimPrefix(reqs[i].Want, "php-")
	}
	return reqs, err
}

func readDenoJSON(data []byte) ([]Requirement, error) {
	clean := stripTrailingCommas(stripJSONComments(data))
	var v any
	if err := json.Unmarshal(clean, &v); err != nil {
		return nil, err
	}
	return []Requirement{{Tool: "deno"}}, nil
}

// stripTrailingCommas removes JSONC trailing commas before closing '}' or ']',
// outside string literals.
func stripTrailingCommas(data []byte) []byte {
	out := slices.Clone(data)
	quoted, escaped := false, false
	for i := range len(out) {
		b := out[i]
		if quoted {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		if b == '"' {
			quoted = true
			continue
		}
		if b != ',' {
			continue
		}
		j := i + 1
		for j < len(out) && (out[j] == ' ' || out[j] == '\t' || out[j] == '\r' || out[j] == '\n') {
			j++
		}
		if j < len(out) && (out[j] == '}' || out[j] == ']') {
			out[i] = ' '
		}
	}
	return out
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

// readSDKMAN reads Java's candidate identifier; other SDKs are outside this page.
func readSDKMAN(data []byte) ([]Requirement, error) {
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) == "java" {
			return []Requirement{{Tool: "java", Want: strings.Trim(strings.TrimSpace(value), `"'`), Key: "java"}}, nil
		}
	}
	return nil, nil
}

// readGemfile recognizes literal Ruby declarations, never evaluating the DSL.
// Dynamic declarations still say that the project uses Ruby.
func readGemfile(data []byte) ([]Requirement, error) {
	req := Requirement{Tool: "ruby"}
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ruby ") && !strings.HasPrefix(line, "ruby(") {
			continue
		}
		req.Key = "ruby"
		rest := strings.TrimSpace(strings.TrimPrefix(line, "ruby"))
		rest = strings.TrimPrefix(rest, "(")
		var parts []string
		for {
			rest = strings.TrimSpace(rest)
			m := rubyLiteral.FindStringSubmatch(rest)
			if m == nil {
				break
			}
			v := m[1]
			if v == "" {
				v = m[2]
			}
			if strings.Contains(v, "#{") {
				parts = nil
				break
			}
			parts = append(parts, v)
			rest = strings.TrimSpace(rest[len(m[0]):])
			if !strings.HasPrefix(rest, ",") {
				break
			}
			rest = strings.TrimPrefix(rest, ",")
		}
		req.Want = strings.Join(parts, ", ")
		break
	}
	return []Requirement{req}, nil
}

var rubyLiteral = regexp.MustCompile(`^(?:'([^']*)'|"([^"]*)")`)

func readRubyVersion(data []byte) ([]Requirement, error) {
	reqs, err := firstLine("ruby")(data)
	for i := range reqs {
		reqs[i].Want = strings.TrimPrefix(reqs[i].Want, "ruby-")
	}
	return reqs, err
}
