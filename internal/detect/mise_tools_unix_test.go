//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// mise's tools without a page come from installs/, its links and empty
// folders aside; the global config picks the default, a version's folder
// on PATH is in it, and with the shims the default runs.
func TestMiseTools(t *testing.T) {
	dir := root(t)
	data := filepath.Join(dir, "mise")
	for _, v := range []string{"node/22.12.0", "zig/0.13.0", "zig/0.12.0", "deno/2.1.4", "npm-prettier/3.4.2"} {
		if err := os.MkdirAll(filepath.Join(data, "installs", filepath.FromSlash(v), "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("0.13.0", filepath.Join(data, "installs", "zig", "latest")); err != nil {
		t.Fatal(err)
	}
	// A download that broke off leaves the version's folder empty.
	if err := os.MkdirAll(filepath.Join(data, "installs", "jq", "1.7.1"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "config.toml")
	writeFile(t, config, "[tools]\nzig = \"0.13\"\ndeno = \"2.1.4\"\n")
	env := sysenv.Env{"HOME=" + dir, "MISE_DATA_DIR=" + data, "MISE_GLOBAL_CONFIG_FILE=" + config,
		"PATH=" + filepath.Join(data, "installs", "deno", "2.1.4", "bin") + ":/usr/bin"}
	gui := sysenv.Env{"HOME=" + dir, "MISE_DATA_DIR=" + data, "MISE_GLOBAL_CONFIG_FILE=" + config, "PATH=" + filepath.Join(data, "shims")}
	st, err := MiseTools(t.Context(), &run.Fake{}, env, gui)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		name, version string
		def           bool
		term, apps    int
	}
	var got []row
	for _, mt := range st.MiseTools {
		for _, in := range mt.Installations {
			got = append(got, row{mt.Name, in.Version, in.Default, int(in.OnPath), int(in.InGUI)})
		}
	}
	want := []row{
		{"deno", "2.1.4", true, 1, 1},
		{"npm-prettier", "3.4.2", false, 0, 0},
		{"zig", "0.13.0", true, 0, 1},
		{"zig", "0.12.0", false, 0, 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if root := st.MiseTools[2].Installations[0].Source.Root; root != filepath.Join(data, "installs", "zig", "0.13.0") {
		t.Errorf("root %q", root)
	}
	// Without mise's directory there is nothing to list.
	if st, err := MiseTools(t.Context(), &run.Fake{}, sysenv.Env{"HOME=" + dir}, nil); err != nil || len(st.MiseTools) != 0 {
		t.Errorf("without mise: %+v, %v", st, err)
	}
}
