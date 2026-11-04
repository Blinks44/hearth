//go:build !windows

package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestJavaSDKMAN(t *testing.T) {
	dir := root(t)
	sdk := filepath.Join(dir, "sdk")
	jdk := filepath.Join(sdk, "candidates", "java", "21.0.5-tem")
	path := fakeJava(t, jdk, "21.0.5")
	symlink(t, jdk, filepath.Join(sdk, "candidates", "java", "current"))
	for _, auto := range []string{"true", "false"} {
		writeFile(t, filepath.Join(sdk, "etc", "config"), "sdkman_auto_env="+auto)
		env := sysenv.Env{"HOME=" + dir, "PATH=" + filepath.Dir(path), "SDKMAN_DIR=" + sdk}
		st, err := Java(t.Context(), &run.Fake{}, env, sysenv.Env{})
		if err != nil {
			t.Fatal(err)
		}
		in := st.Installations[0]
		if !in.Default || in.Source.Manager != "sdkman" || in.Source.Version != "21.0.5-tem" {
			t.Fatalf("copy %+v", in)
		}
		if len(st.Managers) != 1 || st.Managers[0].SwitchOnCD != (auto == "true") {
			t.Fatalf("managers %+v", st.Managers)
		}
	}
}
