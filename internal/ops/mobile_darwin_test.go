package ops

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestRemoveSimRuntimePlanDarwin(t *testing.T) {
	rt := inventory.SimRuntime{
		Platform: "iOS",
		Version:  "17.5",
		Path:     "/Library/Developer/CoreSimulator/Volumes/iOS_21F79/Library/Developer/CoreSimulator/Profiles/Runtimes/iOS 17.5.simruntime",
		ID:       "UUID-175",
	}
	detectGone := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "mobile", Mobile: &inventory.MobileState{}}, nil
	}
	p, err := RemoveSimRuntimePlan(t.Context(), nil, rt, nil, detectGone)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"xcrun", "simctl", "runtime", "delete", "UUID-175"}
	if len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, want) {
		t.Fatalf("steps = %+v, want %v", p.Steps, want)
	}
	f := &run.Fake{}
	f.Stdout("", want...)
	if got := Execute(t.Context(), f, p, &strings.Builder{}, func(State) {}); got.Phase != Succeeded {
		t.Fatalf("phase = %d, err = %q", got.Phase, got.Err)
	}

	// Without rt.ID, resolves UUID via xcrun simctl runtime list -j.
	rtNoID := rt
	rtNoID.ID = ""
	r := &run.Fake{}
	r.Stdout(`{"UUID-JSON":{"identifier":"UUID-JSON","runtimeBundlePath":"`+rt.Path+`","version":"17.5"}}`, "xcrun", "simctl", "runtime", "list", "-j")
	p2, err := RemoveSimRuntimePlan(t.Context(), r, rtNoID, nil, detectGone)
	if err != nil {
		t.Fatal(err)
	}
	want2 := []string{"xcrun", "simctl", "runtime", "delete", "UUID-JSON"}
	if len(p2.Steps) != 1 || !slices.Equal(p2.Steps[0].Argv, want2) {
		t.Fatalf("resolved steps = %+v, want %v", p2.Steps, want2)
	}

	// Legacy bundle fallback when under Profiles/Runtimes and no ID uses elevated rm.
	legacy := inventory.SimRuntime{
		Platform: "watchOS",
		Version:  "10.5",
		Path:     "/Library/Developer/CoreSimulator/Profiles/Runtimes/watchOS 10.5.simruntime",
	}
	pLegacy, err := RemoveSimRuntimePlan(t.Context(), &run.Fake{}, legacy, nil, detectGone)
	if err != nil {
		t.Fatal(err)
	}
	wantLegacy := []string{"/bin/rm", "-rf", "--", legacy.Path}
	if len(pLegacy.Steps) != 1 || !slices.Equal(pLegacy.Steps[0].Argv, wantLegacy) || !pLegacy.Steps[0].Admin {
		t.Fatalf("legacy steps = %+v, want elevated %v", pLegacy.Steps, wantLegacy)
	}
	// Mounted volume without ID must not fall back to deleting a path.
	mountedNoID := rt
	mountedNoID.ID = ""
	if _, err := RemoveSimRuntimePlan(t.Context(), &run.Fake{}, mountedNoID, nil, detectGone); err == nil {
		t.Fatal("mounted runtime without ID should fail")
	}
}
