package detect

import (
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// build-essential counts only installed: a removed package keeps dpkg's
// record.
func TestCppBaseLinux(t *testing.T) {
	argv := []string{"dpkg-query", "-W", "-f=${Status}\t${Version}", "build-essential"}
	for out, want := range map[string]string{
		"install ok installed\t12.10ubuntu1": "12.10ubuntu1",
		"deinstall ok config-files\t12.10":   "",
	} {
		r := &run.Fake{}
		r.Stdout(out, argv...)
		got := cppBase(t.Context(), r, sysenv.Env{})
		if (got == nil) != (want == "") || got != nil && got.Version != want {
			t.Errorf("%q: base %+v", out, got)
		}
	}
	if cppBase(t.Context(), &run.Fake{}, sysenv.Env{}) != nil {
		t.Error("a base without dpkg")
	}
}
