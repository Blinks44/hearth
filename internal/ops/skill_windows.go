package ops

import "golang.org/x/sys/windows"

// The commands that change skills folders, as a user would type them in
// cmd: a junction leads to a skill's folder. Unlike a symlink it needs no
// right a standard user lacks, and the agents read through it the same.

func mkdirCommand(dir string) []string { return []string{"cmd.exe", "/c", "mkdir", dir} }

func linkCommand(target, link string) []string {
	return []string{"cmd.exe", "/c", "mklink", "/J", link, target}
}

// unlinkCommand takes away a junction or a symlink, not what it leads to:
// rmdir without /s removes a directory symlink or junction (FILE_ATTRIBUTE_DIRECTORY),
// while del /F removes a file symlink (including broken symlinks to missing paths).
func unlinkCommand(link string) []string {
	if p, err := windows.UTF16PtrFromString(link); err == nil {
		if attrs, err := windows.GetFileAttributes(p); err == nil && attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return []string{"cmd.exe", "/c", "del", "/F", link}
		}
	}
	return []string{"cmd.exe", "/c", "rmdir", link}
}

func moveCommand(from, to string) []string { return []string{"cmd.exe", "/c", "move", from, to} }
