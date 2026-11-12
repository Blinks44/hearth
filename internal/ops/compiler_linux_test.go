package ops

import (
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

// Each distribution's manager installs its own build packages as the
// administrator; snap and flatpak have none.
func TestCompilerPlanLinux(t *testing.T) {
	tests := []struct {
		manager inventory.PackageManager
		want    []string // nil: no plan
	}{
		{inventory.PackageManager{ID: "apt", Name: "APT", Path: "/usr/bin/apt"}, []string{"/usr/bin/apt-get", "-oDPkg::Lock::Timeout=600", "install", "-y", "build-essential"}},
		{inventory.PackageManager{ID: "dnf", Name: "DNF", Path: "/usr/bin/dnf"}, []string{"/usr/bin/dnf", "install", "-y", "gcc", "gcc-c++", "make"}},
		{inventory.PackageManager{ID: "zypper", Name: "Zypper", Path: "/usr/bin/zypper"}, []string{"/usr/bin/zypper", "--non-interactive", "install", "gcc", "gcc-c++", "make"}},
		{inventory.PackageManager{ID: "pacman", Name: "pacman", Path: "/usr/bin/pacman"}, []string{"/usr/bin/pacman", "-S", "--needed", "--noconfirm", "base-devel"}},
		{inventory.PackageManager{ID: "apk", Name: "APK", Path: "/sbin/apk"}, []string{"/sbin/apk", "add", "build-base"}},
		{inventory.PackageManager{ID: "snap", Name: "Snap", Path: "/usr/bin/snap"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.manager.ID, func(t *testing.T) {
			p, ok := CompilerPlan([]inventory.PackageManager{tt.manager}, nil)
			if ok != (tt.want != nil) {
				t.Fatalf("ok = %v", ok)
			}
			if ok && (len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tt.want) || !p.Steps[0].Admin || p.Title != "Install the C and C++ compiler") {
				t.Errorf("%q: %+v", p.Title, p.Steps)
			}
		})
	}
}
