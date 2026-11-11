package ops

import (
	"os"
	"path/filepath"
	"runtime"

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
		{Argv: []string{"/usr/sbin/installer", "-pkg", pkg, "-target", "/"}, Admin: true},
	}
	notes = []Note{{Title: locales.HomebrewNote(), Text: locales.HomebrewNoteText(), Tag: "Homebrew"}, AdminNote()}
	return filepath.Join(prefix, "bin", "brew"), steps, notes
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
