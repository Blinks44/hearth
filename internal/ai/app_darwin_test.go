package ai

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Cursor's editor in the user's Applications is found with its version.
func TestCursorApp(t *testing.T) {
	applications = t.TempDir()
	t.Cleanup(func() { applications = "/Applications" })
	home := fixtureHome(t)
	cursor := byID(Detect(t.Context(), &run.Fake{}, sysenv.Env{"HOME=" + home}, nil))["cursor"]
	if want := filepath.Join(home, "Applications", "Cursor.app"); cursor.App != want || cursor.AppVersion != "1.7.2" || !cursor.Found() {
		t.Errorf("app %q %q, want %q 1.7.2", cursor.App, cursor.AppVersion, want)
	}
}
