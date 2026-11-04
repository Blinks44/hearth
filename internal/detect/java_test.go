package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func fakeJava(t *testing.T, root, version string) string {
	t.Helper()
	path := filepath.Join(root, "bin", javaExe)
	writeFile(t, path, "")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "release"), "JAVA_VERSION=\""+version+"\"\nIMPLEMENTOR=\"Eclipse Adoptium\"\n")
	return path
}

func TestJavaDiscovery(t *testing.T) {
	dir := root(t)
	mise := filepath.Join(dir, "mise")
	jdk := filepath.Join(mise, "installs", "java", "temurin-21.0.5")
	path := fakeJava(t, jdk, "21.0.5")
	shim := filepath.Join(mise, "shims", javaExe)
	writeFile(t, shim, "")
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "old-jdk")
	fakeJava(t, old, "17.0.13")
	config := filepath.Join(dir, "mise.toml")
	writeFile(t, config, "[tools]\njava = \"temurin-21\"\n")
	env := sysenv.Env{"HOME=" + dir, "USERPROFILE=" + dir, "PATH=" + filepath.Dir(shim), "PATHEXT=.EXE", "MISE_DATA_DIR=" + mise, "MISE_GLOBAL_CONFIG_FILE=" + config, "JAVA_HOME=" + old}
	r := &run.Fake{}
	st, err := Java(t.Context(), r, env, sysenv.Env{"PATH=" + filepath.Dir(path), "PATHEXT=.EXE"})
	if err != nil {
		t.Fatal(err)
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 || st.Installations[active].Version != "21.0.5" {
		t.Fatalf("terminal %d: %+v", active, st)
	}
	in := st.Installations[active]
	if in.Java.Vendor != "Eclipse Adoptium" || in.InGUI != 1 {
		t.Fatalf("JDK %+v", in)
	}
	foundHome := false
	for _, in := range st.Installations {
		if in.Java != nil && in.Java.Home == old {
			foundHome = in.Version == "17.0.13"
		}
	}
	if !foundHome {
		t.Fatalf("JAVA_HOME not found: %+v", st)
	}
	for _, call := range r.Calls() {
		if call.Args[0] != "/usr/libexec/java_home" {
			t.Errorf("executed %v; Java and shims must not run", call.Args)
		}
	}
}

func TestJavaVersion(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"1.8.0_432", "8.0.432"}, {"21.0.5+11", "21.0.5"}, {"17.0.13", "17.0.13"}, {"", ""}} {
		if got := javaVersion(tc.in); got != tc.want {
			t.Errorf("%q = %q, want %q", tc.in, got, tc.want)
		}
	}
}
