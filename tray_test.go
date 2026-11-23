package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

func TestTrayFirstPort(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"", ""},
		{"5432/tcp", ""},
		{"0.0.0.0:5432->5432/tcp, [::]:5432->5432/tcp", ":5432"},
		{"127.0.0.1:8080->80/tcp, 0.0.0.0:8443->443/tcp", ":8080"},
	}
	for _, tc := range cases {
		if got := trayFirstPort(tc.raw); got != tc.want {
			t.Errorf("trayFirstPort(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestBuildTrayMenuItems(t *testing.T) {
	snap := &inventory.Snapshot{
		Tools: []inventory.ToolState{
			{
				Tool: "docker",
				Installations: []inventory.Installation{
					{Tool: "docker", Path: "/opt/homebrew/bin/docker", OnPath: 1},
				},
				Docker: &inventory.DockerState{
					Selected: "colima",
					Contexts: []inventory.DockerContext{
						{Name: "colima", Endpoint: "unix:///home/.colima/default/docker.sock", Local: true, Status: "available"},
						{Name: "orbstack", Endpoint: "unix:///home/.orbstack/run/docker.sock", Local: true},
					},
					Colima: &inventory.ColimaState{
						Path: "/opt/homebrew/bin/colima",
						Profiles: []inventory.ColimaProfile{
							{Name: "default", Runtime: "docker", Status: "running"},
						},
					},
				},
			},
			{
				Tool: "cli",
				Cli: &inventory.CliState{
					Tools: []inventory.Installation{
						{Tool: "kubectl", Path: "/opt/homebrew/bin/kubectl", OnPath: 1},
					},
					Contexts: []inventory.CliContext{
						{Tool: "kubectl", Name: "staging-n3"},
					},
					KubeContexts: []inventory.KubeContext{
						{Name: "kind-dev"},
						{Name: "staging-n3"},
						{Name: "shadowed-ctx", Shadowed: true},
					},
				},
			},
		},
	}

	containers := []docker.Container{
		{ID: "c1", Name: "/postgres-dev", Ports: "0.0.0.0:5432->5432/tcp"},
		{ID: "c2", Name: "redis-cache", Ports: "127.0.0.1:6379->6379/tcp"},
	}

	var switchedDocker, switchedKube string
	var appUpdateChecks int
	items := buildTrayMenuItems(snap, containers, false, "", trayActions{
		switchDockerCtx: func(name string) { switchedDocker = name },
		switchKubeCtx:   func(name string) { switchedKube = name },
		checkAppUpdate:  func() { appUpdateChecks++ },
	})

	byID := func(list []*mygo.MenuItem, id string) *mygo.MenuItem {
		for _, it := range list {
			if it.ID == id {
				return it
			}
		}
		return nil
	}

	if got := byID(items, "container-c1"); got == nil || got.Label != "postgres-dev" || got.Badge != ":5432" {
		t.Fatalf("postgres container item = %+v", got)
	}
	if col := byID(items, "colima"); col == nil || len(col.Submenu) != 2 {
		t.Fatalf("colima submenu = %+v", col)
	}
	ctxMenu := byID(items, "contexts")
	if ctxMenu == nil {
		t.Fatal("missing contexts submenu")
	}
	if byID(ctxMenu.Submenu, "ctx-kube-shadowed-ctx") != nil {
		t.Fatal("shadowed kubernetes context should not appear in tray menu")
	}
	colimaItem := byID(ctxMenu.Submenu, "ctx-docker-colima")
	orbItem := byID(ctxMenu.Submenu, "ctx-docker-orbstack")
	if colimaItem == nil || !colimaItem.Checked || orbItem == nil || orbItem.Checked {
		t.Fatalf("docker context radio states wrong: colima=%+v orb=%+v", colimaItem, orbItem)
	}
	orbItem.Click(orbItem, nil)
	if switchedDocker != "orbstack" {
		t.Fatalf("switchedDocker = %q, want orbstack", switchedDocker)
	}

	kindItem := byID(ctxMenu.Submenu, "ctx-kube-kind-dev")
	stagingItem := byID(ctxMenu.Submenu, "ctx-kube-staging-n3")
	if kindItem == nil || kindItem.Checked || stagingItem == nil || !stagingItem.Checked {
		t.Fatalf("kube context radio states wrong: kind=%+v staging=%+v", kindItem, stagingItem)
	}
	kindItem.Click(kindItem, nil)
	if switchedKube != "kind-dev" {
		t.Fatalf("switchedKube = %q, want kind-dev", switchedKube)
	}

	if byID(items, "updates") != nil {
		t.Fatal("tray menu should not include a separate tool updates item")
	}
	appUpdateItem := byID(items, "app-update")
	if appUpdateItem == nil || appUpdateItem.Label != locales.TrayCheckUpdates() {
		t.Fatalf("app-update item = %+v, want %q", appUpdateItem, locales.TrayCheckUpdates())
	}
	appUpdateItem.Click(appUpdateItem, nil)
	if appUpdateChecks != 1 {
		t.Fatalf("appUpdateChecks = %d, want 1", appUpdateChecks)
	}

	itemsWithUpdate := buildTrayMenuItems(snap, containers, false, "1.1.0", trayActions{})
	appUpdateAvail := byID(itemsWithUpdate, "app-update")
	if appUpdateAvail == nil || appUpdateAvail.Label != locales.AppUpdateAvailable("1.1.0") {
		t.Fatalf("app-update item with available version = %+v, want %q", appUpdateAvail, locales.AppUpdateAvailable("1.1.0"))
	}

	// Verify tray icon bytes are embedded and non-empty.
	png, _ := trayIcon()
	if len(png) < 100 || !slices.Equal(png[:4], []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("trayIcon returned invalid PNG (%d bytes)", len(png))
	}
}

func TestSyncDockIcon(t *testing.T) {
	t.Cleanup(func() { mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular) })

	cases := []struct {
		tray, dock, visible bool
		want                mygo.ActivationPolicy
	}{
		{true, true, true, mygo.ActivationPolicyRegular},
		{true, true, false, mygo.ActivationPolicyRegular},
		{true, false, true, mygo.ActivationPolicyRegular},
		{true, false, false, mygo.ActivationPolicyAccessory},
		{false, true, false, mygo.ActivationPolicyRegular},
		{false, false, false, mygo.ActivationPolicyRegular},
	}
	for _, tc := range cases {
		syncDockIcon(tc.tray, tc.dock, tc.visible)
		if got := mygo.App.ActivationPolicy(); got != tc.want {
			t.Errorf("syncDockIcon(tray=%v, dock=%v, visible=%v) policy = %q, want %q", tc.tray, tc.dock, tc.visible, got, tc.want)
		}
	}
}
