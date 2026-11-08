package ops

import "github.com/Blinks44/hearth/locales"

// elevate returns argv as it is: on Windows the installer or winget asks
// for the rights itself, through UAC, and its output still comes back.
func elevate(argv []string, _ string) []string { return argv }

// adminRefused says that the user did not allow a step in UAC's window,
// for the code a step started through it exits with, 1223
// (ERROR_CANCELLED); "" otherwise: the step fails as the program says.
func adminRefused(code int, _ string) string {
	if code == 1223 {
		return locales.AdminDenied()
	}
	return ""
}

// AdminNote is what to know of a plan that needs the administrator.
func AdminNote() Note {
	return Note{Title: locales.NoteAdmin(), Text: locales.NoteAdminWindows(), Tag: "UAC"}
}
