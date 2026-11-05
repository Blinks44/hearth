package pkgmgr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
)

func TestNetworkNix(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	profile := filepath.Join(home, ".nix-profile")
	if err := os.Mkdir(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/nix-profile-v3.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "manifest.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err := parseNixProfile(data)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nix-env")
	nix := filepath.Join(filepath.Dir(path), "nix")
	f := &run.Fake{}
	f.Stdout(`{"version":"2.13.0","outPath":"/nix/store/new-hello-2.13.0"}`, nix, "eval", "--json", "--refresh", "--no-write-lock-file", "--option", "allow-import-from-derivation", "false", "--option", "accept-flake-config", "false", "--apply", `p: { version = p.version or ""; outPath = p.outPath or ""; }`, "--", "flake:nixpkgs#legacyPackages.x86_64-linux.hello")
	got := Network(t.Context(), &versions.Cache{Dir: t.TempDir()}, f, sysenv.Env{"HOME=" + home}, inventory.PackageManager{ID: "nix", Path: path, Packages: installed})
	if !got.UpdatesKnown || got.UpdateError != "" || got.Packages[0].Available != "2.13.0" {
		t.Fatalf("%+v", got)
	}
	if len(f.Calls()) != 1 {
		t.Fatal(f.Calls())
	}
}

func TestNetworkNixClassic(t *testing.T) {
	t.Parallel()
	f := &run.Fake{}
	f.Stdout(`{"hello":{"pname":"hello","version":"2.13.0"}}`, "nix-env", "--option", "allow-import-from-derivation", "false", "--query", "--available", "--json")
	m := inventory.PackageManager{ID: "nix", Path: "nix-env", Packages: []inventory.SystemPackage{{Name: "hello", Version: "2.12.1"}}}
	got := Network(t.Context(), &versions.Cache{Dir: t.TempDir()}, f, sysenv.Env{"HOME=" + t.TempDir()}, m)
	if !got.UpdatesKnown || got.UpdateError != "" || got.Packages[0].Available != "2.13.0" {
		t.Fatalf("%+v", got)
	}
}

func TestNixUpdateRefs(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, data, ref string
		invalid         bool
	}{
		{"v1 URI", `{"version":1,"elements":[{"storePaths":["/nix/store/p"],"originalUri":"flake:nixpkgs","attrPath":"hello"}]}`, "flake:nixpkgs#hello", false},
		{"v2 locked", `{"version":2,"elements":[{"storePaths":["/nix/store/p"],"url":"github:NixOS/nixpkgs/rev","attrPath":"hello"}]}`, "github:NixOS/nixpkgs/rev#hello", false},
		{"fixed store path", `{"version":3,"elements":{"hello":{"storePaths":["/nix/store/p"]}}}`, "", false},
		{"future version", `{"version":99,"elements":{}}`, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nixUpdateRefs([]byte(tt.data))
			if (err != nil) != tt.invalid {
				t.Fatalf("%v %v", got, err)
			}
			if !tt.invalid && got["/nix/store/p"] != tt.ref {
				t.Fatal(got)
			}
		})
	}
}
