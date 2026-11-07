package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// xcodes are none: Xcode is macOS's.
func xcodes() []inventory.XcodeApp { return nil }

func simRuntimes() []inventory.SimRuntime { return nil }

// androidDefault is where Android Studio puts the SDK:
// %LocalAppData%\Android\Sdk.
func androidDefault(env sysenv.Env, _ string) string {
	if local := env.Get("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "Android", "Sdk")
	}
	return ""
}

func androidStudios(env sysenv.Env, _ string) []string {
	return []string{filepath.Join(or(env.Get("ProgramFiles"), `C:\Program Files`), "Android", "Android Studio")}
}
