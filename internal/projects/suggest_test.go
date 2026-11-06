package projects

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Suggest looks only into the usual code folders, finds the projects in
// them newest first, and leaves a monorepo's packages to its root.
func TestSuggest(t *testing.T) {
	home := t.TempDir()
	write := func(path, content string, age time.Duration) {
		t.Helper()
		full := filepath.Join(home, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-age)
		if err := os.Chtimes(full, when, when); err != nil {
			t.Fatal(err)
		}
	}
	write("dev/old-landing/package.json", `{}`, 240*24*time.Hour)
	write("dev/shop-web/.nvmrc", "22\n", time.Hour)
	write("dev/shop-web/apps/web/.nvmrc", "20\n", time.Hour)
	write("code/api/go.mod", "module api\n\ngo 1.27\n", 48*time.Hour)
	write("Documents/notes/package.json", `{}`, time.Hour)

	folders, found := Suggest(home)
	if len(folders) != 2 {
		t.Errorf("folders %q, want dev and code", folders)
	}
	var names []string
	for _, s := range found {
		names = append(names, filepath.Base(s.Project.Dir))
	}
	want := []string{"shop-web", "api", "old-landing"}
	if len(names) != len(want) {
		t.Fatalf("found %q, want %q", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("found %q, want %q", names, want)
		}
	}
	if old := found[2].Changed; time.Since(old) < 200*24*time.Hour {
		t.Errorf("old-landing changed %v", old)
	}
}
