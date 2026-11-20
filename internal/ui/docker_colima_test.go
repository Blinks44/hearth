package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestColimaResourcesFlow(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"normal", 1280, 820}, {"minimum", 1000, 680}} {
		t.Run(size.name, func(t *testing.T) {
			a := NewApp()
			a.Home = home
			a.Snapshot = colimaSnapshot()
			d := a.Snapshot.Tools[0].Docker
			p := inventory.ColimaProfile{Name: "default", Status: "running", Runtime: "docker", Driver: "VZ", Mount: "virtiofs",
				Socket: "unix://" + home + "/.colima/default/docker.sock", CPU: 4, Memory: 8 * docker.ColimaGiB, Disk: 60 * docker.ColimaGiB}
			d.Colima = &inventory.ColimaState{Path: "/opt/homebrew/bin/colima", Brew: "/opt/homebrew/bin/brew", Profiles: []inventory.ColimaProfile{p}, Autostart: "off"}
			a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
				plan, err := build(nil, nil)
				done(plan, err)
			}
			var executed ops.Plan
			a.RunPlan = func(plan ops.Plan, _ func(ops.State)) func() {
				executed = plan
				return func() {}
			}
			a.router.Push(dockerPage)
			tt := ui.NewTester(a.View, size.w, size.h)
			tt.Advance(time.Second)
			hasTexts(t, tt, locales.ColimaRunning(), "Colima · ~/.colima/default/docker.sock · Managed by Hearth · current in docker CLI", locales.ColimaAutostartOff())
			screenshot(t, tt, "docker-colima-"+size.name)
			if edit, ok := tt.AccessByRole(ui.RoleButton, locales.ColimaEdit()); ok &&
				edit.Bounds.Y+edit.Bounds.H > float32(size.h) {
				tt.Scroll(float32(size.w-40), float32(size.h-90), 0, 220)
				tt.Advance(time.Second)
			}
			if err := tt.Click(locales.ColimaEdit()); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			hasTexts(t, tt, locales.ColimaResources(), locales.ColimaDiskGrowthHint())
			screenshot(t, tt, "docker-colima-resources-"+size.name)
			if len(executed.Steps) != 0 {
				t.Fatal("opening the editor executed a plan")
			}
			if err := tt.Click(locales.ColimaIncrease(locales.ColimaDiskGiB())); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if err := tt.Click(locales.ColimaRestart()); err != nil {
				t.Fatal(err)
			}
			tt.Advance(time.Second)
			if len(executed.Steps) != 2 || executed.Steps[1].Argv[len(executed.Steps[1].Argv)-1] != "61" {
				t.Fatalf("resize plan = %+v", executed.Steps)
			}
		})
	}
}

func TestColimaChoiceAndExternalEngine(t *testing.T) {
	if goos != "darwin" {
		t.Skip("OrbStack is a macOS engine")
	}
	for _, tc := range []struct {
		name    string
		clean   bool
		partial bool
	}{
		{"clean", true, false},
		{"external", false, false},
		{"partial", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NewApp()
			a.Snapshot = dockerSnapshot()
			st := &a.Snapshot.Tools[0]
			if tc.clean {
				st.Installations = nil
				st.Docker.Contexts = nil
				st.Docker.Selected = ""
				st.Docker.Plugins = nil
				st.Docker.Engines = []inventory.DockerEngine{{Name: "Colima"}, {Name: "OrbStack"}}
			}
			if tc.partial {
				st.Docker.Colima = &inventory.ColimaState{Path: "/opt/homebrew/bin/colima",
					Profiles: []inventory.ColimaProfile{{Name: "default", Status: "stopped"}}}
			}
			var planned bool
			a.BuildPlan = func(_ func(sysenv.Env, ops.Terminal) (ops.Plan, error), _ func(ops.Plan, error)) {
				planned = true
			}
			tt := openDocker(a)
			action := locales.ColimaMigrate()
			if tc.clean {
				action = locales.ColimaInstall()
				hasTexts(t, tt, locales.ColimaChoice(), locales.ColimaRecommended(), locales.ColimaOtherEngineHint())
			} else {
				hasTexts(t, tt, locales.ColimaOnlyHearth("OrbStack"), locales.ColimaSeparateDataHint())
			}
			screenshot(t, tt, "docker-colima-"+tc.name)
			if planned {
				t.Fatal("rendering built an installation plan")
			}
			if err := tt.Click(action); err != nil {
				t.Fatal(err)
			}
			if !planned {
				t.Fatal("click did not request an installation plan")
			}
		})
	}
}
