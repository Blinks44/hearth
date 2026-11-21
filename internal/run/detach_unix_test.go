//go:build !windows

package run

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

func TestLinesCancelWithInheritedPipe(t *testing.T) {
	t.Parallel()
	for _, stream := range []struct {
		name string
		read func(context.Context, Cmd, func(string)) error
	}{
		{"stdout", Lines},
		{"combined", CombinedLines},
	} {
		t.Run(stream.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			// The grandchild holds the pipe open after its parent is stopped.
			cmd := Cmd{Args: []string{"/bin/sh", "-c", "sleep 5 & echo ready; wait"}}
			start := time.Now()
			err := stream.read(ctx, cmd, func(line string) {
				if line == "ready" {
					cancel()
				}
			})
			if !errors.Is(err, context.Canceled) || time.Since(start) >= 3*time.Second {
				t.Fatalf("canceled stream after %v: %v", time.Since(start), err)
			}
		})
	}
}
