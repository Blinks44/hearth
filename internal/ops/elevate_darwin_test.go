package ops

import (
	"os/exec"
	"strings"
	"testing"
)

// The script quotes every argument as the shell takes it: spaces, quotes
// and dollars stay text. It runs here without asking for the password.
func TestAdminScriptQuotes(t *testing.T) {
	script := strings.Replace(adminScript, " with administrator privileges", "", 1)
	script = strings.Replace(script, " with prompt (item 1 of argv as text)", "", 1)
	args := []string{"/usr/bin/printf", "%s|", "a b", `it's "$HOME"`, "x;y"}
	out, err := exec.Command("/usr/bin/osascript", append([]string{"-e", script, "why"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got, want := strings.TrimSpace(string(out)), `a b|it's "$HOME"|x;y|`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestAdminRefusedMac(t *testing.T) {
	if adminRefused(1, "execution error: User canceled. (-128)") == "" {
		t.Error("a canceled password prompt is not told")
	}
	if adminRefused(1, "Error: no network") != "" {
		t.Error("another failure is told as a canceled prompt")
	}
}
