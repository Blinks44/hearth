package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
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
	want := []inventory.Package{{Name: "@angular/cli", Version: "19.0.0", Manager: "npm"}, {Name: "npm", Version: "10.9.0", Manager: "npm"}}
	if got := globalPackages(dir); !reflect.DeepEqual(got, want) {
		t.Errorf("globalPackages = %+v, want %+v", got, want)
	}
	if got := globalPackages(filepath.Join(dir, "missing")); got != nil {
		t.Errorf("globalPackages(missing) = %+v, want nil", got)
	}
}

// npm -g goes where npm's prefix says, found as npm finds it: Node's
// Windows installer ships an npmrc with ${APPDATA}\npm, ~/.npmrc and
// npm_config_prefix come before it, and without any it is node's own.
func TestNpmGlobal(t *testing.T) {
	dir := t.TempDir()
	home, appData := filepath.Join(dir, "home"), filepath.Join(dir, "AppData")
	node := filepath.Join(dir, "nodejs", nodeBin)
	sep := string(filepath.Separator)
	writeFile(t, filepath.Join(nodeModules(node), "npm", "npmrc"), "; builtin\r\nprefix=${APPDATA}"+sep+"npm\r\n")
	env := sysenv.Env{"HOME=" + home, "APPDATA=" + appData}

	modules, bin := npmGlobal(node, env)
	if want := filepath.Join(appData, "npm"); modules != prefixModules(want) || bin != prefixBin(want) {
		t.Errorf("builtin npmrc: %s, %s", modules, bin)
	}
	writeFile(t, filepath.Join(home, ".npmrc"), "prefix = \"~"+sep+".npm-global\"\n")
	if modules, _ := npmGlobal(node, env); modules != prefixModules(filepath.Join(home, ".npm-global")) {
		t.Errorf("~/.npmrc: %s", modules)
	}
	if modules, _ := npmGlobal(node, append(env, "npm_config_prefix="+filepath.Join(dir, "env"))); modules != prefixModules(filepath.Join(dir, "env")) {
		t.Errorf("npm_config_prefix: %s", modules)
	}
	if modules, bin := npmGlobal(filepath.Join(dir, "other", nodeBin), sysenv.Env{"HOME=" + filepath.Join(dir, "nobody")}); modules != nodeModules(filepath.Join(dir, "other", nodeBin)) || bin != filepath.Dir(filepath.Join(dir, "other", nodeBin)) {
		t.Errorf("no prefix: %s, %s", modules, bin)
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

func checkInstallations(t *testing.T, got inventory.ToolState, want []inventory.Installation) {
	t.Helper()
	if len(want) > 0 && got.Tool != want[0].Tool {
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
