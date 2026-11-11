//go:build !windows

package ops

// The commands that change skills folders, as a user would type them: a
// symlink leads to a skill's folder.

func mkdirCommand(dir string) []string { return []string{"mkdir", "-p", dir} }

func linkCommand(target, link string) []string { return []string{"ln", "-s", target, link} }

// unlinkCommand takes away a link to a folder, not what it leads to: rm
// does not follow a symlink.
func unlinkCommand(link string) []string { return []string{"rm", link} }

func moveCommand(from, to string) []string { return []string{"mv", from, to} }
