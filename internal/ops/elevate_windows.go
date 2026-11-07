package ops

import "github.com/Blinks44/hearth/locales"

// elevate returns argv as it is: on Windows the installer or winget asks
// for the rights itself, through UAC, and its output still comes back.
func elevate(argv []string, _ string) []string { return argv }

// adminRefused is "" on Windows: a step the user did not allow fails as
// the program says.
func adminRefused(int, string) string { return "" }

// AdminNote is what to know of a plan that needs the administrator.
func AdminNote() Note {
	return Note{Title: locales.NoteAdmin(), Text: locales.NoteAdminWindows(), Tag: "UAC"}
}
