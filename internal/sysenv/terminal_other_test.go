//go:build !windows && !darwin

package sysenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEtcEnvironmentPath(t *testing.T) {
	f := filepath.Join(t.TempDir(), "environment")
	body := "# comment\nLANG=C.UTF-8\nPATH=\"/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/bin\"\n"
	if err := os.WriteFile(f, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	want := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/bin"
	if got := etcEnvironmentPath(f); got != want {
		t.Fatalf("etcEnvironmentPath = %q, want %q", got, want)
	}
	if got := etcEnvironmentPath(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Fatalf("missing file returned %q, want empty", got)
	}
}
