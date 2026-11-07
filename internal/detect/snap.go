package detect

import (
	"os"
	"path/filepath"
	"strings"
)

// snapBin is where snapd puts the commands of Linux's snaps.
var snapBin = "/snap/bin"

// snapRoot is the folder of the snap whose command path is, as
// /snap/google-cloud-cli/current for /snap/bin/gcloud; "" for other paths.
// Every snap command links to /usr/bin/snap, the snap's name is the one of
// the link before it: gcloud -> google-cloud-cli.gcloud -> /usr/bin/snap.
func snapRoot(path string) string {
	if filepath.Dir(path) != snapBin {
		return ""
	}
	name := filepath.Base(path)
	for range 2 {
		target, err := os.Readlink(filepath.Join(snapBin, name))
		if err != nil {
			return ""
		}
		if filepath.Base(target) == "snap" && filepath.IsAbs(target) {
			snap, _, _ := strings.Cut(name, ".")
			return filepath.Join(filepath.Dir(snapBin), snap, "current")
		}
		name = target
	}
	return ""
}
