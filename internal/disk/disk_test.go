package disk

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	agentDir := filepath.Join(t.TempDir(), ".claude")
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
		Agents: []inventory.Agent{{Dir: agentDir, Installations: []inventory.Installation{{Source: inventory.Source{Kind: inventory.KindPackageManager, Root: "/brew/claude"}}}}},
	}
	snap.Tools = append(snap.Tools, inventory.ToolState{Tool: "editors", Editors: &inventory.EditorsState{Editors: []inventory.Editor{
		{App: "/Applications/Visual Studio Code.app", Extensions: "/home/.vscode/extensions", Superseded: []string{"/home/.vscode/extensions/go-0.45.0"}},
	}}})
	got := Targets(snap, []inventory.Cache{{ID: "npm", Path: "/home/.npm/_cacache"}, {ID: "rustup", Paths: []string{"/home/.rustup/downloads", "/home/.rustup/tmp"}}})
	want := []string{"/brew/node", "/Applications/OrbStack.app", "/brew/Caskroom/app", "/Applications/App.app", "/home/.vscode/extensions", "/home/.vscode/extensions/go-0.45.0", "/brew/claude", agentDir, "/home/.npm/_cacache", "/home/.rustup/downloads", "/home/.rustup/tmp"}
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
	if !reflect.DeepEqual(got, want) {
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

// Expanded Rust, NuGet, Playwright, and safe AI agent caches are found on disk
// while agent configs, credentials, skills, rules, memory, and active Claude
// versions are never included.
func TestCachesExpandedAndSafe(t *testing.T) {
	home := t.TempDir()
	nugetDir := filepath.Join(home, "custom-nuget")
	pwDir := filepath.Join(home, "custom-playwright")
	env := sysenv.Env{
		"HOME=" + home,
		"USERPROFILE=" + home,
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"NUGET_PACKAGES=" + nugetDir,
		"PLAYWRIGHT_BROWSERS_PATH=" + pwDir,
	}
	mkdirs := []string{
		filepath.Join(home, ".cargo", "registry"),
		filepath.Join(home, ".cargo", "git"),
		filepath.Join(home, ".rustup", "downloads"),
		filepath.Join(home, ".rustup", "tmp"),
		nugetDir,
		pwDir,
		// Claude Code safe dirs + versions (1.0.0 old, 2.0.0 active, 2.1.0 newest).
		filepath.Join(home, ".claude", "file-history"),
		filepath.Join(home, ".claude", "debug"),
		filepath.Join(home, ".claude", "paste-cache"),
		filepath.Join(home, ".claude", "shell-snapshots"),
		filepath.Join(home, ".claude", "session-env"),
		filepath.Join(home, ".claude", "telemetry"),
		filepath.Join(home, ".claude", "statsig"),
		filepath.Join(home, ".claude", "backups"),
		filepath.Join(home, ".local", "share", "claude", "versions"),
		// Codex safe dirs + opt-in worktrees.
		filepath.Join(home, ".codex", "log"),
		filepath.Join(home, ".codex", "tmp"),
		filepath.Join(home, ".codex", "shell_snapshots"),
		filepath.Join(home, ".codex", "worktrees"),
		// Gemini safe dirs.
		filepath.Join(home, ".gemini", "cache"),
		filepath.Join(home, ".gemini", "tmp", "abc123", "logs"),
		filepath.Join(home, ".gemini", "tmp", "abc123", "checkpoints"),
		filepath.Join(home, ".gemini", "tmp", "abc123", "shell-snapshots"),
		// Copilot safe dirs.
		filepath.Join(home, ".copilot", "logs"),
		filepath.Join(home, ".copilot", "pkg"),
		// OpenCode safe dirs.
		filepath.Join(home, ".local", "share", "opencode", "snapshot"),
		filepath.Join(home, ".local", "share", "opencode", "log"),
		filepath.Join(home, ".cache", "opencode"),
		// Cursor logs + Electron caches.
		filepath.Join(home, ".cursor", "logs"),
	}
	if appData := cursorAppData(env, home); appData != "" {
		mkdirs = append(mkdirs,
			filepath.Join(appData, "Cache"),
			filepath.Join(appData, "CachedData"),
			filepath.Join(appData, "GPUCache"),
		)
	}
	for _, d := range mkdirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Claude native versions: 1.0.0 (old), 2.0.0 (active via symlink), 2.1.0 (newest).
	claudeVerDir := filepath.Join(home, ".local", "share", "claude", "versions")
	v100 := filepath.Join(claudeVerDir, "1.0.0")
	v200 := filepath.Join(claudeVerDir, "2.0.0")
	v210 := filepath.Join(claudeVerDir, "2.1.0")
	for _, f := range []string{v100, v200, v210} {
		write(t, f, 1024)
	}
	if err := os.MkdirAll(filepath.Join(home, ".local", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(v200, filepath.Join(home, ".local", "bin", "claude"))

	// Sensitive agent files that must NEVER appear in any cache plan.
	sensitive := []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".claude", ".credentials.json"),
		filepath.Join(home, ".claude", "skills", "my-skill", "SKILL.md"),
		filepath.Join(home, ".claude", "projects", "p", "memory", "MEMORY.md"),
		filepath.Join(home, ".codex", "config.toml"),
		filepath.Join(home, ".codex", "auth.json"),
		filepath.Join(home, ".codex", "AGENTS.md"),
		filepath.Join(home, ".gemini", "settings.json"),
		filepath.Join(home, ".gemini", "oauth_creds.json"),
		filepath.Join(home, ".gemini", "GEMINI.md"),
		filepath.Join(home, ".copilot", "config.json"),
		filepath.Join(home, ".config", "opencode", "opencode.json"),
		filepath.Join(home, ".cursor", "mcp.json"),
	}
	if appData := cursorAppData(env, home); appData != "" {
		sensitive = append(sensitive,
			filepath.Join(appData, "User", "settings.json"),
			filepath.Join(appData, "User", "globalStorage", "state.vscdb"),
		)
	}
	for _, f := range sensitive {
		write(t, f, 64)
	}

	snap := &inventory.Snapshot{
		Agents: []inventory.Agent{{
			ID:            "claude",
			Installations: []inventory.Installation{{Path: filepath.Join(home, ".local", "bin", "claude"), RealPath: v200}},
		}},
	}
	got := Caches(env, snap)
	byID := map[string]inventory.Cache{}
	for _, c := range got {
		byID[c.ID] = c
	}

	for _, id := range []string{"cargo", "rustup", "nuget", "playwright", "claude-cache", "codex-cache", "codex-worktrees", "gemini-cache", "copilot-cache", "opencode-cache", "cursor-cache"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing expected cache %q in %+v", id, got)
		}
	}
	if len(byID["cargo"].AllPaths()) != 2 {
		t.Errorf("cargo paths = %q, want registry+git", byID["cargo"].AllPaths())
	}
	if len(byID["rustup"].AllPaths()) != 2 {
		t.Errorf("rustup paths = %q, want downloads+tmp", byID["rustup"].AllPaths())
	}
	if !byID["codex-worktrees"].OptIn {
		t.Errorf("codex-worktrees OptIn = false, want true")
	}
	if byID["codex-cache"].OptIn {
		t.Errorf("codex-cache OptIn = true, want false")
	}

	// Claude versions: only 1.0.0 may be offered; active (2.0.0) and newest (2.1.0) stay protected.
	claudePaths := byID["claude-cache"].AllPaths()
	if !slices.Contains(claudePaths, v100) {
		t.Errorf("old Claude version %s not offered in %q", v100, claudePaths)
	}
	if slices.Contains(claudePaths, v200) || slices.Contains(claudePaths, v210) {
		t.Errorf("active or newest Claude version offered in %q", claudePaths)
	}

	// Verify no sensitive file or parent of a sensitive file is ever included.
	for _, c := range got {
		for _, cp := range c.AllPaths() {
			for _, sf := range sensitive {
				if cp == sf || isSubpath(sf, cp) {
					t.Errorf("cache %q path %q covers sensitive file %q", c.ID, cp, sf)
				}
			}
		}
	}

	// PLAYWRIGHT_BROWSERS_PATH=0 means project-local browsers and must not create a relative cache.
	envZero := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PLAYWRIGHT_BROWSERS_PATH=0"}
	for _, c := range Caches(envZero, nil) {
		if c.ID == "playwright" {
			t.Errorf("PLAYWRIGHT_BROWSERS_PATH=0 unexpectedly produced playwright cache: %+v", c)
		}
	}
}

func isSubpath(target, base string) bool {
	rel, err := filepath.Rel(base, target)
	return err == nil && rel != ".." && len(rel) > 0 && rel[0] != '.'
}

func TestCountWithOptionsWorkerPoolAndThrottle(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	extProj := filepath.Join(outside, "ext-web")
	write(t, filepath.Join(extProj, "package.json"), 2)
	write(t, filepath.Join(extProj, "node_modules", "pkg", "index.js"), 2<<20)
	write(t, filepath.Join(extProj, ".next", "cache.bin"), 1<<20)

	// Create 12 small projects under home so throttling collapses per-folder progress callbacks.
	for i := range 12 {
		dir := filepath.Join(home, "p"+string(rune('a'+i)))
		write(t, filepath.Join(dir, "package.json"), 2)
		write(t, filepath.Join(dir, "node_modules", "a.js"), 16<<10)
	}

	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home}
	callbacks := 0
	got, err := CountWithOptions(t.Context(), &run.Fake{}, env, &inventory.Snapshot{}, nil, CountOptions{
		Projects: []string{extProj},
	}, func(inventory.Sizes) {
		callbacks++
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Installs) != 14 {
		t.Fatalf("Installs count = %d, want 14 (12 home + 2 external)", len(got.Installs))
	}
	if got.Paths[filepath.Join(extProj, "node_modules")].Bytes < 2<<20 {
		t.Errorf("external node_modules not measured: %+v", got.Paths[filepath.Join(extProj, "node_modules")])
	}
	if got.Paths[filepath.Join(extProj, ".next")].Bytes < 1<<20 {
		t.Errorf("external .next not measured: %+v", got.Paths[filepath.Join(extProj, ".next")])
	}
	if callbacks >= 10 {
		t.Errorf("progress callbacks = %d, expected throttling to coalesce rapid folder measurements", callbacks)
	}
}
