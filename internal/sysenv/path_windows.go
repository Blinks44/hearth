package sysenv

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/Blinks44/hearth/internal/inventory"
)

func pathKey(dir string) string { return strings.ToLower(filepath.Clean(dir)) }

// pathEntryUsed reports that commands are looked up in dir: cmd, PowerShell
// and Go's exec.LookPath skip empty entries, as the trailing ";" of a fresh
// Windows user Path makes, so they do not mean the working directory.
func pathEntryUsed(dir string) bool { return dir != "" }

func pathSources(Env) map[string][]inventory.PathSource {
	out := map[string][]inventory.PathSource{}
	for _, src := range []struct {
		root       registry.Key
		key, label string
	}{
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`, `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment\Path`},
		{registry.CURRENT_USER, `Environment`, `HKCU\Environment\Path`},
	} {
		value, err := registryPath(src.root, src.key)
		if err != nil {
			continue
		}
		for _, dir := range filepath.SplitList(value) {
			key := pathKey(dir)
			out[key] = append(out[key], inventory.PathSource{File: src.label})
		}
	}
	return out
}

// registryPath reads the Path value of a key, expanding %VAR% references.
func registryPath(root registry.Key, key string) (string, error) {
	k, err := registry.OpenKey(root, key, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", key, err)
	}
	defer k.Close() //nolint:errcheck // a read-only key: nothing to lose on close
	v, typ, err := k.GetStringValue("Path")
	if err != nil {
		return "", fmt.Errorf("read %s\\Path: %w", key, err)
	}
	if typ == registry.EXPAND_SZ {
		if v, err = registry.ExpandString(v); err != nil {
			return "", fmt.Errorf("expand %s\\Path: %w", key, err)
		}
	}
	return v, nil
}
