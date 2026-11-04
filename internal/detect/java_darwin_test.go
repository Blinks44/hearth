package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestJavaMacLauncher(t *testing.T) {
	dir := root(t)
	jdk := filepath.Join(dir, "JDK With Spaces", "Contents", "Home")
	fakeJava(t, jdk, "21.0.5")
	for _, tc := range []struct{ installed, home bool }{{false, false}, {true, false}, {false, true}} {
		installed := tc.installed
		r := &run.Fake{}
		if installed {
			r.Stdout(jdk+"\n", "/usr/libexec/java_home", "-V")
		}
		env := sysenv.Env{"HOME=" + dir, "PATH=/usr/bin"}
		if tc.home {
			env = append(env, "JAVA_HOME="+jdk)
		}
		st, err := Java(t.Context(), r, env, sysenv.Env{})
		if err != nil {
			t.Fatal(err)
		}
		i := inventory.TerminalCopy(st.Installations)
		if (installed || tc.home) && (i < 0 || st.Installations[i].Version != "21.0.5") {
			t.Fatalf("launcher not resolved: %+v", st)
		}
		if !installed && !tc.home && (i >= 0 || !st.Installations[0].Stub) {
			t.Fatalf("launcher not a stub: %+v", st)
		}
		for _, call := range r.Calls() {
			if call.Args[0] != "/usr/libexec/java_home" {
				t.Fatalf("ran %v", call.Args)
			}
		}
	}
}
