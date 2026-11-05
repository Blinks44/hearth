package pkgmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
)

func TestNetworkQueries(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		id, file        string
		args            []string
		name, available string
		code            int
	}{
		{"winget", "winget-list.txt", []string{"list", "--source", "winget", "--disable-interactivity", "--authentication-mode", "silent"}, "Example.CLI", "2.0", 0},
		{"choco", "choco-outdated.txt", []string{"outdated", "--limit-output", "--no-color"}, "example.cli", "2.0", 2},
		{"snap", "snap-updates.txt", []string{"refresh", "--list"}, "example-cli", "2.0 (200)", 0},
	} {
		t.Run(tt.id, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatal(err)
			}
			f := &run.Fake{}
			path := filepath.Join(t.TempDir(), tt.id)
			args := append([]string{path}, tt.args...)
			f.Set(run.Result{Stdout: data, ExitCode: tt.code}, nil, args...)
			f.Stdout(`{"Name":"winget"}`, path, "source", "export", "--disable-interactivity")
			now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
			c := &versions.Cache{Dir: t.TempDir(), Now: func() time.Time { return now }}
			env := sysenv.Env{"HOME=" + t.TempDir()}
			m := inventory.PackageManager{ID: tt.id, Path: path, Unlisted: tt.id == "winget", Packages: []inventory.SystemPackage{{Name: tt.name, Version: "1.0"}}}
			c.SetOffline(true)
			offline := Network(t.Context(), c, f, env, m)
			if len(f.Calls()) != 0 || offline.UpdatesKnown || offline.UpdateError != "" {
				t.Fatalf("%+v, calls %v", offline, f.Calls())
			}
			c.SetOffline(false)
			got := Network(t.Context(), c, f, env, m)
			if !got.UpdatesKnown || got.UpdateError != "" || got.Packages[0].Available != tt.available || got.UpdatesFetched != now {
				t.Fatalf("%+v", got)
			}
			if m.Packages[0].Available != "" {
				t.Fatal("mutated local inventory")
			}
			if tt.id == "winget" && (got.Unlisted || len(got.Packages) != 2) {
				t.Fatalf("%+v", got)
			}
			calls := len(f.Calls())
			c.SetOffline(true)
			cached := Network(t.Context(), c, f, env, m)
			if len(f.Calls()) != calls || !cached.UpdatesKnown || cached.Packages[0].Available != tt.available {
				t.Fatalf("%+v", cached)
			}
			c.SetOffline(false)
			now = now.Add(25 * time.Hour)
			f.Set(run.Result{Stdout: data, Truncated: true}, nil, args...)
			stale := Network(t.Context(), c, f, env, m)
			if !stale.UpdatesKnown || stale.UpdateError == "" || stale.UpdatesFetched.Equal(now) {
				t.Fatalf("%+v", stale)
			}
			// A changed installation must not inherit a cached result for the old one.
			c.SetOffline(true)
			m.Packages[0].Version = "9.0"
			changed := Network(t.Context(), c, f, env, m)
			if changed.UpdatesKnown || changed.Packages[0].Available != "" {
				t.Fatalf("%+v", changed)
			}
		})
	}
}

func TestNetworkFailurePreservesLocal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		result run.Result
	}{
		{"failed", run.Result{ExitCode: 1}},
		{"truncated", run.Result{Truncated: true}},
		{"warning", run.Result{Stderr: []byte("repository unavailable")}},
		{"invalid", run.Result{Stdout: []byte("unexpected response")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &run.Fake{}
			f.Set(tt.result, nil, "choco", "outdated", "--limit-output", "--no-color")
			m := inventory.PackageManager{ID: "choco", Path: "choco", Packages: []inventory.SystemPackage{{Name: "example.cli", Version: "1.0"}}}
			got := Network(t.Context(), &versions.Cache{Dir: t.TempDir()}, f, nil, m)
			if got.UpdatesKnown || got.UpdateError == "" || len(got.Packages) != 1 || got.Packages[0].Version != "1.0" {
				t.Fatalf("%+v", got)
			}
		})
	}
}

func TestNetworkCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	f := &run.Fake{}
	got := Network(ctx, &versions.Cache{Dir: t.TempDir()}, f, nil, inventory.PackageManager{ID: "choco", Path: "choco"})
	if got.UpdatesKnown || len(f.Calls()) != 0 {
		t.Fatalf("%+v", got)
	}
}

func TestNetworkSnapUpToDate(t *testing.T) {
	t.Parallel()
	f := &run.Fake{}
	f.Set(run.Result{Stderr: []byte("All snaps up to date.\n")}, nil, "snap", "refresh", "--list")
	m := inventory.PackageManager{ID: "snap", Path: "snap", Packages: []inventory.SystemPackage{{Name: "example-cli", Version: "1.0"}}}
	got := Network(t.Context(), &versions.Cache{Dir: t.TempDir()}, f, nil, m)
	if !got.UpdatesKnown || got.UpdateError != "" || got.Packages[0].Available != "" {
		t.Fatalf("%+v", got)
	}
}

func TestNetworkCacheIgnoresSessionEnv(t *testing.T) {
	t.Parallel()
	f := &run.Fake{}
	f.Stdout("example.cli|1.0|2.0|false", "choco", "outdated", "--limit-output", "--no-color")
	c := &versions.Cache{Dir: t.TempDir()}
	m := inventory.PackageManager{ID: "choco", Path: "choco", Packages: []inventory.SystemPackage{{Name: "example.cli", Version: "1.0"}}}
	if got := Network(t.Context(), c, f, sysenv.Env{"PATH=/bin", "SSH_AUTH_SOCK=/tmp/a"}, m); !got.UpdatesKnown {
		t.Fatalf("%+v", got)
	}
	// After a new login the session variables differ; the result is still cached.
	c.SetOffline(true)
	if got := Network(t.Context(), c, f, sysenv.Env{"SSH_AUTH_SOCK=/tmp/b", "PATH=/bin"}, m); !got.UpdatesKnown {
		t.Fatalf("%+v", got)
	}
	if got := Network(t.Context(), c, f, sysenv.Env{"PATH=/usr/bin"}, m); got.UpdatesKnown {
		t.Fatalf("another PATH reused the result: %+v", got)
	}
}
