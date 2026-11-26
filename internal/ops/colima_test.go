package ops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestColimaPluginEdit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, config string
		want         []string
		block        bool
	}{
		{"new", "", []string{"/brew/plugins"}, false},
		{"keep existing configuration", `{"auths":{"example.test":{"auth":"FAKE-SECRET"}},"currentContext":"orbstack"}`, []string{"/brew/plugins"}, false},
		{"append", `{"cliPluginsExtraDirs":["/existing"]}`, []string{"/existing", "/brew/plugins"}, false},
		{"already present", `{"cliPluginsExtraDirs":["/brew/plugins"]}`, nil, false},
		{"malformed", `{"auths":`, nil, true},
		{"wrong type", `{"cliPluginsExtraDirs":"other"}`, nil, true},
		{"ambiguous", `{"cliPluginsExtraDirs":[],"cliPluginsExtraDirs":[]}`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			env := sysenv.Env{"DOCKER_CONFIG=" + dir}
			path := filepath.Join(dir, "config.json")
			if tc.config != "" {
				if err := os.WriteFile(path, []byte(tc.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			edit, err := colimaPluginEdit(env, "/brew/plugins")
			switch {
			case tc.block:
				if err == nil || edit != nil {
					t.Fatalf("bad config accepted: %+v, %v", edit, err)
				}
			case tc.want == nil:
				if err != nil || edit != nil {
					t.Fatalf("already configured: %+v, %v", edit, err)
				}
			default:
				if err != nil || edit == nil || edit.Path != path || edit.Before != tc.config {
					t.Fatalf("edit = %+v, %v", edit, err)
				}
				var parsed struct{ CliPluginsExtraDirs []string }
				if err := json.Unmarshal([]byte(edit.After), &parsed); err != nil || !slices.Equal(parsed.CliPluginsExtraDirs, tc.want) {
					t.Fatalf("new config %s: %v", edit.After, err)
				}
				if strings.Contains(tc.config, "FAKE-SECRET") {
					if !strings.Contains(edit.After, `"auths":{"example.test":{"auth":"FAKE-SECRET"}},"currentContext":"orbstack"`) ||
						strings.Contains(edit.Masked(edit.After), "FAKE-SECRET") || strings.Contains(edit.Masked(edit.Before), "FAKE-SECRET") {
						t.Fatalf("config changed outside plugin entry or preview exposed auth")
					}
				}
				backup, err := applyEdit(*edit, time.Now())
				if err != nil || (backup == "") != edit.New {
					t.Fatalf("apply: %q, %v", backup, err)
				}
				if backup != "" {
					data, err := os.ReadFile(backup)
					if err != nil || string(data) != tc.config {
						t.Fatalf("backup differs: %v", err)
					}
				}
			}
		})
	}
}

func TestColimaInstallPlan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Colima runs on macOS and Linux")
	}
	t.Parallel()
	brew := inventory.PackageManager{Path: "/opt/brew-launchers/brew", Prefix: "/opt/brew"}
	env := sysenv.Env{"HOME=" + t.TempDir(), "PATH="}
	var r run.Fake
	plan, err := ColimaInstallPlan(brew, true, env, &r)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[1].Edit == nil || len(r.Calls()) != 0 {
		t.Fatalf("steps = %+v, calls = %+v", plan.Steps, r.Calls())
	}
	for _, pkg := range []string{"docker", "colima", "docker-compose", "docker-buildx"} {
		if !slices.Contains(plan.Steps[0].Argv, pkg) {
			t.Errorf("missing %s: %v", pkg, plan.Steps[0].Argv)
		}
	}
	if want := append([]string{"/opt/brew/bin/colima", "start", "--profile", "default", "--ssh-config=false"}, colimaFirstStart()...); !slices.Equal(plan.Steps[2].Argv, want) {
		t.Errorf("start = %v, want %v", plan.Steps[2].Argv, want)
	}
	for _, want := range []string{
		locales.DockerSwitchUntouchedTitle(),
		locales.DockerSwitchAutostartTitle(),
		locales.DockerSwitchRollbackTitle(),
	} {
		if !slices.ContainsFunc(plan.Notes, func(n Note) bool { return n.Title == want }) {
			t.Errorf("Colima migration plan missing consequence note %q: %+v", want, plan.Notes)
		}
	}
	cleanPlan, err := ColimaInstallPlan(brew, false, env, &r)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(cleanPlan.Notes, func(n Note) bool { return n.Title == locales.DockerSwitchAutostartTitle() }) {
		t.Errorf("clean Colima plan should not include switch-only autostart note: %+v", cleanPlan.Notes)
	}
	r.Stdout(`{"runtime":"docker","docker_socket":"unix:///tmp/default/docker.sock"}`, "/opt/brew/bin/colima", "status", "--profile", "default", "--json")
	target := docker.Target{Docker: "/opt/brew/bin/docker", Endpoint: "unix:///tmp/default/docker.sock"}
	for _, cmd := range [][]string{{"version", "--format", "{{.Server.Version}}"}, {"compose", "version"}, {"buildx", "version"}} {
		r.Stdout("version", target.Command(nil, cmd...).Args...)
	}
	if result, err := plan.Verify(t.Context()); err != nil || result == "" {
		t.Fatalf("verification = %s, %v", result, err)
	}
}

func TestColimaInstallPlanSelectsStandaloneDocker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Colima runs on macOS and Linux")
	}
	for _, tc := range []struct {
		name       string
		standalone bool
	}{
		{"OrbStack is the only Docker CLI", false},
		{"an independent CLI follows OrbStack in PATH", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			root := t.TempDir()
			links := filepath.Join(root, "links")
			app := filepath.Join(home, "Applications", "OrbStack.app", "Contents", "bin")
			system := filepath.Join(root, "system", "bin")
			prefix := filepath.Join(root, "brew")
			for _, dir := range []string{links, app, system} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			bundled := filepath.Join(app, "docker")
			if err := os.WriteFile(bundled, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(bundled, filepath.Join(links, "docker")); err != nil {
				t.Skipf("symbolic links unavailable: %v", err)
			}
			exe := filepath.Join(prefix, "bin", "docker")
			if tc.standalone {
				exe = filepath.Join(system, "docker")
				if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			env := sysenv.Env{"HOME=" + home, "PATH=" + strings.Join([]string{links, system}, string(filepath.ListSeparator))}
			manager := inventory.PackageManager{Path: filepath.Join(prefix, "bin", "brew")}
			var r run.Fake
			plan, err := ColimaInstallPlan(manager, true, env, &r)
			if err != nil {
				t.Fatal(err)
			}
			if got := slices.Contains(plan.Steps[0].Argv, "docker"); got == tc.standalone {
				t.Errorf("install docker = %t for PATH %q", got, env.Get("PATH"))
			}
			start := plan.Steps[len(plan.Steps)-1]
			if dirs := filepath.SplitList(sysenv.Env(start.Env).Get("PATH")); len(dirs) == 0 || dirs[0] != filepath.Dir(exe) {
				t.Errorf("Colima's PATH = %v, want %s first", dirs, filepath.Dir(exe))
			}
			// The old application's link is gone when verification runs.
			if err := os.Remove(filepath.Join(links, "docker")); err != nil {
				t.Fatal(err)
			}
			r.Stdout(`{"runtime":"docker","docker_socket":"unix:///tmp/default/docker.sock"}`, start.Argv[0], "status", "--profile", "default", "--json")
			target := docker.Target{Docker: exe, Endpoint: "unix:///tmp/default/docker.sock"}
			for _, cmd := range [][]string{{"version", "--format", "{{.Server.Version}}"}, {"compose", "version"}, {"buildx", "version"}} {
				r.Stdout("version", target.Command(nil, cmd...).Args...)
			}
			if result, err := plan.Verify(t.Context()); err != nil || result == "" {
				t.Fatalf("verification = %q, %v", result, err)
			}
		})
	}
}

func TestColimaResizePlan(t *testing.T) {
	t.Parallel()
	const colima = "/brew/bin/colima"
	var r run.Fake
	observed := inventory.ColimaState{Path: colima}
	profile := inventory.ColimaProfile{Name: "work", Status: "running", Runtime: "docker", CPU: 2, Memory: 2 * docker.ColimaGiB, Disk: 60 * docker.ColimaGiB}
	limits := ColimaLimits{CPU: 4, Memory: 3.5, Disk: 80}
	plan, err := ColimaPlan(&r, nil, observed, profile, ColimaResize, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || !slices.Equal(plan.Steps[0].Argv, []string{colima, "stop", "--profile", "work"}) ||
		!slices.Equal(plan.Steps[1].Argv, []string{colima, "start", "--profile", "work", "--activate=false", "--cpu", "4", "--memory", "3.5", "--disk", "80"}) {
		t.Fatalf("resize steps = %+v", plan.Steps)
	}
	r.Stdout(`{"name":"work","status":"Running","runtime":"docker","cpus":2,"memory":2147483648,"disk":64424509440}`, colima, "list", "--json")
	if err := plan.Before(t.Context()); err != nil {
		t.Fatal(err)
	}
	r.Stdout(`{"name":"work","status":"Running","runtime":"docker","cpus":4,"memory":3758096384,"disk":85899345920}`, colima, "list", "--json")
	if err := plan.Before(t.Context()); err == nil {
		t.Fatal("stale resize accepted")
	}
	if result, err := plan.Verify(t.Context()); err != nil || result == "" {
		t.Fatalf("verification = %s, %v", result, err)
	}
	limits.Disk = 40
	if _, err := ColimaPlan(&r, nil, observed, profile, ColimaResize, limits); err == nil {
		t.Fatal("disk shrink accepted")
	}
}

func TestColimaManagedResizeAndFirstStart(t *testing.T) {
	t.Parallel()
	const colima, brew = "/brew/bin/colima", "/brew/bin/brew"
	var r run.Fake
	observed := inventory.ColimaState{Path: colima, Brew: brew, Autostart: "on"}

	running := inventory.ColimaProfile{Name: "default", Status: "running", Runtime: "docker", CPU: 2, Memory: 2 * docker.ColimaGiB, Disk: 60 * docker.ColimaGiB}
	plan, err := ColimaPlan(&r, nil, observed, running, ColimaResize, ColimaLimits{CPU: 4, Memory: 4, Disk: 80})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 ||
		!slices.Equal(plan.Steps[0].Argv, []string{brew, "services", "stop", "colima"}) ||
		!slices.Equal(plan.Steps[1].Argv, []string{colima, "stop", "--profile", "default"}) ||
		!slices.Equal(plan.Steps[2].Argv, []string{colima, "start", "--profile", "default", "--activate=false", "--cpu", "4", "--memory", "4", "--disk", "80"}) ||
		!slices.Equal(plan.Steps[3].Argv, []string{colima, "stop", "--profile", "default"}) ||
		!slices.Equal(plan.Steps[4].Argv, []string{brew, "services", "start", "colima"}) {
		t.Fatalf("managed resize steps = %+v", plan.Steps)
	}

	first := inventory.ColimaProfile{Name: "default"}
	startPlan, err := ColimaPlan(&r, nil, observed, first, ColimaStart, ColimaLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(startPlan.Steps) != 3 || !slices.Contains(startPlan.Steps[0].Argv, "--ssh-config=false") ||
		!slices.Equal(startPlan.Steps[1].Argv, []string{colima, "stop", "--profile", "default"}) ||
		!slices.Equal(startPlan.Steps[2].Argv, []string{brew, "services", "start", "colima"}) {
		t.Fatalf("managed first start steps = %+v", startPlan.Steps)
	}
}

func TestSetupPrefersNativeDockerOnLinuxWithHomebrew(t *testing.T) {
	t.Parallel()
	setup := Setup{
		Tools: []string{"docker"}, GOOS: "linux",
		Managers: []inventory.PackageManager{
			{ID: "brew", Path: "/brew/bin/brew"},
			{ID: "apt", Path: "/usr/bin/apt-get"},
		},
		Title: func(id string) string { return id },
	}
	plan, parts, err := SetupPlan(setup, sysenv.Env{"HOME=" + t.TempDir(), "PATH=/usr/bin"})
	if err != nil || len(parts) != 1 || parts[0].Method.Engine != "Docker Engine" || parts[0].Method.Manager != "apt" {
		t.Fatalf("Linux with Homebrew: %+v, %+v, %v", parts, plan.Steps, err)
	}
	if len(plan.Steps) != 1 || !plan.Steps[0].Admin || !strings.Contains(strings.Join(plan.Steps[0].Argv, " "), "docker.io") {
		t.Fatalf("native Docker install = %+v", plan.Steps)
	}
}

func TestColimaTrimPlan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	disk := filepath.Join(dir, "colima-work", "disk")
	if err := os.MkdirAll(filepath.Dir(disk), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(disk, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	const cli = "/brew/bin/colima"
	var r run.Fake
	state := inventory.ColimaState{Path: cli}
	profile := inventory.ColimaProfile{Name: "work", Status: "running", Runtime: "docker"}
	r.Stdout(`{"name":"work","status":"Running","runtime":"docker"}`, cli, "list", "--json")
	plan, err := ColimaPlan(&r, sysenv.Env{"LIMA_HOME=" + dir}, state, profile, ColimaTrim, ColimaLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || !slices.Equal(plan.Steps[0].Argv, []string{cli, "ssh", "--profile", "work", "--", "sudo", "fstrim", "-a"}) {
		t.Fatalf("trim steps = %+v", plan.Steps)
	}
	if err := plan.Before(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Verify(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(disk); err != nil {
		t.Fatal(err)
	}
	if err := plan.Before(t.Context()); err == nil {
		t.Fatal("trim accepted after the disk disappeared")
	}
}

func TestColimaPlanDisclaim(t *testing.T) {
	t.Parallel()
	const cli = "/brew/bin/colima"
	const brew = "/brew/bin/brew"
	var r run.Fake
	state := inventory.ColimaState{Path: cli, Brew: brew, Autostart: "off"}

	stopped := inventory.ColimaProfile{Name: "default", Status: "stopped", Runtime: "docker", Disk: 60 * 1 << 30}
	startPlan, err := ColimaPlan(&r, nil, state, stopped, ColimaStart, ColimaLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(startPlan.Steps) != 1 || !startPlan.Steps[0].Disclaim {
		t.Fatalf("ColimaStart Disclaim = %v, want true", startPlan.Steps[0].Disclaim)
	}

	running := inventory.ColimaProfile{Name: "default", Status: "running", Runtime: "docker", Disk: 60 * 1 << 30}
	stopPlan, err := ColimaPlan(&r, nil, state, running, ColimaStop, ColimaLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(stopPlan.Steps) != 1 || stopPlan.Steps[0].Disclaim {
		t.Fatalf("ColimaStop Disclaim = %v, want false", stopPlan.Steps[0].Disclaim)
	}

	resizePlan, err := ColimaPlan(&r, nil, state, running, ColimaResize, ColimaLimits{CPU: 4, Memory: 6, Disk: 60})
	if err != nil {
		t.Fatal(err)
	}
	if len(resizePlan.Steps) != 2 || resizePlan.Steps[0].Disclaim || resizePlan.Steps[0].Undo == nil || !resizePlan.Steps[0].Undo.Disclaim || !resizePlan.Steps[1].Disclaim {
		t.Fatalf("ColimaResize steps Disclaim mismatch: %+v", resizePlan.Steps)
	}

	enablePlan, err := ColimaPlan(&r, nil, state, running, ColimaEnable, ColimaLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(enablePlan.Steps) != 1 || enablePlan.Steps[0].Disclaim {
		t.Fatalf("ColimaEnable (brew services) Disclaim = %v, want false", enablePlan.Steps[0].Disclaim)
	}
}

func TestExecuteDisclaimForwarding(t *testing.T) {
	t.Parallel()
	var r run.Fake
	r.Stdout("ok\n", "/opt/homebrew/bin/colima", "start")
	p := Plan{
		Steps: []Step{{Argv: []string{"/opt/homebrew/bin/colima", "start"}, Disclaim: true}},
	}
	st := Execute(t.Context(), &r, p, &strings.Builder{}, func(State) {})
	if st.Phase != Succeeded {
		t.Fatalf("phase = %v, err = %q", st.Phase, st.Err)
	}
	calls := r.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if !calls[0].Disclaim {
		t.Errorf("Disclaim was not forwarded to runner")
	}
	if home, err := os.UserHomeDir(); err == nil && calls[0].Dir != home {
		t.Errorf("Dir = %q, want home dir %q", calls[0].Dir, home)
	}
}
