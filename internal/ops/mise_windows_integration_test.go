//go:build integration

package ops

import (
	"os"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// TestWinGetMiseReal checks, once WinGet installed mise, that it is where
// SetUpMise says it will be when the terminal does not find it yet.
func TestWinGetMiseReal(t *testing.T) {
	env, err := sysenv.Terminal(t.Context(), run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := SetUpMise(append(env, `PATH=C:\Windows\System32`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Mise); err != nil {
		t.Fatalf("mise is not at %s: %v", s.Mise, err)
	}
}
