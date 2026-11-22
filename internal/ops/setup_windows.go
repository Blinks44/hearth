package ops

import (
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// composerSetupScript downloads Composer's official Windows installer
// (Inno Setup) and runs it silently for the current user, adding
// Composer to the user's PATH without requiring elevation.
const composerSetupScript = `$ErrorActionPreference = 'Stop'
$f = Join-Path $env:TEMP 'Composer-Setup.exe'
Invoke-WebRequest -UseBasicParsing -Uri 'https://getcomposer.org/Composer-Setup.exe' -OutFile $f
try {
    $p = Start-Process -FilePath $f -ArgumentList '/VERYSILENT','/SUPPRESSMSGBOXES','/CURRENTUSER','/NORESTART' -Wait -PassThru
    if ($p.ExitCode -ne 0) { exit $p.ExitCode }
} finally {
    Remove-Item -Force -ErrorAction SilentlyContinue $f
}`

// installerSteps are the steps that install a tool with its maker's
// Windows installer when WinGet has no package for it.
func installerSteps(id string, _ sysenv.Env) ([]Step, bool) {
	if id == "composer" {
		return []Step{{
			Argv: []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", composerSetupScript},
			Note: locales.SetupComposerInstaller(),
		}}, true
	}
	return nil, false
}

// homebrew: Hearth installs Homebrew on macOS alone.
func homebrew(sysenv.Env) (string, []Step, []Note) { return "", nil, nil }

// cltSteps: the Command Line Tools are macOS's.
func cltSteps() ([]Step, []Note) { return nil, nil }
