package versions

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestFetchMise(t *testing.T) {
	t.Parallel()
	// As mise 2026.11.1 printed them on macOS, with jq 1.7 asked for in
	// the global config, shellcheck installed without one and go added.
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	const mise = "/opt/homebrew/bin/mise"
	env := sysenv.Env{"HOME=/Users/admin", "PATH=/opt/homebrew/bin"}
	var r run.Fake
	r.Stdout(read("mise-outdated.json"), mise, "outdated", "--json", "--inactive")
	r.Stdout(read("mise-outdated-bump.json"), mise, "outdated", "--json", "--inactive", "--bump")
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	c := &Cache{Dir: t.TempDir(), Now: func() time.Time { return now }}
	installed := []string{"jq@1.7", "shellcheck@0.9.0", "go@1.26.3"}

	c.SetOffline(true)
	if _, err := FetchMise(t.Context(), c, &r, env, mise, installed); !errors.Is(err, ErrOffline) || len(r.Calls()) != 0 {
		t.Fatalf("offline with nothing kept: %v, %d calls", err, len(r.Calls()))
	}
	c.SetOffline(false)
	got, err := FetchMise(t.Context(), c, &r, env, mise, installed)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]MiseTool{
		"go":         {Current: "1.26.3", InRange: "1.26.10", Latest: "1.27.3"},
		"jq":         {Current: "1.7", Latest: "1.8.2"},
		"shellcheck": {Current: "0.9.0", Latest: "0.11.0"},
	}
	if !reflect.DeepEqual(got.Tools, want) || !got.Fetched.Equal(now) {
		t.Errorf("FetchMise() = %+v, want %+v", got, want)
	}
	for _, call := range r.Calls() {
		if call.Dir != "/Users/admin" {
			t.Errorf("mise ran in %q, not the home folder", call.Dir)
		}
	}

	// The same tools in another order: kept, mise is not asked.
	if _, err := FetchMise(t.Context(), c, &r, env, mise, []string{"go@1.26.3", "jq@1.7", "shellcheck@0.9.0"}); err != nil || len(r.Calls()) != 2 {
		t.Fatalf("kept result: %v, %d calls", err, len(r.Calls()))
	}
	// Another version installed: mise is asked again.
	if _, err := FetchMise(t.Context(), c, &r, env, mise, append(installed, "jq@1.8.2")); err != nil || len(r.Calls()) != 4 {
		t.Fatalf("changed tools: %v, %d calls", err, len(r.Calls()))
	}

	var failing run.Fake
	failing.Set(run.Result{ExitCode: 1, Stderr: []byte("mise ERROR network")}, nil, mise, "outdated", "--json", "--inactive")
	if _, err := FetchMise(t.Context(), &Cache{Dir: t.TempDir()}, &failing, env, mise, installed); err == nil {
		t.Error("a failed mise outdated gave no error")
	}
}
