//go:build darwin

package sysenv

// loginPath is macOS's _PATH_DEFPATH, the baseline PATH login(1) and launchd
// give a new login shell before /etc/zprofile or /etc/profile runs path_helper.
func loginPath() string {
	return "/usr/bin:/bin:/usr/sbin:/sbin"
}
