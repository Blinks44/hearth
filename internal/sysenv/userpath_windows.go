package sysenv

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// UserPath returns the user's Path from the registry as it is written,
// %VARIABLES% unexpanded; "" when the user has none.
func UserPath() (string, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open HKCU\\Environment: %w", err)
	}
	defer k.Close() //nolint:errcheck // a read-only key: nothing to lose on close
	v, _, err := k.GetStringValue("Path")
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read HKCU\\Environment\\Path: %w", err)
	}
	return v, nil
}

// SetUserPath writes the user's Path, as an expandable string, as Windows'
// own editor does, and tells the running apps, Explorer among them, so
// that terminals started after see it.
func SetUserPath(value string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKCU\\Environment: %w", err)
	}
	err = k.SetExpandStringValue("Path", value)
	if cerr := k.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("write HKCU\\Environment\\Path: %w", err)
	}
	return broadcastEnvironment()
}

// UserVar returns the user's environment variable name from the registry,
// ok false when the user has none.
func UserVar(name string) (value string, ok bool, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return "", false, fmt.Errorf("open HKCU\\Environment: %w", err)
	}
	defer k.Close() //nolint:errcheck // a read-only key: nothing to lose on close
	v, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read HKCU\\Environment\\%s: %w", name, err)
	}
	return v, true, nil
}

// SetUserVar writes the user's environment variable name as a plain
// string and tells the running apps, as SetUserPath does.
func SetUserVar(name, value string) error {
	return editUserVars(name, func(k registry.Key) error { return k.SetStringValue(name, value) })
}

// DeleteUserVar removes the user's environment variable name and tells the
// running apps.
func DeleteUserVar(name string) error {
	return editUserVars(name, func(k registry.Key) error { return k.DeleteValue(name) })
}

// editUserVars changes HKCU\Environment by edit, then broadcasts it.
func editUserVars(name string, edit func(registry.Key) error) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKCU\\Environment: %w", err)
	}
	err = edit(k)
	if cerr := k.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("write HKCU\\Environment\\%s: %w", name, err)
	}
	return broadcastEnvironment()
}

// SystemPath returns the Path of the computer, before the user's in every
// new process, %VARIABLES% expanded.
func SystemPath() (string, error) {
	return registryPath(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`)
}

// ExpandPath expands the %VARIABLES% of a Path entry, as Windows does.
func ExpandPath(dir string) string {
	if v, err := registry.ExpandString(dir); err == nil {
		return v
	}
	return dir
}

var sendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

// broadcastEnvironment sends WM_SETTINGCHANGE for "Environment" to the
// top windows, waiting at most 5 seconds for each, skipping those that
// hang.
func broadcastEnvironment() error {
	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001a
		smtoAbortIfHung = 0x0002
	)
	param, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return err
	}
	var result uintptr
	ok, _, err := sendMessageTimeout.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(param)),
		smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
	if ok == 0 {
		return fmt.Errorf("tell the apps the environment changed: %w", err)
	}
	return nil
}
