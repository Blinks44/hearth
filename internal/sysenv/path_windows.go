package sysenv

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/Blinks44/hearth/internal/inventory"
)

func pathKey(dir string) string { return strings.ToLower(filepath.Clean(dir)) }

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
