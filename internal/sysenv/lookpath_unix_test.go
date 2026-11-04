//go:build !windows

package sysenv

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLookPathAll(t *testing.T) {
	root := t.TempDir()
	dir := func(name string) string {
		d := filepath.Join(root, name)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		return d
	}
	file := func(path string, mode os.FileMode) {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), mode); err != nil {
			t.Fatal(err)
		}
	}
	link := func(target, path string) {
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}

	a, b, c, d, e := dir("a"), dir("b"), dir("c"), dir("d"), dir("e")
	file(filepath.Join(a, "tool"), 0o755)
	file(filepath.Join(b, "tool"), 0o755)
	file(filepath.Join(c, "tool"), 0o644) // not executable
	link(filepath.Join(a, "tool"), filepath.Join(d, "tool"))
	link(filepath.Join(root, "missing"), filepath.Join(e, "tool")) // broken
	dir("f/tool")                                                  // a directory
	linkDir := filepath.Join(root, "linked")
	link(b, linkDir) // another name for b

	path := strings.Join([]string{a, "", ".", "rel", b, a + "/", c, d, e, filepath.Join(root, "f"), linkDir, a}, ":")
	got := LookPathAll("tool", Env{"PATH=" + path})
	want := []string{
		filepath.Join(a, "tool"),
		filepath.Join(b, "tool"),
		filepath.Join(d, "tool"),
		filepath.Join(linkDir, "tool"),
	}
	if !slices.Equal(got, want) {
		t.Errorf("LookPathAll = %q\nwant %q", got, want)
	}
	if got := LookPathAll("none", Env{"PATH=" + path}); got != nil {
		t.Errorf("LookPathAll(none) = %q, want nil", got)
	}
	if got := LookPathAll("tool", Env{}); got != nil {
		t.Errorf("LookPathAll without PATH = %q, want nil", got)
	}
}
