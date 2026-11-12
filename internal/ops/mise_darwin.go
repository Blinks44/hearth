package ops

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// homebrewPkgScript downloads the newest Homebrew installer package, as
// its releases on GitHub list it, to the path after it. The script is
// fixed: the path goes in as an argument.
const homebrewPkgScript = `set -e
url=$(curl -fsSL https://api.github.com/repos/Homebrew/brew/releases/latest | sed -n 's/.*"browser_download_url": *"\([^"]*\.pkg\)".*/\1/p' | head -n 1)
test -n "$url"
curl -fL -o "$1" "$url"`

// homebrewSigned checks that the package at the path after it is signed
// with a Developer ID and notarized by Apple. Not the signer's name: the
// maintainer who signs Homebrew's releases changes.
const homebrewSigned = `pkgutil --check-signature "$1" | grep -q 'Notarization: trusted by the Apple notary service'`

// cltScript installs Apple's Command Line Tools when they are not there,
// without the installer's window, as Homebrew's own install script does:
// the flag file makes softwareupdate list them. Homebrew's package does
// not install them itself when it runs from the command line.
const cltScript = `if ! /usr/bin/xcode-select -p >/dev/null 2>&1; then
flag=/tmp/.com.apple.dt.CommandLineTools.installondemand.in-progress
touch "$flag"
label=$(/usr/sbin/softwareupdate -l 2>/dev/null | grep -B 1 -E 'Command Line Tools' | awk -F'*' '/^ *\*/ {print $2}' | sed -e 's/^ *Label: //' -e 's/^ *//' | sort -V | tail -n 1)
test -n "$label" && /usr/sbin/softwareupdate -i "$label"
rm -f "$flag"
/usr/bin/xcode-select -p >/dev/null
fi`

// cltSteps install the Command Line Tools with the administrator's
// password, without Apple's window.
func cltSteps() ([]Step, []Note) {
	return []Step{{Argv: []string{"/bin/sh", "-c", cltScript}, Runs: []string{"softwareupdate", "--install", "Command Line Tools"},
		Note: locales.CltInstallStep(), Admin: true, Timeout: time.Hour}}, []Note{AdminNote()}
}

// installMise returns the steps that install mise with Homebrew, after
// Homebrew itself when it is not there.
func installMise(env sysenv.Env) (MiseSetup, error) {
	brew, steps, notes := homebrew(env)
	s := MiseSetup{Steps: steps, Notes: notes, Base: len(steps)}
	s.Steps = append(s.Steps, Step{Argv: []string{brew, "install", "mise"}})
	s.Mise = filepath.Join(filepath.Dir(brew), "mise")
	return s, nil
}

// homebrew returns where brew is, or will be, and the steps that install
// Homebrew when it is not there: its own installer package, which asks
// for the administrator.
func homebrew(env sysenv.Env) (brew string, steps []Step, notes []Note) {
	if found := sysenv.LookPathAll("brew", env); len(found) > 0 {
		return found[0], nil, nil
	}
	for _, prefix := range brewPrefixes[:2] {
		if p := filepath.Join(prefix, "bin", "brew"); isFile(p) {
			return p, nil, nil
		}
	}
	prefix := "/usr/local"
	if runtime.GOARCH == "arm64" {
		prefix = "/opt/homebrew"
	}
	pkg := filepath.Join(os.TempDir(), "hearth-Homebrew.pkg")
	steps = []Step{
		{Argv: []string{"/bin/sh", "-c", homebrewPkgScript, "sh", pkg}, Note: locales.HomebrewDownload()},
		{Argv: []string{"/bin/sh", "-c", homebrewSigned, "sh", pkg}, Note: locales.HomebrewSignature()},
		// One password: the Command Line Tools, which Homebrew needs, then
		// its package.
		{Argv: []string{"/bin/sh", "-c", cltScript + "\n/usr/sbin/installer -pkg \"$1\" -target /", "sh", pkg},
			Runs: []string{"/usr/sbin/installer", "-pkg", pkg, "-target", "/"}, Note: locales.HomebrewInstallStep(), Admin: true, Timeout: time.Hour},
	}
	notes = []Note{{Title: locales.HomebrewNote(), Text: locales.HomebrewNoteText(), Tag: "Homebrew"}, AdminNote()}
	return filepath.Join(prefix, "bin", "brew"), steps, notes
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
