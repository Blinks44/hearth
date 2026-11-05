package pkgmgr

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestApplyLocalUpdates(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		format, fixture string
		installed       []inventory.SystemPackage
		want            []string
	}{
		{"zypper-updates", "zypper-updates.xml", []inventory.SystemPackage{
			{Name: "libtest", Version: "1.0-1", Architecture: "x86_64"},
			{Name: "libtest", Version: "1.0-1", Architecture: "i586"},
		}, []string{"2.0-1", ""}},
		{"apk-updates", "apk-updates.txt", []inventory.SystemPackage{
			{Name: "nodejs-current", Version: "22.1.0-r0"},
			{Name: "libtest", Version: "1.0_rc1-r2"},
			{Name: "unchanged", Version: "1.0-r0"},
		}, []string{"22.2.0-r1", "1.0-r0", ""}},
	} {
		t.Run(tt.format, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			m := inventory.PackageManager{Packages: slices.Clone(tt.installed)}
			if err := applyUpdates(&m, tt.format, data); err != nil {
				t.Fatal(err)
			}
			for i, p := range m.Packages {
				if p.Available != tt.want[i] || p.Version != tt.installed[i].Version {
					t.Fatalf("package %d: %+v", i, p)
				}
			}
		})
	}
}

func TestLocalUpdatesEmptyAndInvalid(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, format, data string
		invalid            bool
	}{
		{"apk empty", "apk-updates", "Installed: Available:\n", false},
		{"apk malformed", "apk-updates", "unexpected output", true},
		{"apk malformed package", "apk-updates", "nodejs < 2.0-r0", true},
		{"apk truncated", "apk-updates", "nodejs-1.0-r0 <", true},
		{"zypper empty", "zypper-updates", `<stream><update-status><update-list/></update-status></stream>`, false},
		{"zypper wrong document", "zypper-updates", `<stream><message>Something else</message></stream>`, true},
		{"zypper warning", "zypper-updates", `<stream><message type="warning">No repositories defined.</message><update-status><update-list/></update-status></stream>`, true},
		{"zypper truncated", "zypper-updates", `<stream><update-status>`, true},
		{"zypper missing edition", "zypper-updates", `<stream><update-status><update-list><update name="test"/></update-list></update-status></stream>`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(tt.format, []byte(tt.data))
			if (err != nil) != tt.invalid {
				t.Fatalf("error = %v, want invalid %v", err, tt.invalid)
			}
		})
	}
}
