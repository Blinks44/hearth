package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/projects"
)

func TestPinPlan(t *testing.T) {
	node := PinTarget{Tool: "node", Title: "Node.js", Version: "22.12.0", Manager: "fnm"}
	tests := []struct {
		name   string
		files  map[string]string
		target PinTarget
		// file is the file the plan edits, "" when there is no plan.
		file, after string
	}{
		{"the .nvmrc the project has", map[string]string{".nvmrc": "# LTS\n18\n", "package.json": "{}\n"}, node,
			".nvmrc", "# LTS\n22.12.0\n"},
		{".node-version", map[string]string{".node-version": "v20.1.0"}, node, ".node-version", "22.12.0\n"},
		{"the nodejs line of .tool-versions", map[string]string{".tool-versions": "python 3.12.1\nnodejs 18.20.0\n"}, node,
			".tool-versions", "python 3.12.1\nnodejs 22.12.0\n"},
		{".tool-versions without Node", map[string]string{".tool-versions": "python 3.12.1\n"}, node,
			".tool-versions", "python 3.12.1\nnodejs 22.12.0\n"},
		{"mise.toml", map[string]string{"mise.toml": "[tools]\npython = \"3.12\"\n"}, node,
			"mise.toml", "[tools]\npython = \"3.12\"\nnode = \"22.12.0\"\n"},
		{"no file: the one fnm reads", map[string]string{"package.json": "{}\n"}, node, ".nvmrc", "22.12.0\n"},
		{"no file: the one mise reads", nil, PinTarget{Tool: "node", Title: "Node.js", Version: "22.12.0", Manager: "mise"},
			"mise.toml", "[tools]\nnode = \"22.12.0\"\n"},
		{"no file: the one asdf reads", nil, PinTarget{Tool: "node", Title: "Node.js", Version: "22.12.0", Manager: "asdf"},
			".tool-versions", "nodejs 22.12.0\n"},
		{"pinned already", map[string]string{".nvmrc": "22.12.0\n"}, node, "", ""},
		{"packageManager", map[string]string{"package.json": "{\n  \"name\": \"app\",\n  \"packageManager\": \"pnpm@8.15.0\"\n}\n"},
			PinTarget{Tool: "pnpm", Title: "pnpm", Version: "9.1.0"},
			"package.json", "{\n  \"name\": \"app\",\n  \"packageManager\": \"pnpm@9.1.0\"\n}\n"},
		{"packageManager of another tool stays", map[string]string{"package.json": "{\"packageManager\": \"yarn@4.1.0\"}"},
			PinTarget{Tool: "pnpm", Title: "pnpm", Version: "9.1.0"}, "", ""},
		{"no package.json for packageManager", nil, PinTarget{Tool: "pnpm", Title: "pnpm", Version: "9.1.0"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, text := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			p := projects.Read(dir)
			plan, ok := PinPlan(p, tt.target)
			if tt.file == "" {
				if ok {
					t.Fatalf("a plan where none is due: %+v", plan.Steps[0].Edit)
				}
				return
			}
			if !ok {
				t.Fatal("no plan")
			}
			e := plan.Steps[0].Edit
			if e.Path != filepath.Join(dir, tt.file) || e.After != tt.after {
				t.Fatalf("edits %s to\n%q\nwant %s to\n%q", e.Path, e.After, tt.file, tt.after)
			}
			_, existed := tt.files[tt.file]
			if e.New == existed {
				t.Errorf("New = %v for a file that existed: %v", e.New, existed)
			}
			if _, err := applyEdit(*e, time.Now()); err != nil {
				t.Fatal(err)
			}
			if found, err := plan.Verify(context.Background()); err != nil || !strings.Contains(found, tt.target.Version) {
				t.Errorf("Verify() = %q, %v", found, err)
			}
			if again, ok := PinPlan(projects.Read(dir), tt.target); ok {
				t.Errorf("pinned again: %+v", again.Steps[0].Edit)
			}
		})
	}
}
