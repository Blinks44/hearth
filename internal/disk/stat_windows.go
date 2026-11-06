package disk

import (
	"io/fs"

	"golang.org/x/sys/windows"
)

// stat returns a file's size and, when hard links lead to it, as pnpm
// makes them, its identity. The link count needs a handle: the listing of
// a folder does not give it.
func stat(path string, info fs.FileInfo) (int64, fileID, bool) {
	if !info.Mode().IsRegular() {
		return info.Size(), fileID{}, false
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return info.Size(), fileID{}, false
	}
	// No access asked: only the file's attributes are read.
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return info.Size(), fileID{}, false
	}
	defer windows.CloseHandle(h) //nolint:errcheck // read only
	var d windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &d); err != nil || d.NumberOfLinks < 2 {
		return info.Size(), fileID{}, false
	}
	return info.Size(), fileID{uint64(d.VolumeSerialNumber), uint64(d.FileIndexHigh)<<32 | uint64(d.FileIndexLow)}, true
}
