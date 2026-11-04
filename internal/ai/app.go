package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// appVersion is the version an Electron app's package.json at path says;
// "" when it says none.
func appVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct{ Version string }
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	return pkg.Version
}

// firstApp is the first of the app paths that is there, with the version
// its package.json, at the path pkg gives, says.
func firstApp(paths []string, pkg func(app string) string) (path, version string) {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, appVersion(pkg(p))
		}
	}
	return "", ""
}

func join(dir string, elem ...string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(append([]string{dir}, elem...)...)
}
