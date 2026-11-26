//go:build !darwin

package issues

import "testing"

func TestNonDarwinDoesNotSuppressDarwinDirs(t *testing.T) {
	if OSDir("/var/run/com.apple.security.cryptexd/codex.system/bootstrap/usr/bin") {
		t.Fatal("macOS cryptexd dir should not be suppressed on non-darwin platforms")
	}
	if OSDir("/usr/local/bin") {
		t.Fatal("/usr/local/bin should not be treated as macOS-default on non-darwin platforms")
	}
}
