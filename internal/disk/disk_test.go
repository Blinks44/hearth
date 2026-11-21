package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A hard link is counted once; a symlink is not followed, except root.
func TestMeasure(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "root")
	write(t, filepath.Join(root, "a"), 64<<10)
	write(t, filepath.Join(dir, "outside", "big"), 1<<20)
	single, err := Measure(context.Background(), root)
	if err != nil || single.Bytes < 64<<10 || single.Partial {
		t.Fatalf("one file: %+v, %v", single, err)
	}
	if err := os.Link(filepath.Join(root, "a"), filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	got, err := Measure(context.Background(), root)
	if err != nil || got.Bytes >= single.Bytes+64<<10 || got.Shared != 0 {
		t.Fatalf("with a hard link: %+v, want about %d", got, single.Bytes)
	}
	// A link from outside makes the file shared: removing root keeps it.
	if err := os.Link(filepath.Join(root, "a"), filepath.Join(dir, "c")); err != nil {
		t.Fatal(err)
	}
	if shared, err := Measure(context.Background(), root); err != nil || shared.Bytes != got.Bytes || shared.Shared < 64<<10 {
		t.Fatalf("linked from outside: %+v, %v", shared, err)
	}
	if err := os.Symlink(filepath.Join(dir, "outside"), filepath.Join(root, "link")); err != nil {
		t.Skip("no symlinks:", err)
	}
	got, err = Measure(context.Background(), root)
	if err != nil || got.Bytes >= single.Bytes+64<<10 {
		t.Fatalf("with a symlink: %+v, want about %d", got, single.Bytes)
	}
	link := filepath.Join(dir, "to-root")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if via, err := Measure(context.Background(), link); err != nil || via != got {
		t.Fatalf("through a symlinked root: %+v, %v", via, err)
	}
	if _, err := Measure(context.Background(), filepath.Join(dir, "missing")); err == nil {
		t.Fatal("no error for a missing root")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Measure(ctx, root); err == nil {
		t.Fatal("no error when canceled")
	}
}

// The system's copies, an installer's in a shared folder, shims and
// packages with a recorded size are not counted; a path shared by copies is counted once.
func TestTargets(t *testing.T) {
	snap := &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{Tool: "node", Installations: []inventory.Installation{
				{Source: inventory.Source{Kind: inventory.KindPackageManager, Root: "/brew/node"}},
				{Source: inventory.Source{Kind: inventory.KindVersionManager, Root: "/mise", Shim: true}},
				{Source: inventory.Source{Kind: inventory.KindSystem, Root: "/usr"}},
				{Source: inventory.Source{Kind: inventory.KindInstaller, Root: "/usr/local"}},
				{Source: inventory.Source{Kind: inventory.KindInstaller, Manager: "OrbStack", Root: "/Applications/OrbStack.app"}},
			}},
			{Tool: "npm", Installations: []inventory.Installation{{Source: inventory.Source{Kind: inventory.KindPackageManager, Root: "/brew/node"}}}},
			{Tool: "packages", PackageManagers: []inventory.PackageManager{{Packages: []inventory.SystemPackage{
				{Path: "/brew/node"},
				{Path: "/brew/Caskroom/app", Artifacts: []string{"/Applications/App.app"}},
				{Path: "/var/lib/git", Size: 10},
			}}}},
		},
		Agents: []inventory.Agent{{Installations: []inventory.Installation{{Source: inventory.Source{Kind: inventory.KindPackageManager, Root: "/brew/claude"}}}}},
	}
	snap.Tools = append(snap.Tools, inventory.ToolState{Tool: "editors", Editors: &inventory.EditorsState{Editors: []inventory.Editor{
		{App: "/Applications/Visual Studio Code.app", Extensions: "/home/.vscode/extensions", Superseded: []string{"/home/.vscode/extensions/go-0.45.0"}},
	}}})
	got := Targets(snap, []inventory.Cache{{ID: "npm", Path: "/home/.npm/_cacache"}})
	want := []string{"/brew/node", "/Applications/OrbStack.app", "/brew/Caskroom/app", "/Applications/App.app", "/home/.vscode/extensions", "/home/.vscode/extensions/go-0.45.0", "/brew/claude", "/home/.npm/_cacache"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Caches are where the environment moves them, else the defaults, and
// only those on disk.
func TestCaches(t *testing.T) {
	home := t.TempDir()
	moved := filepath.Join(home, "elsewhere", "uv")
	gomod := filepath.Join(home, "go", "pkg", "mod")
	for _, dir := range []string{moved, gomod, filepath.Join(home, ".cargo", "registry"), filepath.Join(home, ".bun", "install", "cache"), filepath.Join(home, ".ccache")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "UV_CACHE_DIR=" + moved}
	idea := filepath.Join(home, "jetbrains", "IntelliJIdea2024.3")
	if err := os.MkdirAll(idea, 0o755); err != nil {
		t.Fatal(err)
	}
	snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "go", Vars: map[string]string{"GOMODCACHE": gomod}},
		{Tool: "editors", Editors: &inventory.EditorsState{Editors: []inventory.Editor{{Caches: idea}, {Caches: filepath.Join(home, "gone")}}}}}}
	got := Caches(env, snap)
	want := []inventory.Cache{
		{ID: "bun", Path: filepath.Join(home, ".bun", "install", "cache")},
		{ID: "uv", Path: moved},
		{ID: "go-mod", Path: gomod},
		{ID: "cargo", Path: filepath.Join(home, ".cargo", "registry")},
		// ccache's old folder, while it is there.
		{ID: "ccache", Path: filepath.Join(home, ".ccache")},
		{ID: "jetbrains", Path: idea},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestDocker(t *testing.T) {
	snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "docker",
		Installations: []inventory.Installation{{Path: "/bin/docker", OnPath: 1}},
		Docker: &inventory.DockerState{Selected: "orbstack", Contexts: []inventory.DockerContext{
			{Name: "default", Endpoint: "unix:///var/run/docker.sock", Local: true},
			{Name: "orbstack", Endpoint: "unix:///orb.sock", Local: true, Status: "available"},
		}},
	}}}
	f := &run.Fake{}
	f.Stdout(`{"Active":"14","Reclaimable":"1.664GB (14%)","Size":"11.66GB","TotalCount":"16","Type":"Images"}
{"Active":"0","Reclaimable":"0B","Size":"0B","TotalCount":"0","Type":"Containers"}
{"Active":"0","Reclaimable":"6.979GB","Size":"14.46GB","TotalCount":"201","Type":"Build Cache"}
`, "/bin/docker", "--host", "unix:///orb.sock", "system", "df", "--format", "{{json .}}")
	// Of the reclaimable images, two have stopped containers: only what
	// the others alone hold goes.
	f.Stdout(`[{"Containers":"0","UniqueSize":"213.9MB"},{"Containers":"1","UniqueSize":"1.136GB"},{"Containers":"0","UniqueSize":"0B"},{"Containers":"2","UniqueSize":"300MB"}]
`, "/bin/docker", "--host", "unix:///orb.sock", "system", "df", "--verbose", "--format", "{{json .Images}}")
	got, err := Docker(context.Background(), f, nil, snap)
	want := []inventory.DockerUsage{
		{Type: "Images", Count: 16, Active: 14, Bytes: 11_660_000_000, Reclaimable: 213_900_000},
		{Type: "Containers"},
		{Type: "Build Cache", Count: 201, Bytes: 14_460_000_000, Reclaimable: 6_979_000_000},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("got %+v, %v", got, err)
	}
	// Images that all have containers free nothing, whatever Docker says.
	f.Stdout(`{"Active":"3","Reclaimable":"3.781GB (94%)","Size":"3.995GB","TotalCount":"3","Type":"Images"}
`, "/bin/docker", "--host", "unix:///orb.sock", "system", "df", "--format", "{{json .}}")
	got, err = Docker(context.Background(), f, nil, snap)
	if want := []inventory.DockerUsage{{Type: "Images", Count: 3, Active: 3, Bytes: 3_995_000_000}}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("all images in use: got %+v, %v", got, err)
	}
	// The context the terminal selects is not asked unless the scan found
	// its engine running.
	snap.Tools[0].Docker.Contexts[1].Status = "unavailable"
	if _, err := Docker(context.Background(), f, nil, snap); !errors.Is(err, ErrNoEngine) {
		t.Fatalf("stopped engine: %v", err)
	}
	if _, err := Docker(context.Background(), f, nil, &inventory.Snapshot{}); !errors.Is(err, ErrNoEngine) {
		t.Fatalf("no docker: %v", err)
	}

}

// Count refreshes caches before discovering projects and gives each callback
// its own map, so a background walk never mutates data already shown by the UI.
func TestCountProgress(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, "cache")
	modules := filepath.Join(home, "project", "node_modules")
	write(t, filepath.Join(cache, "data"), 2<<20)
	write(t, filepath.Join(home, "project", "package.json"), 2)
	write(t, filepath.Join(modules, "package", "data"), 1<<20)
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "UV_CACHE_DIR=" + cache}
	var updates []inventory.Sizes
	got, err := Count(t.Context(), &run.Fake{}, env, &inventory.Snapshot{}, nil, func(s inventory.Sizes) { updates = append(updates, s) })
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) < 3 {
		t.Fatalf("no incremental progress: %d", len(updates))
	}
	if len(updates[0].Paths) != 0 {
		t.Fatal("initial result was mutated")
	}
	first := updates[1]
	if first.Paths[cache].Bytes < 2<<20 || len(first.Installs) != 0 || len(first.Paths) != 1 {
		t.Fatalf("cache not published first: %+v", first)
	}
	if got.Paths[modules].Bytes < 1<<20 || len(got.Installs) != 1 || got.Taken.IsZero() {
		t.Fatalf("final result: %+v", got)
	}
	// Removing the cache must remove its old size, including from progress.
	if err := os.RemoveAll(cache); err != nil {
		t.Fatal(err)
	}
	got, err = Count(t.Context(), &run.Fake{}, env, &inventory.Snapshot{}, nil, func(s inventory.Sizes) {
		if _, ok := s.Paths[cache]; ok {
			t.Error("removed cache returned during refresh")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Paths[cache]; ok {
		t.Fatal("removed cache remains")
	}
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	_, err = Count(ctx, &run.Fake{}, env, &inventory.Snapshot{}, nil, func(inventory.Sizes) { calls++; cancel() })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("cancelled refresh: %d updates, %v", calls, err)
	}
}

// Only the active store is counted, including a configured path outside the
// defaults. A failed lookup must not substitute the whole parent store.
func TestActivePnpmStore(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	executable := "pnpm"
	if filepath.Separator == '\\' {
		executable += ".exe"
	}
	command := filepath.Join(bin, executable)
	write(t, command, 0)
	if err := os.Chmod(command, 0o755); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(home, "custom-store", "v11")
	write(t, filepath.Join(store, "data"), 4096)
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + bin}
	for _, tc := range []struct {
		name, output string
		exit         int
		want         bool
	}{
		{"active", store + "\n", 0, true},
		{"failed", store, 1, false},
		{"relative", "store/v11", 0, false},
		{"multiline", store + "\nwarning", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &run.Fake{}
			f.Set(run.Result{Stdout: []byte(tc.output), ExitCode: tc.exit}, nil, command, "store", "path")
			got := activeCaches(t.Context(), f, env, &inventory.Snapshot{})
			i := slices.IndexFunc(got, func(c inventory.Cache) bool { return c.ID == "pnpm" })
			if (i >= 0) != tc.want || i >= 0 && got[i].Path != store {
				t.Fatalf("caches: %+v", got)
			}
		})
	}
}
