package ops

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/run"
)

func TestRunScriptStreamsAndReportsPID(t *testing.T) {
	var gotPID int
	var gotLines []containerlog.LogLine
	var gotCmd run.Cmd
	fakeLines := func(_ context.Context, c run.Cmd, emit func(string)) error {
		gotCmd = c
		if c.OnStart != nil {
			c.OnStart(4242)
		}
		emit("INFO starting server on :5173")
		emit("ERROR failed binding")
		return nil
	}
	exe := filepath.Join(t.TempDir(), "fake-bin")
	err := RunScript(t.Context(), fakeLines, nil, ScriptSpec{
		Dir:    t.TempDir(),
		Name:   "dev",
		Source: "package.json",
		Argv:   []string{exe, "run", "dev"},
	}, func(pid int) {
		gotPID = pid
	}, func(lines []containerlog.LogLine) {
		gotLines = append(gotLines, lines...)
	})
	if err != nil {
		t.Fatalf("RunScript error: %v", err)
	}
	if gotPID != 4242 {
		t.Fatalf("gotPID = %d, want 4242", gotPID)
	}
	if !gotCmd.KillGroup {
		t.Fatalf("expected KillGroup=true")
	}
	if !slices.Equal(gotCmd.Args, []string{exe, "run", "dev"}) {
		t.Fatalf("unexpected argv: %q", gotCmd.Args)
	}
	if len(gotLines) != 2 || gotLines[0].Level != containerlog.LevelInfo || gotLines[1].Level != containerlog.LevelError {
		t.Fatalf("unexpected lines: %+v", gotLines)
	}
}
