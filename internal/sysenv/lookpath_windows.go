package sysenv

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// defaultPathExt is used when PATHEXT is not set.
const defaultPathExt = ".COM;.EXE;.BAT;.CMD"

// candidates is name itself when it has an executable extension, else name
// with each PATHEXT extension.
func candidates(name string, env Env) []string {
	pathExt := env.Get("PATHEXT")
	if pathExt == "" {
		pathExt = defaultPathExt
	}
	exts := strings.Split(strings.ToLower(pathExt), ";")
	if ext := strings.ToLower(filepath.Ext(name)); ext != "" && slices.Contains(exts, ext) {
		return []string{name}
	}
	var out []string
	for _, ext := range exts {
		if ext != "" {
			out = append(out, name+ext)
		}
	}
	return out
}

func executable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}
