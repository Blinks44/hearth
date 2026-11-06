//go:build !windows

package run

import (
	"strings"
	"testing"
)

func TestDetach(t *testing.T) {
	res, err := Exec{}.Run(t.Context(), Cmd{Args: []string{"/bin/sh", "-c", `echo $$ $(ps -o pgid= -p $$)`}})
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(res.Stdout))
	if len(f) != 2 || f[0] != f[1] {
		t.Fatalf("not a process group of its own: %q", res.Stdout)
	}
}
