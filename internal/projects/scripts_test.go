package projects

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestReadScriptsPackageJSONAndMakefile(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("pnpm-lock.yaml", "lockfileVersion: '9.0'\n")
	mustWrite("package.json", `{
  "scripts": {
    "prebuild": "echo pre",
    "build": "vite build",
    "dev": "vite --port 5173",
    "test": "vitest run",
    "lint": "eslint ."
  }
}`)
	mustWrite("Makefile", `.PHONY: all check
check:
	go test ./...
serve:
	python3 -m http.server
`)
	mustWrite("mise.toml", `[tasks]
watch = "air"
_private = "echo hidden"
`)

	p := Read(dir)
	if len(p.Scripts) < 5 {
		t.Fatalf("expected >=5 scripts, got %+v", p.Scripts)
	}
	// "dev" must be sorted first and use pnpm.
	first := p.Scripts[0]
	if first.Name != "dev" || !first.Dev || !slices.Equal(first.Argv, []string{"pnpm", "run", "dev"}) {
		t.Fatalf("unexpected first script: %+v", first)
	}
	// prebuild and _private must be excluded.
	for _, s := range p.Scripts {
		if s.Name == "prebuild" || s.Name == "_private" {
			t.Fatalf("unexpected script included: %+v", s)
		}
	}
	// Makefile "serve" and mise.toml "watch" should be Dev scripts.
	if !slices.ContainsFunc(p.Scripts, func(s Script) bool {
		return s.Name == "serve" && s.Source == "Makefile" && s.Dev && slices.Equal(s.Argv, []string{"make", "serve"})
	}) {
		t.Fatalf("missing Makefile serve script in %+v", p.Scripts)
	}
	if !slices.ContainsFunc(p.Scripts, func(s Script) bool {
		return s.Name == "watch" && s.Source == "mise.toml" && s.Dev && slices.Equal(s.Argv, []string{"mise", "run", "watch"})
	}) {
		t.Fatalf("missing mise.toml watch script in %+v", p.Scripts)
	}
}

func TestReadScriptsGoFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := Read(dir)
	if len(p.Scripts) != 1 || p.Scripts[0].Name != "run" || !slices.Equal(p.Scripts[0].Argv, []string{"go", "run", "."}) {
		t.Fatalf("unexpected Go fallback scripts: %+v", p.Scripts)
	}
}

func TestReadScriptsYarnAndFlagPrefix(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("yarn.lock", "")
	mustWrite("package.json", `{
  "scripts": {
    "add": "echo shadow",
    "--eval": "echo bad"
  }
}`)
	mustWrite("mise.toml", `[tasks]
"-bad" = "echo nope"
ok = "echo ok"
`)
	p := Read(dir)
	for _, s := range p.Scripts {
		if s.Name == "--eval" || s.Name == "-bad" {
			t.Fatalf("flag-like script should be skipped: %+v", s)
		}
	}
	if !slices.ContainsFunc(p.Scripts, func(s Script) bool {
		return s.Name == "add" && slices.Equal(s.Argv, []string{"yarn", "run", "add"})
	}) {
		t.Fatalf("expected yarn run add, got %+v", p.Scripts)
	}
}
