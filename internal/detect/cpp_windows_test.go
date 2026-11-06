package detect

import (
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestMsvcBase(t *testing.T) {
	got := msvcBase([]byte(`[{"installationPath":"C:\\Program Files (x86)\\Microsoft Visual Studio\\2022\\BuildTools","installationVersion":"17.12.35527.113","displayName":"Visual Studio Build Tools 2022","catalog":{"productDisplayVersion":"17.12.3"}}]`))
	want := inventory.CppBase{ID: "msvc", Name: "Visual Studio Build Tools 2022", Version: "17.12.3", Path: `C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools`}
	if got == nil || *got != want {
		t.Errorf("base %+v, want %+v", got, want)
	}
	if msvcBase([]byte("[]")) != nil {
		t.Error("a base without installations")
	}
}
