package inventory

import "testing"

func TestTerminalCopy(t *testing.T) {
	mise := func(v string, rank PathRank, def bool) Installation {
		return Installation{Version: v, OnPath: rank, Default: def,
			Source: Source{Kind: KindVersionManager, Manager: "mise"}}
	}
	shim := Installation{OnPath: 1, Source: Source{Kind: KindVersionManager, Manager: "mise", Shim: true}}
	tests := []struct {
		name string
		ins  []Installation
		want int
	}{
		{"first in PATH", []Installation{mise("20", 2, false), mise("22", 1, false)}, 1},
		{"shim picks the default", []Installation{shim, mise("20", 0, false), mise("22", 0, true)}, 2},
		{"shim without a default", []Installation{shim, mise("22", 0, false)}, -1},
		{"not in PATH", []Installation{mise("22", 0, true)}, -1},
	}
	for _, tt := range tests {
		if got := TerminalCopy(tt.ins); got != tt.want {
			t.Errorf("%s: %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestSystemRuby(t *testing.T) {
	for _, tc := range []struct {
		tool string
		kind Kind
		want bool
	}{
		{"ruby", KindSystem, true}, {"ruby", KindPackageManager, false}, {"ruby", KindVersionManager, false}, {"node", KindSystem, false},
	} {
		if got := NeededBySystem(Installation{Tool: tc.tool, Source: Source{Kind: tc.kind}}); got != tc.want {
			t.Errorf("%s %v: %v", tc.tool, tc.kind, got)
		}
	}
}
