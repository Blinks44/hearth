package sysenv

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestLookPathAll(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	for _, p := range []string{
		filepath.Join(a, "node.exe"),
		filepath.Join(a, "node.cmd"),
		filepath.Join(a, "node"), // no extension: not run by cmd
		filepath.Join(a, "node.txt"),
		filepath.Join(b, "node.exe"),
	} {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(b, "node.cmd"), 0o755); err != nil { // a directory: not run
		t.Fatal(err)
	}
	path := a + ";;.;" + b + ";" + a + `\`

	tests := []struct {
		name, find, pathExt string
		want                []string
	}{
		{"PATHEXT order", "node", ".EXE;.CMD", []string{
			filepath.Join(a, "node.exe"), filepath.Join(a, "node.cmd"), filepath.Join(b, "node.exe"),
		}},
		{"default PATHEXT", "node", "", []string{
			filepath.Join(a, "node.exe"), filepath.Join(a, "node.cmd"), filepath.Join(b, "node.exe"),
		}},
		{"name with extension", "node.exe", ".EXE;.CMD", []string{
			filepath.Join(a, "node.exe"), filepath.Join(b, "node.exe"),
		}},
		{"extension not in PATHEXT", "node", ".CMD", []string{filepath.Join(a, "node.cmd")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LookPathAll(tt.find, Env{"Path=" + path, "PATHEXT=" + tt.pathExt})
			if !slices.Equal(got, tt.want) {
				t.Errorf("LookPathAll = %q\nwant %q", got, tt.want)
			}
		})
	}
}
