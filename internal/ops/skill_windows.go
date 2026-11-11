package ops

// The commands that change skills folders, as a user would type them in
// cmd: a junction leads to a skill's folder. Unlike a symlink it needs no
// right a standard user lacks, and the agents read through it the same.

func mkdirCommand(dir string) []string { return []string{"cmd.exe", "/c", "mkdir", dir} }

func linkCommand(target, link string) []string {
	return []string{"cmd.exe", "/c", "mklink", "/J", link, target}
}

// unlinkCommand takes away a junction or a symlink to a folder, not what
// it leads to: rmdir without /s removes the link alone.
func unlinkCommand(link string) []string { return []string{"cmd.exe", "/c", "rmdir", link} }

func moveCommand(from, to string) []string { return []string{"cmd.exe", "/c", "move", from, to} }
