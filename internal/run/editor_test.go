package run

import (
	"runtime"
	"slices"
	"testing"
)

func TestEditorLauncher(t *testing.T) {
	if got := editorLauncher("", "/usr/local/bin/code", "/tmp/a b.yaml"); !slices.Equal(got, []string{"/usr/local/bin/code", "/tmp/a b.yaml"}) {
		t.Errorf("command launcher = %q", got)
	}
	if got := editorLauncher("", "", "/tmp/a"); got != nil {
		t.Errorf("no editor = %q", got)
	}
	if runtime.GOOS == "darwin" {
		if got := editorLauncher("/Applications/Zed.app", "zed", "/tmp/a"); !slices.Equal(got, []string{"/usr/bin/open", "-a", "/Applications/Zed.app", "/tmp/a"}) {
			t.Errorf("app launcher = %q", got)
		}
	}
}
