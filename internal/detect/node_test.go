package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestPickVersion(t *testing.T) {
	installed := []string{"20.18.0", "22.9.0", "22.11.0", "18.20.4"}
	tests := []struct{ want, got string }{
		{"22.11.0", "22.11.0"},
		{"v20.18.0", "20.18.0"},
		{"22", "22.11.0"}, // 22.11 is newer than 22.9
		{"22.9", "22.9.0"},
		{"2", ""}, // a line, not a prefix of "22"
		{"lts/*", "22.11.0"},
		{"node", "22.11.0"},
		{"23", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := pickVersion(tt.want, installed); got != tt.got {
			t.Errorf("pickVersion(%q) = %q, want %q", tt.want, got, tt.got)
		}
	}
}

func TestMiseTool(t *testing.T) {
	config := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, config, `[settings]
node = "ignored"

[tools]
python = "3.13"
"node" = ["22", "20"] # first is the default
go = { version = "1.27" }
`)
	for tool, want := range map[string]string{"node": "22", "python": "3.13", "go": "1.27", "ruby": ""} {
		if got := miseTool(config, tool); got != want {
			t.Errorf("miseTool(%q) = %q, want %q", tool, got, want)
		}
	}
}

func TestGlobalPackages(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "npm", "package.json"), `{"name": "npm", "version": "10.9.0"}`)
	writeFile(t, filepath.Join(dir, "@angular", "cli", "package.json"), `{"name": "@angular/cli", "version": "19.0.0"}`)
	writeFile(t, filepath.Join(dir, "broken", "package.json"), `{`)
	writeFile(t, filepath.Join(dir, ".package-lock.json"), `{}`)
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := []inventory.Package{{Name: "@angular/cli", Version: "19.0.0"}, {Name: "npm", Version: "10.9.0"}}
	if got := globalPackages(dir); !reflect.DeepEqual(got, want) {
		t.Errorf("globalPackages = %+v, want %+v", got, want)
	}
	if got := globalPackages(filepath.Join(dir, "missing")); got != nil {
		t.Errorf("globalPackages(missing) = %+v, want nil", got)
	}
}

// root is a temporary directory with symlinks resolved, so paths built on
// it equal the real paths the detector resolves (macOS's temp lies under
// the /var symlink).
func root(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeNode puts an executable node at dir/nodeBin and returns its path.
func fakeNode(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, nodeBin)
	writeFile(t, path, "")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func checkInstallations(t *testing.T, got inventory.ToolState, want []inventory.Installation) {
	t.Helper()
	if got.Tool != "node" {
		t.Errorf("Tool = %q", got.Tool)
	}
	if len(got.Installations) != len(want) {
		t.Fatalf("got %d installations, want %d:\n%+v", len(got.Installations), len(want), got.Installations)
	}
	for i := range want {
		if !reflect.DeepEqual(got.Installations[i], want[i]) {
			t.Errorf("installation %d\n got %+v\nwant %+v", i, got.Installations[i], want[i])
		}
	}
}
