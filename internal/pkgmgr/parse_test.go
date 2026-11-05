package pkgmgr

import (
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, format, input, want, version string }{
		{"apt installed", "apt", "Listing...\nnodejs/stable,now 22.1 amd64 [installed]\n", "nodejs", "22.1"},
		{"apt update", "apt", "Listing...\nnodejs/stable 24.0 amd64 [upgradable from: 22.1]\n", "nodejs", "24.0"},
		{"dnf", "rpm", "Installed Packages\nnodejs.x86_64 1:22.1-1 @fedora\n", "nodejs", "1:22.1-1"},
		{"pacman update", "pairs", "nodejs 22.1 -> 24.0\n", "nodejs", "24.0"},
		{"flatpak", "tsv", "org.app.Editor\t2.0\tuser\n", "org.app.Editor", "2.0"},
		{"choco", "choco", "Chocolatey v2.0\nnodejs|22.1\n", "nodejs", "22.1"},
		{"snap", "snap", "Name Version Rev Tracking Publisher Notes\nnode 22.1 100 latest/stable publisher -\n", "node", "22.1"},
		{"apk hyphenated name", "apk", "nodejs-current-22.1.0-r0\n", "nodejs-current", "22.1.0-r0"},
		{"nix", "nix", `{"key":{"name":"nodejs-22.1","pname":"nodejs","version":"22.1","outputs":{"out":"/nix/store/node"}}}`, "nodejs", "22.1"},
		{"zypper", "zypper", `<stream><search-result><solvable-list><solvable name="nodejs" edition="22.1"/></solvable-list></search-result></stream>`, "nodejs", "22.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parse(tt.format, []byte(tt.input))
			if err != nil || len(got) != 1 {
				t.Fatalf("%+v, %v", got, err)
			}
			if got[0].Name != tt.want || got[0].Version != tt.version {
				t.Fatalf("%+v", got[0])
			}
		})
	}
	for _, format := range []string{"nix", "zypper"} {
		t.Run(format+" malformed", func(t *testing.T) {
			if _, err := parse(format, []byte("broken")); err == nil {
				t.Fatal("malformed data accepted")
			}
		})
	}
}

func TestExitOK(t *testing.T) {
	t.Parallel()
	dnf := spec{installed: []string{"list"}, updates: []string{"check-update"}, updatesExit: 100}
	pacman := spec{installed: []string{"-Q"}, updates: []string{"-Qu"}, updatesExit: 1}
	tests := []struct {
		name string
		s    spec
		args []string
		code int
		want bool
	}{
		{"success", dnf, []string{"list"}, 0, true},
		{"dnf updates exist", dnf, []string{"check-update"}, 100, true},
		{"dnf error", dnf, []string{"check-update"}, 1, false},
		{"dnf 100 outside updates", dnf, []string{"list"}, 100, false},
		{"pacman no updates", pacman, []string{"-Qu"}, 1, true},
		{"pacman installed fails", pacman, []string{"-Q"}, 1, false},
		{"no updates query", spec{}, []string{"--version"}, 1, false},
	}
	for _, tt := range tests {
		if got := tt.s.exitOK(tt.args, tt.code); got != tt.want {
			t.Errorf("%s: got %v", tt.name, got)
		}
	}
}
