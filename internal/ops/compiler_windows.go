package ops

import (
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// compilerWait is zero: winget waits for the installer, as --wait tells it.
const compilerWait = 0

// buildToolsOverride are the installer's own arguments: the C++ workload
// with what it recommends, MSVC and the Windows SDK, with progress shown
// and nothing asked.
const buildToolsOverride = "--wait --passive --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"

// compilerPlan installs Visual Studio Build Tools through WinGet when it
// is here.
func compilerPlan(managers []inventory.PackageManager) (Plan, bool) {
	for _, m := range managers {
		if m.ID != "winget" {
			continue
		}
		return Plan{
			Why: locales.CompilerWhyWindows(),
			Steps: []Step{{Argv: []string{"winget", "install", "--id", "Microsoft.VisualStudio.BuildTools", "--exact", "--source", "winget",
				"--override", buildToolsOverride, "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity"}}},
			Notes:   []Note{{Title: locales.NoteAdmin(), Text: locales.CompilerNoteWindows(), Tag: "UAC"}},
			Changes: []string{"+ Visual Studio Build Tools · WinGet"},
		}, true
	}
	return Plan{}, false
}
