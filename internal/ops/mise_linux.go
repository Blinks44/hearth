package ops

import (
	"path/filepath"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// miseRunScript runs mise's official installer, which puts mise in
// ~/.local/bin without the administrator. The script is fixed.
const miseRunScript = `set -e
if command -v curl >/dev/null 2>&1; then curl -fsSL https://mise.run | sh; else wget -qO- https://mise.run | sh; fi`

// installMise returns the steps that install mise: the package of the
// system where its own repositories have it, Arch's and Alpine's, else
// mise's official installer.
func installMise(env sysenv.Env) (MiseSetup, error) {
	for _, pm := range []struct {
		name string
		argv []string
	}{
		{"pacman", []string{"pacman", "-S", "--needed", "--noconfirm", "mise"}},
		{"apk", []string{"apk", "add", "mise"}},
	} {
		if len(sysenv.LookPathAll(pm.name, env)) > 0 {
			return MiseSetup{Steps: []Step{{Argv: pm.argv, Admin: true}}, Notes: []Note{AdminNote()}, Mise: "/usr/bin/mise"}, nil
		}
	}
	return MiseSetup{
		Steps: []Step{{Argv: []string{"/bin/sh", "-c", miseRunScript}, Note: locales.MiseRunInstaller()}},
		Mise:  filepath.Join(env.Get("HOME"), ".local", "bin", "mise"),
	}, nil
}
