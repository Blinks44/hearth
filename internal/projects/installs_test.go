package projects

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Only a node_modules next to a package.json is a project's; tools',
// apps' and the ones inside node_modules are not.
func TestNodeModules(t *testing.T) {
	home := t.TempDir()
	// mk makes the files named package.json and the folders of the others.
	mk := func(rel ...string) {
		t.Helper()
		for _, r := range rel {
			path := filepath.Join(home, filepath.FromSlash(r))
			dir := path
			if filepath.Base(path) == "package.json" {
				dir = filepath.Dir(path)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if dir != path {
				if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	project := func(dir string) { mk(dir+"/package.json", dir+"/node_modules") }
	project("dev/web")
	// A monorepo: the root's and a package's, not the ones inside them.
	project("dev/mono")
	project("dev/mono/packages/ui")
	project("dev/mono/node_modules/left-pad")
	mk("dev/mono/node_modules/left-pad/node_modules/x/package.json")
	// No package.json next to it: not a project's.
	mk("dev/loose/node_modules")
	// Global packages of a prefix under home.
	mk("n/lib/node_modules/typescript/package.json")
	project(".npm-global/lib/node_modules/typescript")
	// A test's fixture, as Find leaves out.
	project("dev/web/testdata/app")
	// Another file system mounted under home: OrbStack's volumes.
	project("OrbStack/docker/volumes/app_node_modules/glob")
	// Tools', editors' and apps' own.
	project(".nvm/versions/node/v22.0.0/lib/node_modules/npm")
	project(".vscode/extensions/ms-python.python-2024.0.0")
	project("Library/Application Support/Code/User/globalStorage/x")
	project("AppData/Roaming/npm/node_modules/pnpm")
	project("Music/Music Library/x")
	project("Pictures/Photos Library.photoslibrary/x")
	project("go/pkg/mod/github.com/x/y@v1.0.0/web")
	project("miniconda3/share/jupyter/lab/staging")
	project("Applications/Tool.app/Contents/Resources/app")
	project("Downloads/VSCode-linux-x64/resources/app")

	old := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(home, "dev", "web", "package.json"), old, old); err != nil {
		t.Fatal(err)
	}
	device := deviceOf
	t.Cleanup(func() { deviceOf = device })
	deviceOf = func(info fs.FileInfo) (uint64, bool) {
		if info.Name() == "OrbStack" {
			return 99, true
		}
		return 1, true
	}
	got, err := NodeModules(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, in := range got {
		rel, _ := filepath.Rel(home, in.Project)
		dirs = append(dirs, filepath.ToSlash(rel))
		if in.Name != "node_modules" || in.Changed.IsZero() {
			t.Errorf("%s: %+v", rel, in)
		}
		if rel == filepath.Join("dev", "web") && !in.Changed.Equal(old) {
			t.Errorf("web changed %v, want %v", in.Changed, old)
		}
	}
	slices.Sort(dirs)
	if want := []string{"dev/mono", "dev/mono/packages/ui", "dev/web"}; !slices.Equal(dirs, want) {
		t.Errorf("found %q, want %q", dirs, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NodeModules(ctx, home); err == nil {
		t.Error("no error after ctx ended")
	}
}

func TestInstallsEcosystems(t *testing.T) {
	home := t.TempDir()
	mkFile := func(rel string) {
		t.Helper()
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkDir := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Valid project dependency/build folders next to their manifests.
	mkFile("dev/py/pyproject.toml")
	mkDir("dev/py/.venv")
	mkFile("dev/rust/Cargo.toml")
	mkDir("dev/rust/target")
	mkFile("dev/java/pom.xml")
	mkDir("dev/java/target")
	mkFile("dev/php/composer.json")
	mkDir("dev/php/vendor")
	// Go vendor/ is often committed and must not be suggested for automatic cleanup.
	mkFile("dev/goapp/go.mod")
	mkDir("dev/goapp/vendor")

	// Loose folders without matching manifests must be ignored.
	mkDir("dev/loose/.venv")
	mkDir("dev/loose/target")
	mkDir("dev/loose/vendor")
	// Inside testdata or hidden dirs must be ignored.
	mkFile("dev/rust/testdata/sample/Cargo.toml")
	mkDir("dev/rust/testdata/sample/target")
	mkFile(".cache/py/pyproject.toml")
	mkDir(".cache/py/.venv")

	got, err := Installs(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	var pairs []string
	for _, in := range got {
		rel, _ := filepath.Rel(home, in.Project)
		pairs = append(pairs, filepath.ToSlash(rel)+"/"+in.Name)
		if in.Changed.IsZero() {
			t.Errorf("%s/%s has zero Changed", rel, in.Name)
		}
	}
	slices.Sort(pairs)
	want := []string{
		"dev/java/target",
		"dev/php/vendor",
		"dev/py/.venv",
		"dev/rust/target",
	}
	if !slices.Equal(pairs, want) {
		t.Errorf("Installs = %q, want %q", pairs, want)
	}

	// Read records manifest-guarded target and vendor in Project.Installs.
	rustProj := Read(filepath.Join(home, "dev", "rust"))
	if !slices.Equal(rustProj.Installs, []string{"target"}) {
		t.Errorf("rust Installs = %q, want [target]", rustProj.Installs)
	}
	looseProj := Read(filepath.Join(home, "dev", "loose"))
	if !slices.Equal(looseProj.Installs, []string{".venv"}) {
		t.Errorf("loose Installs = %q, want [.venv]", looseProj.Installs)
	}
}

func TestInstallsWithExclude(t *testing.T) {
	home := t.TempDir()
	for _, rel := range []string{"work/keep/package.json", "archive/skip/package.json"} {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(filepath.Dir(p), "node_modules"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := InstallsWithExclude(context.Background(), home, []string{filepath.Join(home, "archive")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0].Project) != "keep" {
		t.Fatalf("InstallsWithExclude = %+v, want only keep", got)
	}
}

func TestInstallsExpandedSpecsRolesAndExtraProjects(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()

	mkFile := func(root, rel string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkDir := func(root, rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Next.js + Turbo + dist + parcel + svelte-kit + nuxt
	mkFile(home, "web/package.json")
	mkFile(home, "web/next.config.mjs")
	mkDir(home, "web/node_modules")
	mkDir(home, "web/.next")
	mkDir(home, "web/.turbo")
	mkDir(home, "web/.parcel-cache")
	mkDir(home, "web/dist")
	mkFile(home, "nuxtapp/nuxt.config.ts")
	mkDir(home, "nuxtapp/.nuxt")
	mkFile(home, "svelteapp/svelte.config.js")
	mkDir(home, "svelteapp/.svelte-kit")

	// Gradle + iOS Pods + Python venv / caches
	mkFile(home, "android/build.gradle.kts")
	mkDir(home, "android/build")
	mkDir(home, "android/.gradle")
	mkFile(home, "ios/Podfile")
	mkDir(home, "ios/Pods")
	mkFile(home, "py/pyproject.toml")
	mkDir(home, "py/venv")
	mkDir(home, "py/__pycache__")
	mkDir(home, "py/.pytest_cache")

	// .NET glob manifests (*.csproj) for bin/ and obj/
	mkFile(home, "dotnet/App.csproj")
	mkDir(home, "dotnet/bin")
	mkDir(home, "dotnet/obj")

	// False-positive guards: Go repo with build/ and bin/, loose dist/obj without manifests
	mkFile(home, "goapp/go.mod")
	mkDir(home, "goapp/build")
	mkDir(home, "goapp/bin")
	mkDir(home, "loose/dist")
	mkDir(home, "loose/obj")
	mkDir(home, "loose/Pods")

	// Tracked project outside $HOME
	extProj := filepath.Join(outside, "external-app")
	mkFile(outside, "external-app/package.json")
	mkDir(outside, "external-app/node_modules")
	mkDir(outside, "external-app/.next")

	got, err := InstallsWithOptions(context.Background(), home, InstallsOptions{
		ExtraProjects: []string{extProj},
	})
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]string{}
	for _, in := range got {
		key := filepath.Base(in.Project) + "/" + in.Name
		roles[key] = in.Role
	}
	wantRoles := map[string]string{
		"web/node_modules":          "deps",
		"web/.next":                 "build",
		"web/.turbo":                "build",
		"web/.parcel-cache":         "build",
		"web/dist":                  "build",
		"nuxtapp/.nuxt":             "build",
		"svelteapp/.svelte-kit":     "build",
		"android/build":             "build",
		"android/.gradle":           "build",
		"ios/Pods":                  "deps",
		"py/venv":                   "deps",
		"py/__pycache__":            "build",
		"py/.pytest_cache":          "build",
		"dotnet/bin":                "build",
		"dotnet/obj":                "build",
		"external-app/node_modules": "deps",
		"external-app/.next":        "build",
	}
	for k, want := range wantRoles {
		if roles[k] != want {
			t.Errorf("%s role = %q, want %q", k, roles[k], want)
		}
	}
	for _, forbidden := range []string{"goapp/build", "goapp/bin", "loose/dist", "loose/obj", "loose/Pods"} {
		if _, ok := roles[forbidden]; ok {
			t.Errorf("unexpected false positive %q", forbidden)
		}
	}
}

func TestChangedMonorepoAndWorktree(t *testing.T) {
	root := t.TempDir()
	old := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

	// 1. Monorepo subpackage inherits parent .git/index mtime.
	mono := filepath.Join(root, "mono")
	sub := filepath.Join(mono, "packages", "web")
	if err := os.MkdirAll(filepath.Join(mono, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(sub, "package.json")
	if err := os.WriteFile(pkg, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(pkg, old, old); err != nil {
		t.Fatal(err)
	}
	idx := filepath.Join(mono, ".git", "index")
	if err := os.WriteFile(idx, []byte("idx"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(idx, recent, recent); err != nil {
		t.Fatal(err)
	}
	if got := changed(Project{Dir: sub, Files: []string{"package.json"}}); !got.Equal(recent) {
		t.Errorf("monorepo subpackage changed = %v, want %v", got, recent)
	}

	// 2. Git worktree (.git file pointing to worktree gitdir).
	wt := filepath.Join(root, "wt-feature")
	wtGitDir := filepath.Join(mono, ".git", "worktrees", "wt-feature")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wtPkg := filepath.Join(wt, "package.json")
	if err := os.WriteFile(wtPkg, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(wtPkg, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtHead := filepath.Join(wtGitDir, "HEAD")
	if err := os.WriteFile(wtHead, []byte("ref: refs/heads/feat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(wtHead, recent, recent); err != nil {
		t.Fatal(err)
	}
	if got := changed(Project{Dir: wt, Files: []string{"package.json"}}); !got.Equal(recent) {
		t.Errorf("worktree changed = %v, want %v", got, recent)
	}
}

func TestInstallsEnclosingProjectRoot(t *testing.T) {
	home := t.TempDir()
	mkFile := func(rel string) {
		t.Helper()
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkDir := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(home, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Dotfiles repo at $HOME/.git must never swallow standalone projects under $HOME.
	mkDir(".git")

	// 1. Git monorepo with root node_modules and nested subpackages.
	mkDir("dev/moduix/.git")
	mkFile("dev/moduix/package.json")
	mkDir("dev/moduix/node_modules")
	mkFile("dev/moduix/packages/solid/package.json")
	mkDir("dev/moduix/packages/solid/node_modules")
	mkDir("dev/moduix/packages/solid/dist")
	mkFile("dev/moduix/playgrounds/vue/package.json")
	mkDir("dev/moduix/playgrounds/vue/node_modules")

	// 2. Repo with nested website subpackage.
	mkDir("dev/hearth/.git")
	mkFile("dev/hearth/website/package.json")
	mkDir("dev/hearth/website/node_modules")

	// 3. Non-git workspace using pnpm-workspace.yaml.
	mkFile("dev/ws/pnpm-workspace.yaml")
	mkFile("dev/ws/apps/admin/package.json")
	mkDir("dev/ws/apps/admin/node_modules")

	// 4. Standalone project without .git (only $HOME/.git above it).
	mkFile("dev/standalone/package.json")
	mkDir("dev/standalone/node_modules")

	got, err := InstallsWithOptions(context.Background(), home, InstallsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{}
	for _, in := range got {
		relProj, _ := filepath.Rel(home, in.Project)
		key := filepath.ToSlash(relProj) + "/" + in.Name
		relRoot := ""
		if in.Root != "" {
			r, _ := filepath.Rel(home, in.Root)
			relRoot = filepath.ToSlash(r)
		}
		roots[key] = relRoot
	}

	want := map[string]string{
		"dev/moduix/node_modules":                 "",
		"dev/moduix/packages/solid/node_modules":  "dev/moduix",
		"dev/moduix/packages/solid/dist":          "dev/moduix",
		"dev/moduix/playgrounds/vue/node_modules": "dev/moduix",
		"dev/hearth/website/node_modules":         "dev/hearth",
		"dev/ws/apps/admin/node_modules":          "dev/ws",
		"dev/standalone/node_modules":             "",
	}
	for k, wantRoot := range want {
		gotRoot, ok := roots[k]
		if !ok {
			t.Errorf("missing install %q (found: %v)", k, roots)
			continue
		}
		if gotRoot != wantRoot {
			t.Errorf("%s Root = %q, want %q", k, gotRoot, wantRoot)
		}
	}
}
