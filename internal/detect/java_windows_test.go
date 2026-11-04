package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestJavaWindowsInstaller(t *testing.T) {
	dir := root(t)
	path := fakeJava(t, filepath.Join(dir, "Eclipse Adoptium", "jdk-21.0.5"), "21.0.5")
	st, err := Java(t.Context(), &run.Fake{}, sysenv.Env{"ProgramFiles=" + dir, "PATH=", "PATHEXT=.EXE"}, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installations) != 1 || st.Installations[0].Path != path || st.Installations[0].Version != "21.0.5" {
		t.Fatalf("state %+v", st)
	}
}
