package ops

import (
	"strings"

	"github.com/Blinks44/hearth/locales"
)

// adminScript runs its arguments after the first, quoted, as root once
// the user gives the password in macOS's own dialog, which says the first
// argument. The script is fixed: the command goes in as arguments.
const adminScript = `on run argv
	set cmd to ""
	repeat with i from 2 to count of argv
		set cmd to cmd & quoted form of (item i of argv as text) & " "
	end repeat
	return do shell script cmd & "2>&1" with prompt (item 1 of argv as text) with administrator privileges
end run`

// elevate returns the argv that runs argv as the administrator, asking
// for the password; why says what for. argv[0] must be a full path: the
// command runs with root's PATH.
func elevate(argv []string, why string) []string {
	return append([]string{"/usr/bin/osascript", "-e", adminScript, why}, argv...)
}

// adminRefused says why an elevated step failed when the user would not
// give the password, or "" when that is not why.
func adminRefused(code int, output string) string {
	if code != 0 && strings.Contains(output, "(-128)") {
		return locales.AdminCanceled()
	}
	return ""
}

// AdminNote is what to know of a plan that needs the administrator.
func AdminNote() Note {
	return Note{Title: locales.NoteAdmin(), Text: locales.NoteAdminMac(), Tag: "sudo"}
}
