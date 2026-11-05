package sysenv

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// volumeNameDOS asks GetFinalPathNameByHandle for a path with a drive
// letter; x/sys does not name it.
const volumeNameDOS = 0x0

// RealPath is path with symlinks and junctions resolved, the path Windows
// opens. filepath.EvalSymlinks leaves junctions since Go 1.23, and Scoop's
// current and mise's version aliases are junctions.
func RealPath(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("real path of %s: %w", path, err)
	}
	// Backup semantics opens directories too; no access is asked for.
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", fmt.Errorf("real path of %s: %w", path, err)
	}
	defer windows.CloseHandle(h) //nolint:errcheck // a handle opened for reading only
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), volumeNameDOS)
		if err != nil {
			return "", fmt.Errorf("real path of %s: %w", path, err)
		}
		// Too small a buffer gets the size it needs instead.
		if int(n) < len(buf) {
			s := windows.UTF16ToString(buf[:n])
			if rest, ok := strings.CutPrefix(s, `\\?\UNC\`); ok {
				return `\\` + rest, nil
			}
			return strings.TrimPrefix(s, `\\?\`), nil
		}
		buf = make([]uint16, n)
	}
}
