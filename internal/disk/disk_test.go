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
	if err != nil || got.Bytes >= single.Bytes+64<<10 {
		t.Fatalf("with a hard link: %+v, want about %d", got, single.Bytes)
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
	got := Targets(snap, []inventory.Cache{{ID: "npm", Path: "/home/.npm/_cacache"}})
	want := []string{"/brew/node", "/Applications/OrbStack.app", "/brew/Caskroom/app", "/Applications/App.app", "/brew/claude", "/home/.npm/_cacache"}
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
	for _, dir := range []string{moved, gomod, filepath.Join(home, ".cargo", "registry"), filepath.Join(home, ".bun", "install", "cache")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "UV_CACHE_DIR=" + moved}
	snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "go", Vars: map[string]string{"GOMODCACHE": gomod}}}}
	got := Caches(env, snap)
	want := []inventory.Cache{
		{ID: "bun", Path: filepath.Join(home, ".bun", "install", "cache")},
		{ID: "uv", Path: moved},
		{ID: "go-mod", Path: gomod},
		{ID: "cargo", Path: filepath.Join(home, ".cargo", "registry")},
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
	got, err := Docker(context.Background(), f, nil, snap)
	want := []inventory.DockerUsage{
		{Type: "Images", Count: 16, Active: 14, Bytes: 11_660_000_000, Reclaimable: 1_664_000_000},
		{Type: "Containers"},
		{Type: "Build Cache", Count: 201, Bytes: 14_460_000_000, Reclaimable: 6_979_000_000},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("got %+v, %v", got, err)
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
	if _, err := parseDockerDF([]byte(`{"Type":"Images","TotalCount":"1","Active":"1","Size":"12XB","Reclaimable":"0B"}`)); err == nil {
		t.Fatal("unknown unit read")
	}
}
