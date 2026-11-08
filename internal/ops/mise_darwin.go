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
// Homebrew itself when it is not there: its own installer package, which
// asks for the administrator.
func installMise(env sysenv.Env) (MiseSetup, error) {
	var s MiseSetup
	brew := ""
	if found := sysenv.LookPathAll("brew", env); len(found) > 0 {
		brew = found[0]
	}
	for _, prefix := range brewPrefixes[:2] {
		if p := filepath.Join(prefix, "bin", "brew"); brew == "" && isFile(p) {
			brew = p
		}
	}
	if brew == "" {
		prefix := "/usr/local"
		if runtime.GOARCH == "arm64" {
			prefix = "/opt/homebrew"
		}
		pkg := filepath.Join(os.TempDir(), "hearth-Homebrew.pkg")
		s.Steps = []Step{
			{Argv: []string{"/bin/sh", "-c", homebrewPkgScript, "sh", pkg}, Note: locales.HomebrewDownload()},
			{Argv: []string{"/bin/sh", "-c", homebrewSigned, "sh", pkg}, Note: locales.HomebrewSignature()},
			{Argv: []string{"/usr/sbin/installer", "-pkg", pkg, "-target", "/"}, Admin: true},
		}
		s.Notes = []Note{{Title: locales.HomebrewNote(), Text: locales.HomebrewNoteText(), Tag: "Homebrew"}, AdminNote()}
		brew = filepath.Join(prefix, "bin", "brew")
	}
	s.Steps = append(s.Steps, Step{Argv: []string{brew, "install", "mise"}})
	s.Mise = filepath.Join(filepath.Dir(brew), "mise")
	return s, nil
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
