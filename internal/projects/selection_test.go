package projects

import (
	"errors"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestSelectVersion(t *testing.T) {
	tests := []struct {
		name                string
		wants               []string
		installed, releases []string
		want                string
		have, fail          bool
	}{
		{name: "intersection", wants: []string{"22", ">=22.12"}, releases: []string{"24.0.0", "22.14.0", "22.10.0"}, want: "22.14.0"},
		{name: "installed before newer download", wants: []string{"22", ">=22.12"}, installed: []string{"22.12.0"}, releases: []string{"22.14.0"}, want: "22.12.0", have: true},
		{name: "different installed matches are not one common version", wants: []string{"18", ">=22"}, installed: []string{"18.20.8", "22.14.0"}, releases: []string{"24.0.0"}, fail: true},
		{name: "conflicting pins cannot fall back", wants: []string{"18.20.8", "22.14.0"}, fail: true},
		{name: "exact pin without list", wants: []string{"3.12.12", ">=3.12,<3.13"}, want: "3.12.12"},
		{name: "unknown requirement", wants: []string{"team-runtime"}, releases: []string{"22.14.0"}, fail: true},
		{name: "incomplete list stays unknown", wants: []string{"18"}, releases: []string{"24.0.0"}, fail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := Status{Tool: "node", Checked: true}
			for i, w := range tt.wants {
				st.Checks = append(st.Checks, Check{Tool: "node", Want: w, File: []string{".nvmrc", "package.json"}[i%2]})
			}
			for _, v := range tt.installed {
				st.Installations = append(st.Installations, inventory.Installation{Version: v})
			}
			got, err := SelectVersion(st, tt.releases, nil)
			if (err != nil) != tt.fail {
				t.Fatalf("selection %+v, error %v", got, err)
			}
			if !tt.fail && (got.Version != tt.want || (got.Installed != nil) != tt.have) {
				t.Fatalf("selection %+v", got)
			}
			if tt.name == "incomplete list stays unknown" && !errors.Is(err, ErrNoCommonVersion) {
				t.Fatal(err)
			}
		})
	}
}

func TestSelectVersionNeverTreatsUnknownOrDownloadAsReady(t *testing.T) {
	in := &inventory.Installation{Version: "22.14.0"}
	st := Status{Tool: "node", Terminal: in, Checks: []Check{{Want: "22", Met: true}}}
	got, _ := SelectVersion(st, []string{"22.14.0"}, nil)
	if got.Ready {
		t.Fatal("unchecked terminal is ready")
	}
	st.Checked = true
	st.Downloads = true
	got, _ = SelectVersion(st, []string{"22.14.0"}, nil)
	if got.Ready || got.Installed != nil {
		t.Fatalf("pending download is installed: %+v", got)
	}
	st.Downloads = false
	got, _ = SelectVersion(st, nil, nil)
	if !got.Ready {
		t.Fatalf("checked matching terminal not ready: %+v", got)
	}
	st.Checks = append(st.Checks, Check{Want: "team-version"})
	if _, err := SelectVersion(st, nil, nil); err == nil {
		t.Fatal("unknown additional requirement ignored")
	}
}

func TestSelectVersionPackageManager(t *testing.T) {
	tests := []struct {
		name      string
		checks    []Check
		installed []string
		wantVer   string
		wantHave  bool
		wantErr   bool
	}{
		{
			name:      "exact installed",
			checks:    []Check{{Tool: "pnpm", Want: "10.6.0", File: "package.json", Key: "packageManager"}},
			installed: []string{"10.6.0"},
			wantVer:   "10.6.0",
			wantHave:  true,
		},
		{
			name:      "newer in same major satisfies packageManager",
			checks:    []Check{{Tool: "pnpm", Want: "10.6.0", File: "package.json", Key: "packageManager"}},
			installed: []string{"10.12.1"},
			wantVer:   "10.12.1",
			wantHave:  true,
		},
		{
			name:      "different major falls back to exact requested version",
			checks:    []Check{{Tool: "pnpm", Want: "10.6.0", File: "package.json", Key: "packageManager"}},
			installed: []string{"9.15.0"},
			wantVer:   "10.6.0",
			wantHave:  false,
		},
		{
			name: "conflict with engines returns ErrNoCommonVersion",
			checks: []Check{
				{Tool: "pnpm", Want: "10.6.0", File: "package.json", Key: "packageManager"},
				{Tool: "pnpm", Want: "^9.0.0", File: "package.json", Key: "engines.pnpm"},
			},
			installed: []string{"10.6.0"},
			wantErr:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st := Status{Tool: "pnpm", Checked: true, Checks: tc.checks}
			for _, v := range tc.installed {
				st.Installations = append(st.Installations, inventory.Installation{Tool: "pnpm", Version: v})
			}
			got, err := SelectVersion(st, nil, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SelectVersion err=%v wantErr=%v (got=%+v)", err, tc.wantErr, got)
			}
			if tc.wantErr {
				if !errors.Is(err, ErrNoCommonVersion) {
					t.Fatalf("expected ErrNoCommonVersion, got %v", err)
				}
				return
			}
			if got.Version != tc.wantVer || (got.Installed != nil) != tc.wantHave {
				t.Fatalf("got %+v, want version=%q have=%v", got, tc.wantVer, tc.wantHave)
			}
		})
	}
}
