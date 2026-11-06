package pkgmgr

import (
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/versions"
)

func TestNetworkFlatpak(t *testing.T) {
	t.Parallel()
	f := &run.Fake{}
	f.Stdout("org.example.Editor/x86_64/stable\t1.0\tuser\tflathub\told\tuser,current\norg.example.Editor/x86_64/beta\t1.0\tsystem\tother\told\tsystem\norg.example.Platform/x86_64/1\t\tuser\tflathub\tabc\tuser,runtime\n", "flatpak", "list", "--columns=ref,version,installation,origin,active,options")
	f.Stdout("app/org.example.Editor/x86_64/stable\t1.0\tnew-commit\tflathub\n", "flatpak", "remote-ls", "--user", "--updates", "--all", "--arch=*", "--columns=ref,version,commit,origin")
	f.Stdout("", "flatpak", "remote-ls", "--system", "--updates", "--all", "--arch=*", "--columns=ref,version,commit,origin")
	m := inventory.PackageManager{ID: "flatpak", Path: "flatpak"}
	got := Network(t.Context(), &versions.Cache{Dir: t.TempDir()}, f, nil, m)
	if !got.UpdatesKnown || got.UpdateError != "" || len(got.Packages) != 3 || got.Packages[0].Available != "new-commit" || got.Packages[1].Available != "" {
		t.Fatalf("%+v", got)
	}
	if got.Packages[2].Kind != "runtime" || got.Packages[2].Version != "abc" {
		t.Fatal(got.Packages)
	}
	if got.Packages[0].Reference == got.Packages[1].Reference || got.Packages[0].Scope != "user" {
		t.Fatal(got.Packages)
	}
}

func TestNetworkEmptyUpdates(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		id, output string
		args       []string
	}{
		{"choco", "", []string{"outdated", "--limit-output", "--no-color"}},
		{"snap", "All snaps up to date.\n", []string{"refresh", "--list"}},
	} {
		t.Run(tt.id, func(t *testing.T) {
			f := &run.Fake{}
			f.Stdout(tt.output, append([]string{tt.id}, tt.args...)...)
			m := inventory.PackageManager{ID: tt.id, Path: tt.id, Packages: []inventory.SystemPackage{{Name: "example", Version: "1.0", Available: "old"}}}
			got := Network(t.Context(), &versions.Cache{Dir: t.TempDir()}, f, nil, m)
			if !got.UpdatesKnown || got.UpdateError != "" || got.Packages[0].Available != "" {
				t.Fatalf("%+v", got)
			}
		})
	}
}
