package ops

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestRustTargetPlan(t *testing.T) {
	tests := []struct {
		name      string
		add       bool
		toolchain string
		target    string
		wantArgv  []string
		wantErr   bool
	}{
		{
			name:      "add wasm32 target",
			add:       true,
			toolchain: "stable-aarch64-apple-darwin",
			target:    "wasm32-unknown-unknown",
			wantArgv:  []string{"rustup", "target", "add", "--toolchain", "stable-aarch64-apple-darwin", "wasm32-unknown-unknown"},
		},
		{
			name:      "remove wasm32 target",
			add:       false,
			toolchain: "stable-aarch64-apple-darwin",
			target:    "wasm32-unknown-unknown",
			wantArgv:  []string{"rustup", "target", "remove", "--toolchain", "stable-aarch64-apple-darwin", "wasm32-unknown-unknown"},
		},
		{
			name:      "reject unsafe target",
			add:       true,
			toolchain: "stable",
			target:    "wasm32;rm -rf",
			wantErr:   true,
		},
		{
			name:      "reject empty toolchain",
			add:       true,
			toolchain: "",
			target:    "wasm32-unknown-unknown",
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := RustTargetPlan(tt.add, tt.toolchain, tt.target, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, tt.wantArgv) {
				t.Errorf("argv = %v, want %v", p.Steps, tt.wantArgv)
			}
		})
	}
}

func TestRustTargetPlanVerify(t *testing.T) {
	tc := "stable-aarch64-apple-darwin"
	target := "wasm32-unknown-unknown"
	detectWith := func(targets []string) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			return inventory.ToolState{
				Tool: "rust",
				Installations: []inventory.Installation{{
					Tool:   "rust",
					Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup", Version: tc},
					Rust:   &inventory.RustToolchain{Name: tc, Targets: targets},
				}},
			}, nil
		}
	}
	pAdd, err := RustTargetPlan(true, tc, target, detectWith([]string{"aarch64-apple-darwin", target}))
	if err != nil {
		t.Fatal(err)
	}
	f := &run.Fake{}
	f.Stdout("", pAdd.Steps[0].Argv...)
	if got := Execute(t.Context(), f, pAdd, &strings.Builder{}, func(State) {}); got.Phase != Succeeded {
		t.Fatalf("add phase = %d, err = %q", got.Phase, got.Err)
	}

	pRm, err := RustTargetPlan(false, tc, target, detectWith([]string{"aarch64-apple-darwin"}))
	if err != nil {
		t.Fatal(err)
	}
	f2 := &run.Fake{}
	f2.Stdout("", pRm.Steps[0].Argv...)
	if got := Execute(t.Context(), f2, pRm, &strings.Builder{}, func(State) {}); got.Phase != Succeeded {
		t.Fatalf("remove phase = %d, err = %q", got.Phase, got.Err)
	}

	pFail, _ := RustTargetPlan(true, tc, target, detectWith([]string{"aarch64-apple-darwin"}))
	if _, err := pFail.Verify(t.Context()); err == nil {
		t.Error("expected verify error when target was not added")
	}
}

func TestVersionPlanVerifyRust(t *testing.T) {
	found := []inventory.Installation{{
		Tool:    "rust",
		Version: "1.85.0",
		Default: true,
		Source:  inventory.Source{Kind: inventory.KindVersionManager, Manager: "rustup", Version: "stable-aarch64-apple-darwin"},
		Rust:    &inventory.RustToolchain{Name: "stable-aarch64-apple-darwin"},
	}}
	detect := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "rust", Installations: found}, nil
	}
	p, ok := VersionPlan(VersionInstall, VersionTarget{Tool: "rust", Title: "Rust", Manager: "rustup", Version: "stable"}, detect)
	if !ok {
		t.Fatal("VersionPlan returned !ok for rustup")
	}
	f := &run.Fake{}
	f.Stdout("", p.Steps[0].Argv...)
	if got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {}); got.Phase != Succeeded {
		t.Fatalf("phase = %d, err = %q", got.Phase, got.Err)
	}
}
