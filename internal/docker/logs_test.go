package docker

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/run"
)

func TestDockerLogsCombineContainerNames(t *testing.T) {
	first, second := strings.Repeat("a", 64), strings.Repeat("b", 64)
	logs := Logs{Target: colima, Since: 5 * time.Minute, Containers: []LogTarget{{first, "web"}, {second, "api"}}}
	read := func(_ context.Context, cmd run.Cmd, got func(string)) error {
		args := cmd.Args
		id := args[len(args)-1]
		want := colima.Command(nil, "logs", "--follow", "--timestamps", "--tail", "10000", "--since", "5m0s", "--", id).Args
		if !slices.Equal(args, want) {
			t.Errorf("command: %v, want %v", args, want)
		}
		got("2026-11-20T12:00:00Z INFO served")
		return nil
	}
	var names []string
	err := logs.Stream(t.Context(), read, nil, func(batch []containerlog.LogLine) {
		for _, line := range batch {
			names = append(names, line.Pod)
			if line.Text != "served" || line.At.IsZero() {
				t.Errorf("line: %+v", line)
			}
		}
	})
	slices.Sort(names)
	if err != nil || !slices.Equal(names, []string{"api", "web"}) {
		t.Fatalf("sources: %v, error: %v", names, err)
	}
}

func TestDockerShellCommand(t *testing.T) {
	for _, tc := range []struct {
		name string
		exit int
	}{
		{"bash", 0},
		{"sh", 127},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := strings.Repeat("a", 64)
			var r run.Fake
			r.Set(run.Result{ExitCode: tc.exit}, nil, colima.Command(nil, "exec", "--", id, "bash", "--version").Args...)
			cmd, err := ShellCommand(t.Context(), &r, nil, colima, id)
			if want := colima.Command(nil, "exec", "-it", "--", id, tc.name).Args; err != nil || !slices.Equal(cmd.Args, want) {
				t.Fatalf("command: %v, want %v, error: %v", cmd.Args, want, err)
			}
		})
	}
}
