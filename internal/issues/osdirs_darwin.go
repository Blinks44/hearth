//go:build darwin

package issues

// osDirs are PATH entries macOS itself adds that are often not there, as
// /etc/paths does /usr/local/bin before anything installs there, and
// /etc/paths.d/10-cryptex and 10-pmk-global theirs: leaving them is right,
// so they are no issue. Each ends in a slash and covers what is under it.
var osDirs = []string{
	"/var/run/com.apple.security.cryptexd/",
	"/pkg/env/global/bin/",
	"/usr/local/bin/",
}
