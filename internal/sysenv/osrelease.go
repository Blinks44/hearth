package sysenv

import (
	"strconv"
	"strings"
)

// parseOSRelease takes NAME and VERSION_ID from an os-release file.
func parseOSRelease(data []byte) OS {
	o := OS{Name: "Linux"}
	for line := range strings.Lines(string(data)) {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		value = unquote(value)
		switch key {
		case "NAME":
			if value != "" {
				o.Name = value
			}
		case "VERSION_ID":
			o.Version = value
		}
	}
	return o
}

// unquote strips the quotes around an os-release value. Double quotes
// allow backslash escapes; when they are not Go's too, the quotes are just
// stripped.
func unquote(v string) string {
	if len(v) < 2 || (v[0] != '"' && v[0] != '\'') || v[len(v)-1] != v[0] {
		return v
	}
	if u, err := strconv.Unquote(v); err == nil && v[0] == '"' {
		return u
	}
	return v[1 : len(v)-1]
}
