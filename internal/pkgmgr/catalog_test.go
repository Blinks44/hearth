package pkgmgr

import (
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestAvailableManagersNonEmpty(t *testing.T) {
	list := AvailableManagers()
	if len(list) == 0 {
		t.Fatal("AvailableManagers returned empty slice")
	}
	if PlatformLabel() == "" {
		t.Fatal("PlatformLabel is empty")
	}
	for _, m := range list {
		if m.ID == "" || m.Name == "" || m.WebsiteURL == "" {
			t.Errorf("incomplete catalog entry: %+v", m)
		}
	}
}

func TestUnusedDependencies(t *testing.T) {
	brew := inventory.PackageManager{
		ID:           "brew",
		ReasonsKnown: true,
		Packages: []inventory.SystemPackage{
			{Name: "node", Dependencies: []string{"icu4c"}},
			{Name: "icu4c", Dependency: true},
			{Name: "oniguruma", Dependency: true},
			{Name: "brotli", Dependency: true},
		},
	}
	got := UnusedDependencies(brew)
	var names []string
	for _, p := range got {
		names = append(names, p.Name)
	}
	if !slices.Equal(names, []string{"brotli", "oniguruma"}) {
		t.Fatalf("brew unused = %v, want [brotli oniguruma]", names)
	}

	apt := inventory.PackageManager{
		ID:           "apt",
		ReasonsKnown: true,
		Packages: []inventory.SystemPackage{
			{Name: "git"},
			{Name: "libfoo", Dependency: true, Orphan: true},
			{Name: "libbar", Dependency: true},
		},
	}
	got = UnusedDependencies(apt)
	if len(got) != 1 || got[0].Name != "libfoo" {
		t.Fatalf("apt unused = %+v, want [libfoo]", got)
	}
}
