package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/locales"
)

// PinTarget is a version to pin in a project: the tool, as "node", "python" or
// "pnpm", its title, as the user sees it, and the version manager the user
// switches the tool with, which picks the file a project without one gets.
type PinTarget struct {
	Tool, Title, Version, Manager string
}

type pinSpec struct {
	files       []string
	defaultFile string
	miseKey     string
	asdfKey     string
	asdfAliases []string
}

var pinSpecs = map[string]pinSpec{
	"node": {
		files:       []string{".nvmrc", ".node-version", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".nvmrc",
		miseKey:     "node",
		asdfKey:     "nodejs",
		asdfAliases: []string{"nodejs", "node"},
	},
	"python": {
		files:       []string{".python-version", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".python-version",
		miseKey:     "python",
		asdfKey:     "python",
		asdfAliases: []string{"python"},
	},
	"ruby": {
		files:       []string{".ruby-version", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".ruby-version",
		miseKey:     "ruby",
		asdfKey:     "ruby",
		asdfAliases: []string{"ruby"},
	},
	"go": {
		files:       []string{".go-version", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".go-version",
		miseKey:     "go",
		asdfKey:     "golang",
		asdfAliases: []string{"golang", "go"},
	},
	"java": {
		files:       []string{".java-version", ".sdkmanrc", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".java-version",
		miseKey:     "java",
		asdfKey:     "java",
		asdfAliases: []string{"java"},
	},
	"rust": {
		files:       []string{"rust-toolchain.toml", "rust-toolchain", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: "rust-toolchain.toml",
		miseKey:     "rust",
		asdfKey:     "rust",
		asdfAliases: []string{"rust"},
	},
	"php": {
		files:       []string{".php-version", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".php-version",
		miseKey:     "php",
		asdfKey:     "php",
		asdfAliases: []string{"php"},
	},
	"bun": {
		files:       []string{".bun-version", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".bun-version",
		miseKey:     "bun",
		asdfKey:     "bun",
		asdfAliases: []string{"bun"},
	},
	"dotnet": {
		files:       []string{"global.json", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: "global.json",
		miseKey:     "dotnet",
		asdfKey:     "dotnet",
		asdfAliases: []string{"dotnet", "dotnet-core"},
	},
	"terraform": {
		files:       []string{".terraform-version", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".terraform-version",
		miseKey:     "terraform",
		asdfKey:     "terraform",
		asdfAliases: []string{"terraform"},
	},
	"tofu": {
		files:       []string{".opentofu-version", ".tofu-version", ".tool-versions", "mise.toml", ".mise.toml"},
		defaultFile: ".opentofu-version",
		miseKey:     "opentofu",
		asdfKey:     "opentofu",
		asdfAliases: []string{"opentofu", "tofu"},
	},
}

// PinPlan returns the plan that writes t's version to the file project p
// already pins it in, or that its version manager reads; ok is false when
// the file pins that version already or Hearth cannot write it.
func PinPlan(p projects.Project, t PinTarget) (plan Plan, ok bool) {
	file, key := pinFile(p, t)
	if file == "" {
		return Plan{}, false
	}
	before, existed := p.Texts[file]
	after, err := pinText(file, before, t)
	if err != nil || existed && after == before {
		return Plan{}, false
	}
	shown := file
	if key != "" {
		shown += " · " + key
	}
	path := filepath.Join(p.Dir, file)
	steps := []Step{{Edit: &FileEdit{Path: path, Before: before, After: after, New: !existed}}}
	if !existed && (file == "mise.toml" || file == ".mise.toml") {
		steps = append(steps, Step{Argv: []string{"mise", "trust", path}})
	}
	plan = Plan{
		Title:   locales.PinTitle(t.Title, t.Version, filepath.Base(p.Dir)),
		Why:     locales.PinWhy(shown),
		Action:  locales.PinAction(t.Version),
		Steps:   steps,
		Changes: []string{locales.PinChange(shown, t.Title, t.Version)},
	}
	if !existed {
		plan.Why = locales.PinWhyNew(file, t.Manager)
	}
	plan.Verify = func(context.Context) (string, error) {
		for _, r := range projects.Read(p.Dir).Requirements {
			if r.Tool == t.Tool && r.File == file && r.Want == t.Version {
				return locales.PinChecked(shown, t.Title, t.Version), nil
			}
		}
		return "", errors.New(locales.PinNotChecked(shown, t.Title, t.Version))
	}
	return plan, true
}

// pinFile is the file t's version goes to in p, with the key in it, ""
// when there is none to write: for pnpm and Yarn, packageManager in
// package.json; for supported languages/runtimes, the file that pins it
// already, else an existing mise or asdf file, else the manager's default.
func pinFile(p projects.Project, t PinTarget) (file, key string) {
	if t.Tool == "pnpm" || t.Tool == "yarn" {
		if _, ok := p.Texts["package.json"]; !ok {
			return "", ""
		}
		// A project another package manager is set for stays so.
		for _, r := range p.Requirements {
			if r.File == "package.json" && r.Key == "packageManager" && r.Tool != t.Tool {
				return "", ""
			}
		}
		return "package.json", "packageManager"
	}
	spec, ok := pinSpecs[t.Tool]
	if !ok {
		return "", ""
	}
	for _, f := range spec.files {
		if slices.ContainsFunc(p.Requirements, func(r projects.Requirement) bool { return r.Tool == t.Tool && r.File == f }) {
			return f, pinFileKey(f)
		}
	}
	for _, f := range []string{".tool-versions", "mise.toml", ".mise.toml"} {
		if _, ok := p.Texts[f]; ok {
			return f, ""
		}
	}
	switch t.Manager {
	case "asdf":
		return ".tool-versions", ""
	case "mise":
		return "mise.toml", ""
	case "sdkman":
		if t.Tool == "java" {
			return ".sdkmanrc", "java"
		}
	}
	return spec.defaultFile, pinFileKey(spec.defaultFile)
}

func pinFileKey(file string) string {
	switch file {
	case "rust-toolchain.toml":
		return "toolchain.channel"
	case "global.json":
		return "sdk.version"
	case ".sdkmanrc":
		return "java"
	default:
		return ""
	}
}

// pinText returns text, file's contents, pinning t's version.
func pinText(file, text string, t PinTarget) (string, error) {
	spec := pinSpecs[t.Tool]
	switch file {
	case "package.json":
		return SetJSON(text, "packageManager", t.Tool+"@"+t.Version)
	case "global.json":
		return setGlobalJSONVersion(text, t.Version)
	case "mise.toml", ".mise.toml":
		return SetTOML(text, "tools", spec.miseKey, t.Version)
	case "rust-toolchain.toml":
		return SetTOML(text, "toolchain", "channel", t.Version)
	case ".sdkmanrc":
		return setSDKMANJava(text, t.Version), nil
	case ".tool-versions":
		lines := splitLines(text)
		for i, l := range lines {
			if f := strings.Fields(l); len(f) > 0 && slices.Contains(spec.asdfAliases, f[0]) {
				lines[i] = f[0] + " " + t.Version
				return joinLines(lines), nil
			}
		}
		return joinLines(append(lines, spec.asdfKey+" "+t.Version)), nil
	}
	// Single-version files (.nvmrc, .python-version, .go-version, etc.):
	// the version is the first line that is not empty or a comment.
	lines := splitLines(text)
	for i, l := range lines {
		if v, _, _ := strings.Cut(l, "#"); strings.TrimSpace(v) != "" {
			lines[i] = t.Version
			return joinLines(lines), nil
		}
	}
	return joinLines(append(lines, t.Version)), nil
}

func setSDKMANJava(text, version string) string {
	lines := splitLines(text)
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if k, _, ok := strings.Cut(trimmed, "="); ok && strings.TrimSpace(k) == "java" {
			lines[i] = "java=" + version
			return joinLines(lines)
		}
	}
	return joinLines(append(lines, "java="+version))
}

func setGlobalJSONVersion(text, version string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return fmt.Sprintf("{\n  \"sdk\": {\n    \"version\": %q\n  }\n}\n", version), nil
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return "", fmt.Errorf("read global.json: %w", err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	sdk, ok := doc["sdk"].(map[string]any)
	if !ok || sdk == nil {
		sdk = map[string]any{}
	}
	sdk["version"] = version
	doc["sdk"] = sdk
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode global.json: %w", err)
	}
	return string(out) + "\n", nil
}
