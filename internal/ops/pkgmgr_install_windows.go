package ops

import (
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// scoopSetupScript downloads Scoop's official user installer script to a
// temporary file and executes it without requiring administrator elevation.
const scoopSetupScript = `$ErrorActionPreference = 'Stop'
$f = Join-Path $env:TEMP 'hearth-install-scoop.ps1'
Invoke-WebRequest -UseBasicParsing -Uri 'https://get.scoop.sh' -OutFile $f
try {
    & $f
    if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
} finally {
    Remove-Item -Force -ErrorAction SilentlyContinue $f
}`

// PackageManagerInstallPlan returns a plan to install a safe, supported
// package manager on Windows (Scoop via its official user-level installer).
func PackageManagerInstallPlan(id string, _ []inventory.PackageManager, _ sysenv.Env) (Plan, bool) {
	if id != "scoop" {
		return Plan{}, false
	}
	return Plan{
		Title:   locales.PkgManagerInstallTitle("Scoop"),
		Why:     locales.PkgManagerInstallWhy("Scoop"),
		Changes: []string{locales.PkgManagerInstallChange("Scoop", "Windows")},
		Steps: []Step{{
			Argv: []string{"powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", scoopSetupScript},
			Note: locales.PkgManagerInstallScoopStep(),
		}},
		RestartTerminal: true,
	}, true
}
