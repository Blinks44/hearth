package detect

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// xcodes are none: Xcode is macOS's.
func xcodes() []inventory.XcodeApp { return nil }

func mobileDeveloperDir() string { return "" }

func simRuntimes() []inventory.SimRuntime { return nil }

// androidDefault is where Android Studio puts the SDK.
func androidDefault(_ sysenv.Env, home string) string {
	return filepath.Join(home, "Android", "Sdk")
}

// androidStudios are where Android Studio is unpacked or installed: its
// archive's usual places, and its snap.
func androidStudios(_ sysenv.Env, home string) []string {
	return []string{"/opt/android-studio", filepath.Join(home, "android-studio"), "/snap/android-studio/current"}
}
