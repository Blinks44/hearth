package ops

import "github.com/Blinks44/hearth/locales"

// elevate returns the argv that runs argv as the administrator through
// polkit, whose agent asks for the password. argv[0] must be a full path:
// pkexec runs it with a PATH of its own.
func elevate(argv []string, _ string) []string {
	return append([]string{"pkexec"}, argv...)
}

// adminRefused says why an elevated step failed when the user would not
// give the password, or no polkit agent could ask; "" when that is not
// why.
func adminRefused(code int, _ string) string {
	switch code {
	case 126:
		return locales.AdminCanceled()
	case 127:
		return locales.AdminNoAgent()
	}
	return ""
}

// AdminNote is what to know of a plan that needs the administrator.
func AdminNote() Note {
	return Note{Title: locales.NoteAdmin(), Text: locales.NoteAdminLinux(), Tag: "pkexec"}
}
