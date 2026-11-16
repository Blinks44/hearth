package ops

import (
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/locales"
)

// The vendor wizard owns credentials and data initialization.
// Hearth never accepts a database password or passes one on a command line.
func windowsDatabaseSetup(id string, steps []Step, notes []Note) ([]Step, []Note) {
	if id != "mysql" {
		return steps, notes
	}
	steps = slices.Clone(steps)
	guard := databaseScript(`if (Get-CimInstance Win32_Service | Where-Object { $_.PathName -match '(?i)\\(mysqld|mariadbd)\.exe' }) { throw 'An existing MySQL or MariaDB service must be managed separately; no changes made.' }`)
	guard.Runs = []string{locales.DatabaseWindowsGuard()}
	config := databaseScript(`$roots = @(Get-ChildItem (Join-Path $env:ProgramFiles 'MySQL') -Directory -ErrorAction Stop | Where-Object { $_.Name -like 'MySQL Server *' }); $files = @($roots | ForEach-Object { Get-ChildItem $_.FullName -Filter '*config*.exe' -Recurse } | Where-Object { $_.Name -match '(?i)mysql.?configurator' }); if ($files.Count -ne 1) { throw 'Could not uniquely locate MySQL Configurator.' }; $p = Start-Process -FilePath $files[0].FullName -WorkingDirectory $files[0].DirectoryName -Verb RunAs -Wait -PassThru; if ($p.ExitCode -ne 0) { throw 'MySQL Configurator did not complete.' }`)
	config.Admin = true
	config.Runs = []string{locales.DatabaseWindowsSetup()}
	config.Timeout = 30 * time.Minute
	steps = append(steps, config)
	notes = append(slices.Clone(notes), Note{Title: locales.DatabaseWindowsSetup(), Text: locales.DatabaseWindowsSetupText()})
	verify := databaseScript(strings.ReplaceAll(databaseServiceCheck, "EXPECTED_MARIA", "$false"))
	verify.Timeout = time.Minute
	verify.Runs = []string{locales.DatabaseWindowsVerify()}
	steps = append([]Step{guard}, append(steps, verify)...)
	return steps, notes
}

func databaseScript(script string) Step {
	return Step{Argv: []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference = 'Stop'; " + script}, Timeout: time.Minute}
}

// A listener must belong to the running database service and speak MySQL's
// protocol. Access denied also carries a greeting; credentials are not needed.
const databaseServiceCheck = `
$found = $false
$services = @(Get-CimInstance Win32_Service | Where-Object { $_.State -eq 'Running' -and $_.PathName -match '(?i)\\(mysqld|mariadbd)\.exe' })
foreach ($svc in $services) {
 $pids = @([uint32]$svc.ProcessId)
 foreach ($child in @(Get-CimInstance Win32_Process | Where-Object { $_.ParentProcessId -eq $svc.ProcessId -and $_.Name -match '^(mysqld|mariadbd)\.exe$' })) { $pids += [uint32]$child.ProcessId }
 foreach ($listener in @(Get-NetTCPConnection -State Listen -OwningProcess $pids -ErrorAction SilentlyContinue)) {
  $client = New-Object System.Net.Sockets.TcpClient
  try {
   $task = $client.ConnectAsync('127.0.0.1', $listener.LocalPort)
   if (-not $task.Wait(2000)) { continue }
   $stream = $client.GetStream(); $stream.ReadTimeout = 2000
   $bytes = New-Object byte[] 512
   $count = 0; $end = -1
   while ($count -lt $bytes.Length -and $end -lt 0) {
    $n = $stream.Read($bytes, $count, $bytes.Length - $count)
    if ($n -eq 0) { break }
    $count += $n
    if ($count -gt 5) { $end = [Array]::IndexOf($bytes, [byte]0, 5, $count - 5) }
   }
   if ($end -lt 0) { continue }
   $greeting = [Text.Encoding]::ASCII.GetString($bytes, 5, $end - 5)
   if ($bytes[3] -eq 0 -and $bytes[4] -eq 10 -and (($greeting -match 'MariaDB') -eq EXPECTED_MARIA)) {
    Write-Output ('Verified running service: ' + $svc.Name + ', TCP port ' + $listener.LocalPort)
    $found = $true
   }
  } catch { } finally { $client.Dispose() }
 }
}
if (-not $found) { throw 'No running database service with a matching MySQL protocol greeting was found. Complete the vendor configuration wizard and start the service.' }
`
