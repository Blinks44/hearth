package sysenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A junction leads to its target's path, as Scoop's current does.
func TestRealPathJunction(t *testing.T) {
	dir, err := RealPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	version := filepath.Join(dir, "8.5.12-1")
	if err := os.MkdirAll(version, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(version, "php.exe"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(dir, "current")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", current, version).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v: %s", err, out)
	}
	got, err := RealPath(filepath.Join(current, "php.exe"))
	if err != nil || got != filepath.Join(version, "php.exe") {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := RealPath(filepath.Join(dir, "missing.exe")); err == nil {
		t.Error("a missing file has a real path")
	}
}
