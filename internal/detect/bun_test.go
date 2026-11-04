package detect

import (
	"path/filepath"
	"testing"
)

func TestNpmPackage(t *testing.T) {
	root := root(t)
	// Windows: npm writes bun.cmd beside the global node_modules.
	npm := filepath.Join(root, "npm")
	writeFile(t, filepath.Join(npm, "node_modules", "bun", "package.json"), `{"name":"bun","version":"1.1.30"}`)
	cmd := filepath.Join(npm, "bun.cmd")
	writeFile(t, cmd, "")
	other := filepath.Join(root, "bin", "bun")
	writeFile(t, other, "")

	for _, tt := range []struct{ path, real, want string }{
		{cmd, cmd, filepath.Join(npm, "node_modules", "bun")},
		{other, filepath.Join(root, "lib", "node_modules", "bun", "bin", "bun.exe"), filepath.Join(root, "lib", "node_modules", "bun")},
		{other, other, ""},
		// A package of another name with a bun folder inside is not bun's.
		{other, filepath.Join(root, "lib", "node_modules", "x", "bun", "bun"), ""},
	} {
		if got := npmPackage(tt.path, tt.real, "bun"); got != tt.want {
			t.Errorf("npmPackage(%q, %q) = %q, want %q", tt.path, tt.real, got, tt.want)
		}
	}
}

func TestHomeForms(t *testing.T) {
	home := filepath.Join(string(filepath.Separator)+"home", "me")
	got := homeForms(filepath.Join(home, ".bun"), home)
	want := []string{filepath.Join(home, ".bun"), "$HOME/.bun", "${HOME}/.bun", "~/.bun"}
	if len(got) != len(want) {
		t.Fatalf("homeForms = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("homeForms = %q, want %q", got, want)
		}
	}
	if got := homeForms("/opt/bun", home); len(got) != 1 {
		t.Errorf("homeForms outside home = %q", got)
	}
}
