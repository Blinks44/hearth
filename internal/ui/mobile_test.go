package ui

import (
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Xcode, the simulators, the Android SDK and Flutter with the space each
// takes; the older simulator and the caches as what can go, and the way
// to Cleanup's caches.
func TestMobilePage(t *testing.T) {
	a := NewApp()
	a.Home = home
	const gb = 1_000_000_000
	sdk := home + "/Library/Android/sdk"
	sims := "/Library/Developer/CoreSimulator/Volumes"
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(),
		Variables: []inventory.EnvironmentVariable{{Name: "JAVA_HOME", Terminal: home + "/.local/share/mise/installs/java/21"}},
		Tools: []inventory.ToolState{
			{Tool: "cpp", Cpp: &inventory.CppState{Base: &inventory.CppBase{ID: "xcode", Path: "/Applications/Xcode.app/Contents/Developer"}}},
			{Tool: "mobile", Mobile: &inventory.MobileState{
				Xcodes: []inventory.XcodeApp{{Path: "/Applications/Xcode.app", Version: "16.2", Build: "16C5032a", Selected: true, AppStore: true}},
				Runtimes: []inventory.SimRuntime{
					{Platform: "iOS", Version: "17.5", Path: sims + "/iOS_21F79/iOS 17.5.simruntime"},
					{Platform: "iOS", Version: "18.2", Path: sims + "/iOS_22C150/iOS 18.2.simruntime"},
				},
				Android: &inventory.AndroidSDK{Root: sdk, Var: "ANDROID_HOME", Studio: "/Applications/Android Studio.app",
					Platforms: []inventory.AndroidPart{{Name: "android-35", API: 35, Path: sdk + "/platforms/android-35"}},
					Images:    []inventory.AndroidPart{{Name: "android-34/google_apis/arm64-v8a", API: 34, Path: sdk + "/system-images/android-34/google_apis/arm64-v8a", AVDs: []string{"Pixel_8"}}}},
				CocoaPods: &inventory.Installation{Tool: "pod", Version: "1.16.2", Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "gem"}},
				Flutter: &inventory.Flutter{Tool: "flutter", Version: "3.27.1",
					Source: inventory.Source{Kind: inventory.KindVersionManager, Manager: "mise"}, Root: home + "/.local/share/mise/installs/flutter/3.27.1", Channel: "stable"},
			}},
		}}
	a.Sizes = &inventory.Sizes{Taken: time.Now(),
		Caches: []inventory.Cache{{ID: "gradle", Path: home + "/.gradle/caches"}, {ID: "xcode", Path: home + "/Library/Developer/Xcode/DerivedData"},
			{ID: "cocoapods", Path: home + "/Library/Caches/CocoaPods"}},
		Paths: map[string]inventory.Usage{
			"/Applications/Xcode.app":                               {Bytes: 11.8 * gb},
			sims + "/iOS_22C150/iOS 18.2.simruntime":                {Bytes: 8.1 * gb},
			sims + "/iOS_21F79/iOS 17.5.simruntime":                 {Bytes: 7.4 * gb},
			home + "/Library/Developer/Xcode/DerivedData":           {Bytes: 3.1 * gb},
			home + "/Library/Caches/CocoaPods":                      {Bytes: 1.2 * gb},
			sdk + "/platforms/android-35":                           {Bytes: 6.2 * gb},
			sdk + "/system-images/android-34/google_apis/arm64-v8a": {Bytes: 5.1 * gb},
			home + "/.gradle/caches":                                {Bytes: 3.5 * gb},
			home + "/.local/share/mise/installs/flutter/3.27.1":     {Bytes: 0.9 * gb},
		}}
	a.router.Push(mobilePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "mobile")
	hasTexts(t, tt, "Mobile development", "Tools · iOS and Android", "IOS · XCODE", "Xcode 16.2", "App Store · xcode-select points here", "11.8 GB",
		"iOS simulator 18.2", "the newest iOS", "iOS simulator 17.5", "older than iOS 18.2", "DerivedData", "build cache · rebuilt by itself",
		"CocoaPods 1.16.2", "gem · specs cache", "ANDROID", "Android SDK · platform 35", "Android Studio · ANDROID_HOME set",
		"Emulator image API 34", "google_apis · arm64-v8a · AVD Pixel_8", "Gradle cache", "FLUTTER", "Flutter 3.27.1", "mise · stable channel",
		"API 35", "Environment", "ANDROID_HOME", "~/Library/Android/sdk · folder exists", "JAVA_HOME for Gradle", "~/.local/share/mise/installs/java/21",
		"xcode-select", "Can be removed", "Older simulator", "7.4 GB · iOS 17.5", "Xcode DerivedData", "Open in Cleanup")
	if tt.HasText("Android SDK, emulator and JDK") {
		t.Error("install links shown with everything found")
	}
	a.mobileTab = 2 // Android
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Xcode 16.2") || !tt.HasText("Emulator image API 34") {
		t.Errorf("android tab: %q", tt.Texts())
	}
	if err := tt.Click("Open in Cleanup"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != cleanupPage || cleanupGroups[a.cleanupTab-1] != cleanup.Caches {
		t.Fatalf("at %s, tab %d", a.router.Path(), a.cleanupTab)
	}
}

// Without the tools, the official install pages with what each brings;
// Xcode only where it installs.
func TestMobileInstallLinks(t *testing.T) {
	a := NewApp()
	a.Home = home
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{Tool: "mobile", Mobile: &inventory.MobileState{}}}}
	a.router.Push(mobilePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "mobile-install")
	hasTexts(t, tt, "No Xcode, Android SDK or Flutter found.", "Install", "Android Studio", "Android SDK, emulator and JDK",
		"Flutter", "iOS and Android from one code", "Open ↗")
	if got := tt.HasText("App Store · iOS SDK and simulators"); got != xcodeHere {
		t.Errorf("Xcode link shown %v, want %v", got, xcodeHere)
	}
}

func TestMobileSearchAndRemove(t *testing.T) {
	sdk := t.TempDir()
	usedImg := sdk + "/system-images/android-34/google_apis/arm64-v8a"
	unusedImg := sdk + "/system-images/android-33/google_apis/arm64-v8a"
	sims := "/Library/Developer/CoreSimulator/Volumes"

	sh := &fakeShell{}
	a := NewApp()
	a.Home = home
	a.Shell = sh
	a.BuildPlan = func(build func(sysenv.Env, ops.Terminal) (ops.Plan, error), done func(ops.Plan, error)) {
		done(build(sysenv.Env{"HOME=" + home}, nil))
	}
	a.Snapshot = &inventory.Snapshot{Taken: time.Now(), Tools: []inventory.ToolState{{Tool: "mobile", Mobile: &inventory.MobileState{
		Runtimes: []inventory.SimRuntime{
			{ID: "SIM-175", Platform: "iOS", Version: "17.5", Path: sims + "/iOS_21F79/iOS 17.5.simruntime"},
			{ID: "SIM-182", Platform: "iOS", Version: "18.2", Path: sims + "/iOS_22C150/iOS 18.2.simruntime"},
		},
		Android: &inventory.AndroidSDK{
			Root:      sdk,
			Platforms: []inventory.AndroidPart{{Name: "android-35", API: 35, Path: sdk + "/platforms/android-35"}},
			Images: []inventory.AndroidPart{
				{Name: "android-34/google_apis/arm64-v8a", API: 34, Path: usedImg, AVDs: []string{"Pixel_8"}},
				{Name: "android-33/google_apis/arm64-v8a", API: 33, Path: unusedImg},
			},
		},
	}}}}
	a.router.Push(mobilePage)
	tt := ui.NewTester(a.View, 1280, 820)
	tt.Advance(time.Second)
	screenshot(t, tt, "mobile-actions")
	ttNarrow := ui.NewTester(a.View, 1000, 680)
	ttNarrow.Advance(time.Second)
	screenshot(t, ttNarrow, "mobile-actions-narrow")

	// Search matches AVD name in the row's subtitle.
	a.mobileFilter = "Pixel_8"
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText("Emulator image API 34") || tt.HasText("Emulator image API 33") {
		t.Fatalf("AVD filter failed: %q", tt.Texts())
	}

	// Copy path action works on filtered row.
	if err := tt.Click("Copy path"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := tt.Clipboard(); got != usedImg {
		t.Errorf("clipboard = %q, want %q", got, usedImg)
	}

	// Unused Android image shows under Can be removed and proposes removal.
	hasTexts(t, tt, "Unused emulator image", "android-33/google_apis/arm64-v8a")
	if runtime.GOOS == "darwin" {
		// On macOS, clicking Remove… on the older simulator proposes simctl delete.
		if err := tt.Click("Remove…"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
		if !tt.HasText("xcrun simctl runtime delete SIM-175") {
			t.Fatalf("expected simctl delete plan: %q", tt.Texts())
		}
		if err := tt.Click("Cancel"); err != nil {
			t.Fatal(err)
		}
		tt.Advance(time.Second)
	}

	// Remove unused Android image directly when no older simulator is listed.
	a.Snapshot.Tools[0].Mobile.Runtimes = a.Snapshot.Tools[0].Mobile.Runtimes[1:]
	tt.Frame()
	if err := tt.Click("Remove…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("Remove emulator image API 33") {
		t.Fatalf("expected Android image removal plan: %q", tt.Texts())
	}
}
