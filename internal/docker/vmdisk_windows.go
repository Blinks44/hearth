package docker

import (
	"io/fs"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func desktopDisks(env sysenv.Env, provider string) []string {
	if provider != "Docker Desktop" {
		return nil
	}
	local, shared := env.Get("LOCALAPPDATA"), env.Get("ProgramData")
	return []string{
		filepath.Join(local, "Docker", "wsl", "disk", "docker_data.vhdx"),
		filepath.Join(local, "Docker", "wsl", "data", "ext4.vhdx"),
		filepath.Join(shared, "DockerDesktop", "vm-data", "DockerDesktop.vhdx"),
		filepath.Join(local, "Docker", "vm-data", "DockerDesktop.vhdx"),
	}
}

func allocatedSize(path string, _ fs.FileInfo) (int64, bool) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, false
	}
	h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return 0, false
	}
	defer windows.CloseHandle(h) //nolint:errcheck // attribute query only
	// FILE_COMPRESSION_INFO reports bytes on disk, including sparse files.
	var data struct {
		Size int64
		_    [8]byte
	}
	err = windows.GetFileInformationByHandleEx(h, windows.FileCompressionInfo, (*byte)(unsafe.Pointer(&data)), uint32(unsafe.Sizeof(data)))
	return data.Size, err == nil
}
