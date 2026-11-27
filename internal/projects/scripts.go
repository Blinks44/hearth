package projects

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"go.yaml.in/yaml/v3"
)

// Script is a runnable dev/build/task command declared by a project file.
// Discovery is strictly read-only; scripts are only executed when the user
// explicitly starts them.
type Script struct {
	// Name is the script or task identifier (e.g. "dev", "build", "test").
	Name string
	// Source is the file it came from (e.g. "package.json", "Makefile", "mise.toml").
	Source string
	// Command is a compact human-readable preview of what runs (e.g. "vite --port 5173").
	Command string
	// Argv is the direct command argv Hearth runs (no shell string).
	Argv []string
	// Dev marks scripts likely to launch a long-running dev server or watcher.
	Dev bool
}

const maxProjectScripts = 30

// readScripts statically discovers runnable scripts and tasks from a project's
// manifest files without executing any external tool.
func readScripts(p *Project) {
	var out []Script
	seen := map[string]bool{}
	add := func(s Script) {
		if s.Name == "" || len(s.Argv) == 0 {
			return
		}
		key := s.Source + "\x00" + s.Name
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}

	// 1. package.json scripts (with detected JS package manager).
	if data, err := os.ReadFile(filepath.Join(p.Dir, "package.json")); err == nil {
		for _, s := range parsePackageScripts(p.Dir, data) {
			add(s)
		}
	}

	// 2. mise.toml / .mise.toml [tasks].
	for _, name := range []string{"mise.toml", ".mise.toml"} {
		if data, err := os.ReadFile(filepath.Join(p.Dir, name)); err == nil {
			for _, s := range parseMiseTasks(name, data) {
				add(s)
			}
			break
		}
	}

	// 3. Taskfile.yml / Taskfile.yaml.
	for _, name := range []string{"Taskfile.yml", "Taskfile.yaml", "taskfile.yml", "taskfile.yaml"} {
		if data, err := os.ReadFile(filepath.Join(p.Dir, name)); err == nil {
			for _, s := range parseTaskfile(name, data) {
				add(s)
			}
			break
		}
	}

	// 4. justfile / Justfile / .justfile.
	for _, name := range []string{"justfile", "Justfile", ".justfile"} {
		if data, err := os.ReadFile(filepath.Join(p.Dir, name)); err == nil {
			for _, s := range parseJustfile(name, data) {
				add(s)
			}
			break
		}
	}

	// 5. Makefile / makefile / GNUmakefile.
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		if data, err := os.ReadFile(filepath.Join(p.Dir, name)); err == nil {
			for _, s := range parseMakefile(name, data) {
				add(s)
			}
			break
		}
	}

	// 6. pyproject.toml [project.scripts] / [tool.poetry.scripts].
	if data, err := os.ReadFile(filepath.Join(p.Dir, "pyproject.toml")); err == nil {
		for _, s := range parsePyprojectScripts(p.Dir, data) {
			add(s)
		}
	}

	// 7. Go / Cargo fallback run targets when no explicit dev script already exists.
	hasDev := slices.ContainsFunc(out, func(s Script) bool { return s.Dev })
	if !hasDev {
		if fileExists(filepath.Join(p.Dir, "go.mod")) && fileExists(filepath.Join(p.Dir, "main.go")) {
			add(Script{
				Name:    "run",
				Source:  "go.mod",
				Command: "go run .",
				Argv:    []string{"go", "run", "."},
				Dev:     true,
			})
		}
		if fileExists(filepath.Join(p.Dir, "Cargo.toml")) && fileExists(filepath.Join(p.Dir, "src", "main.rs")) {
			add(Script{
				Name:    "run",
				Source:  "Cargo.toml",
				Command: "cargo run",
				Argv:    []string{"cargo", "run"},
				Dev:     true,
			})
		}
	}

	if len(out) == 0 {
		return
	}
	slices.SortStableFunc(out, compareScripts)
	if len(out) > maxProjectScripts {
		out = out[:maxProjectScripts]
	}
	p.Scripts = out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func detectNodePM(dir string, packageManagerField string) string {
	pmField := strings.ToLower(strings.TrimSpace(packageManagerField))
	for _, pm := range []string{"pnpm", "yarn", "bun", "npm"} {
		if strings.HasPrefix(pmField, pm+"@") || pmField == pm {
			return pm
		}
	}
	switch {
	case fileExists(filepath.Join(dir, "pnpm-lock.yaml")):
		return "pnpm"
	case fileExists(filepath.Join(dir, "yarn.lock")):
		return "yarn"
	case fileExists(filepath.Join(dir, "bun.lock")) || fileExists(filepath.Join(dir, "bun.lockb")):
		return "bun"
	default:
		return "npm"
	}
}

func parsePackageScripts(dir string, data []byte) []Script {
	var pkg struct {
		PackageManager string            `json:"packageManager"`
		Scripts        map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil || len(pkg.Scripts) == 0 {
		return nil
	}
	pm := detectNodePM(dir, pkg.PackageManager)
	out := make([]Script, 0, len(pkg.Scripts))
	for name, cmd := range pkg.Scripts {
		name = strings.TrimSpace(name)
		cmd = strings.TrimSpace(cmd)
		if name == "" || cmd == "" || strings.HasPrefix(name, "-") ||
			strings.HasPrefix(name, "pre") && len(name) > 3 && pkg.Scripts[name[3:]] != "" ||
			strings.HasPrefix(name, "post") && len(name) > 4 && pkg.Scripts[name[4:]] != "" {
			continue
		}
		argv := []string{pm, "run", name}
		out = append(out, Script{
			Name:    name,
			Source:  "package.json",
			Command: cmd,
			Argv:    argv,
			Dev:     isDevScriptName(name),
		})
	}
	return out
}

func parseMiseTasks(source string, data []byte) []Script {
	var cfg struct {
		Tasks map[string]any `toml:"tasks"`
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil || len(cfg.Tasks) == 0 {
		return nil
	}
	out := make([]Script, 0, len(cfg.Tasks))
	for name, raw := range cfg.Tasks {
		if name == "" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, "-") {
			continue
		}
		cmd := ""
		switch v := raw.(type) {
		case string:
			cmd = strings.TrimSpace(v)
		case map[string]any:
			if desc, ok := v["description"].(string); ok && strings.TrimSpace(desc) != "" {
				cmd = strings.TrimSpace(desc)
			} else if runStr, ok := v["run"].(string); ok {
				cmd = firstNonEmptyLine(runStr)
			}
		}
		if cmd == "" {
			cmd = "mise run " + name
		}
		out = append(out, Script{
			Name:    name,
			Source:  source,
			Command: cmd,
			Argv:    []string{"mise", "run", name},
			Dev:     isDevScriptName(name),
		})
	}
	return out
}

func parseTaskfile(source string, data []byte) []Script {
	var doc struct {
		Tasks map[string]any `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Tasks) == 0 {
		return nil
	}
	out := make([]Script, 0, len(doc.Tasks))
	for name, raw := range doc.Tasks {
		if name == "" || strings.HasPrefix(name, "_") || strings.HasPrefix(name, "-") {
			continue
		}
		cmd := ""
		if m, ok := raw.(map[string]any); ok {
			if internal, ok := m["internal"].(bool); ok && internal {
				continue
			}
			if desc, ok := m["desc"].(string); ok && strings.TrimSpace(desc) != "" {
				cmd = strings.TrimSpace(desc)
			}
		}
		if cmd == "" {
			cmd = "task " + name
		}
		out = append(out, Script{
			Name:    name,
			Source:  source,
			Command: cmd,
			Argv:    []string{"task", name},
			Dev:     isDevScriptName(name),
		})
	}
	return out
}

var justRecipeLine = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9_-]*)(?:\s+[^:]*)?:(?:[^=]|$)`)

func parseJustfile(source string, data []byte) []Script {
	var out []Script
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' || strings.HasPrefix(line, "alias ") ||
			strings.HasPrefix(line, "set ") || strings.HasPrefix(line, "export ") || strings.HasPrefix(line, "import ") {
			continue
		}
		m := justRecipeLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		out = append(out, Script{
			Name:    name,
			Source:  source,
			Command: "just " + name,
			Argv:    []string{"just", name},
			Dev:     isDevScriptName(name),
		})
	}
	return out
}

var makeTargetLine = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9_.-]*)\s*:([^=]|$)`)

func parseMakefile(source string, data []byte) []Script {
	var out []Script
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '\t' || line[0] == ' ' || line[0] == '#' || line[0] == '.' {
			continue
		}
		m := makeTargetLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		out = append(out, Script{
			Name:    name,
			Source:  source,
			Command: "make " + name,
			Argv:    []string{"make", name},
			Dev:     isDevScriptName(name),
		})
	}
	return out
}

func parsePyprojectScripts(dir string, data []byte) []Script {
	var doc struct {
		Project struct {
			Scripts map[string]string `toml:"scripts"`
		} `toml:"project"`
		Tool struct {
			Poetry struct {
				Scripts map[string]string `toml:"scripts"`
			} `toml:"poetry"`
		} `toml:"tool"`
	}
	if _, err := toml.Decode(string(data), &doc); err != nil {
		return nil
	}
	scripts := doc.Project.Scripts
	if len(scripts) == 0 {
		scripts = doc.Tool.Poetry.Scripts
	}
	if len(scripts) == 0 {
		return nil
	}
	runner := "uv"
	if !fileExists(filepath.Join(dir, "uv.lock")) && fileExists(filepath.Join(dir, "poetry.lock")) {
		runner = "poetry"
	}
	out := make([]Script, 0, len(scripts))
	for name, entry := range scripts {
		if name == "" || strings.HasPrefix(name, "-") {
			continue
		}
		out = append(out, Script{
			Name:    name,
			Source:  "pyproject.toml",
			Command: strings.TrimSpace(entry),
			Argv:    []string{runner, "run", name},
			Dev:     isDevScriptName(name),
		})
	}
	return out
}

func firstNonEmptyLine(s string) string {
	for line := range strings.Lines(s) {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

func isDevScriptName(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "dev", "start", "serve", "server", "run", "preview", "watch", "storybook", "web", "api":
		return true
	}
	for _, prefix := range []string{"dev:", "start:", "serve:", "watch:", "run:"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func scriptPriority(s Script) int {
	lower := strings.ToLower(s.Name)
	switch lower {
	case "dev":
		return 0
	case "start":
		return 1
	case "serve", "server", "run":
		return 2
	case "preview", "storybook", "watch":
		return 3
	}
	if s.Dev {
		return 4
	}
	switch lower {
	case "build":
		return 10
	case "test":
		return 11
	case "lint", "check":
		return 12
	}
	return 20
}

func compareScripts(a, b Script) int {
	pa, pb := scriptPriority(a), scriptPriority(b)
	if pa != pb {
		return pa - pb
	}
	if a.Name != b.Name {
		return strings.Compare(a.Name, b.Name)
	}
	return strings.Compare(a.Source, b.Source)
}
