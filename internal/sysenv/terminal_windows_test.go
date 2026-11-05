package sysenv

import (
	"os"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

// A new terminal's environment comes from the registry and the user's
// profile, not the system's defaults: the user's home and name, a Path.
func TestTerminalWindows(t *testing.T) {
	env, err := Terminal(t.Context(), run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	if env.Get("Path") == "" || !strings.EqualFold(env.Get("USERPROFILE"), os.Getenv("USERPROFILE")) {
		t.Errorf("Path %q, USERPROFILE %q", env.Get("Path"), env.Get("USERPROFILE"))
	}
	if strings.EqualFold(env.Get("USERNAME"), "SYSTEM") && !strings.EqualFold(os.Getenv("USERNAME"), "SYSTEM") {
		t.Errorf("USERNAME is the system's default")
	}
}
