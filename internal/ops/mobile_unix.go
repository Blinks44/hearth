//go:build !windows

package ops

const sdkmanagerExe = "sdkmanager"

func sdkmanagerUninstallStep(sdkmgr, sdkRoot, pkg string) Step {
	argv := []string{sdkmgr}
	if sdkRoot != "" {
		argv = append(argv, "--sdk_root="+sdkRoot)
	}
	argv = append(argv, "--uninstall", pkg)
	return Step{Argv: argv, Timeout: cleanupTimeout}
}
