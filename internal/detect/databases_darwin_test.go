package detect

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDatabaseLocationsDarwin(t *testing.T) {
	patterns := databaseGlobs(sysenv.Env{"HOMEBREW_PREFIX=/test/brew"}, "postgres")
	if !strings.Contains(strings.Join(patterns, ":"), "/test/brew/Cellar/*/*/bin/postgres") || !strings.Contains(strings.Join(patterns, ":"), "Postgres.app") {
		t.Fatal(patterns)
	}
}

func TestDatabaseListenersDarwin(t *testing.T) {
	got := databaseLsof("p812\ncpostgres\nn127.0.0.1:5432\nn[::1]:5433\np815\ncredis-server\nn*:6379\n")
	if len(got) != 3 || got[0].Port != "5432" || got[1].Address != "[::1]" || got[2].Process != "redis-server" || got[2].PID != "815" {
		t.Fatal(got)
	}
}

func TestDatabaseNoListenersDarwin(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "lsof")
	fakeExe(t, exe)
	r := &run.Fake{}
	r.Set(run.Result{ExitCode: 1}, nil, exe, "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn")
	d := &inventory.DatabaseState{}
	databaseListeners(t.Context(), r, sysenv.Env{"PATH=" + dir}, d)
	if len(d.Warnings) != 0 || len(d.Listeners) != 0 {
		t.Fatal(d)
	}
}
