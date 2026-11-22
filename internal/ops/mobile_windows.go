package ops

const sdkmanagerExe = "sdkmanager.bat"

// sdkmanagerUninstallStep runs sdkmanager.bat via PowerShell with inputs in
// environment variables so cmd.exe does not split the semicolon-delimited
// package name when CreateProcess launches a .bat file directly.
func sdkmanagerUninstallStep(sdkmgr, sdkRoot, pkg string) Step {
	script := "$a = @(); if ($env:HEARTH_SDKROOT) { $a += '--sdk_root=' + $env:HEARTH_SDKROOT }; $a += '--uninstall'; $a += $env:HEARTH_SDKPKG; & $env:HEARTH_SDKMGR @a; exit $LASTEXITCODE"
	return Step{
		Argv:    []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script},
		Env:     []string{"HEARTH_SDKMGR=" + sdkmgr, "HEARTH_SDKROOT=" + sdkRoot, "HEARTH_SDKPKG=" + pkg},
		Timeout: cleanupTimeout,
	}
}
