package sysenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing")
	env := Env{"PATH=" + root + string(os.PathListSeparator) + missing + string(os.PathListSeparator) + root + string(os.PathListSeparator) + file}
	got := Paths(env)
	if len(got) != 4 || got[0].Missing || !got[1].Missing || got[2].Dir != root || !got[3].Missing {
		t.Fatalf("entries: %+v", got)
	}
}

func TestVariables(t *testing.T) {
	got := Variables(Env{"JAVA_HOME=/old", "JAVA_HOME=/jdk", "API_TOKEN=secret", "CUSTOM_SECRET=secret"}, Env{"GOPATH=/go"})
	if len(got) != 2 || got[0].Terminal != "/jdk" || got[0].GUI != "" || got[1].GUI != "/go" {
		t.Fatalf("variables: %+v", got)
	}
}
