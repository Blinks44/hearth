package ops

import (
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// compilerWait is how long the check waits for the Command Line Tools:
// macOS downloads them in its installer's window.
const compilerWait = time.Hour

// compilerPlan asks macOS to install the Command Line Tools: xcode-select
// opens Apple's installer and returns.
func compilerPlan([]inventory.PackageManager) (Plan, bool) {
	return Plan{
		Why:     locales.CompilerWhyMac(),
		Steps:   []Step{{Argv: []string{"xcode-select", "--install"}, Note: locales.CompilerStepMac()}},
		Notes:   []Note{{Title: locales.CompilerNoteMac(), Text: locales.CompilerNoteMacText()}},
		Changes: []string{"+ Command Line Tools"},
	}, true
}
