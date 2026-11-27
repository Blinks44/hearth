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
	got := databaseLsof("p812\ncpostgres\nu501\nn127.0.0.1:5432\nn[::1]:5433\np815\ncredis-server\nu0\nn*:6379\n", "501")
	if len(got) != 3 || got[0].Port != "5432" || got[1].Address != "[::1]" || !got[1].Own || got[2].Process != "redis-server" || got[2].PID != "815" || got[2].Own {
		t.Fatal(got)
	}
}

func TestListenerProcessesDarwin(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "lsof")
	fakeExe(t, exe)
	r := &run.Fake{}
	r.Stdout("p4312\ncnode\nu501\nn*:5173\nn[::]:5173\np90\ncControlCenter\nu501\nn*:5000\n", exe, "+c0", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcun")
	r.Set(run.Result{ExitCode: 1, Stdout: []byte("p4312\nfcwd\nn/Users/me/dev/shop\nftxt\nn/opt/homebrew/bin/node\nftxt\nn/usr/lib/dyld\np90\nfcwd\nn/\nftxt\nn/System/Library/CoreServices/ControlCenter.app/Contents/MacOS/ControlCenter\n")}, nil,
		exe, "-a", "-p", "4312,90", "-d", "cwd,txt", "-Fpfn")
	d := &inventory.DatabaseState{}
	databaseListeners(t.Context(), r, sysenv.Env{"PATH=" + dir}, d)
	if len(d.Listeners) != 3 || d.Listeners[1].Path != "/opt/homebrew/bin/node" || len(d.Listeners[1].Dirs) != 1 || d.Listeners[1].Dirs[0] != "/Users/me/dev/shop" ||
		!strings.HasPrefix(d.Listeners[2].Path, "/System/") || d.Listeners[2].Dirs != nil {
		t.Fatal(d.Listeners)
	}
}

func TestDatabaseNoListenersDarwin(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "lsof")
	fakeExe(t, exe)
	r := &run.Fake{}
	r.Set(run.Result{ExitCode: 1}, nil, exe, "+c0", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcun")
	d := &inventory.DatabaseState{}
	databaseListeners(t.Context(), r, sysenv.Env{"PATH=" + dir}, d)
	if len(d.Warnings) != 0 || len(d.Listeners) != 0 {
		t.Fatal(d)
	}
}

func TestParseDarwinPSPGID(t *testing.T) {
	raw := "  4769  4760  4678  1.4  145408  05:12 /opt/homebrew/bin/node /repo/node_modules/.bin/rsbuild dev\n"
	meta := parseDarwinPS(raw)
	m, ok := meta["4769"]
	if !ok {
		t.Fatalf("missing pid 4769 in %+v", meta)
	}
	if m.ppid != "4760" || m.pgid != "4678" || m.cpu != 1.4 || m.mem != 145408*1024 {
		t.Fatalf("unexpected psMeta: %+v", m)
	}
	if len(m.args) < 2 || !strings.HasSuffix(m.args[1], "rsbuild") {
		t.Fatalf("unexpected args: %+v", m.args)
	}
}

func TestParseDarwinParentMapAndAncestors(t *testing.T) {
	raw := "    1     0\n 27716     1\n 27740 27716\n 27780 27740\n 27803 27780\n"
	parents := parseDarwinParentMap(raw)
	got := walkAncestorPIDs("27803", parents, 12)
	want := []string{"27780", "27740", "27716"}
	if len(got) != len(want) {
		t.Fatalf("walkAncestorPIDs got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("walkAncestorPIDs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
