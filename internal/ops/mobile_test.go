package ops

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestRemoveAndroidImagePlan(t *testing.T) {
	sdk := t.TempDir()
	imgPath := filepath.Join(sdk, "system-images", "android-34", "google_apis", "arm64-v8a")
	if err := os.MkdirAll(imgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	img := inventory.AndroidPart{Name: "android-34/google_apis/arm64-v8a", API: 34, Path: imgPath}

	// Refuses when used by an AVD.
	used := img
	used.AVDs = []string{"Pixel_8"}
	if _, err := RemoveAndroidImagePlan(sdk, used, nil, nil); err == nil {
		t.Fatal("expected error when image is used by an AVD")
	}

	// Without sdkmanager, removes the image folder directly.
	pFolder, err := RemoveAndroidImagePlan(sdk, img, sysenv.Env{"PATH="}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(pFolder.Steps) != 1 || pFolder.Steps[0].Remove != imgPath {
		t.Fatalf("folder steps = %+v, want Remove %q", pFolder.Steps, imgPath)
	}

	// With sdkmanager in cmdline-tools/latest/bin, runs sdkmanager --uninstall.
	sdkmgr := filepath.Join(sdk, "cmdline-tools", "latest", "bin", sdkmanagerExe)
	if err := os.MkdirAll(filepath.Dir(sdkmgr), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sdkmgr, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	detect := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "mobile", Mobile: &inventory.MobileState{Android: &inventory.AndroidSDK{Root: sdk}}}, nil
	}
	pCmd, err := RemoveAndroidImagePlan(sdk, img, sysenv.Env{"PATH="}, detect)
	if err != nil {
		t.Fatal(err)
	}
	wantStep := sdkmanagerUninstallStep(sdkmgr, sdk, "system-images;android-34;google_apis;arm64-v8a")
	if len(pCmd.Steps) != 1 || !slices.Equal(pCmd.Steps[0].Argv, wantStep.Argv) || !slices.Equal(pCmd.Steps[0].Env, wantStep.Env) {
		t.Fatalf("cmd steps = %+v, want %+v", pCmd.Steps, wantStep)
	}
	f := &run.Fake{}
	f.Stdout("", wantStep.Argv...)
	if got := Execute(t.Context(), f, pCmd, &strings.Builder{}, func(State) {}); got.Phase != Succeeded {
		t.Fatalf("phase = %d, err = %q", got.Phase, got.Err)
	}
}
