package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDotnetWindows(t *testing.T) {
	home := root(t)
	root := filepath.Join(home, "Program Files", "dotnet")
	exe := filepath.Join(root, "dotnet.exe")
	writeFile(t, exe, "")
	env := sysenv.Env{"USERPROFILE=" + home, "PATH=" + root, "ProgramFiles=" + filepath.Dir(root)}
	r := &run.Fake{}
	r.Stdout("9.0.102 ["+filepath.Join(root, "sdk")+"]", exe, "--list-sdks")
	r.Stdout("Microsoft.WindowsDesktop.App 9.0.1 ["+filepath.Join(root, "shared", "Microsoft.WindowsDesktop.App")+"]", exe, "--list-runtimes")
	st, err := Dotnet(t.Context(), r, env, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installations) != 1 {
		t.Fatalf("%+v", st)
	}
	in := st.Installations[0]
	if in.Version != "9.0.102" || in.OnPath != 1 || in.InGUI != 1 || len(in.Dotnet.Runtimes) != 1 {
		t.Fatalf("%+v", in)
	}
	if in.Dotnet.SDKs[0].Path != filepath.Join(root, "sdk", "9.0.102") {
		t.Fatalf("%+v", in.Dotnet.SDKs)
	}
}
