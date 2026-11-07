//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// The SDK ANDROID_HOME names, its platforms and images newest first, each
// image with the AVDs made of it; Flutter's version from its SDK's files
// without running it; pod's from pod --version.
func TestMobile(t *testing.T) {
	dir := root(t)
	sdk := filepath.Join(dir, "sdk")
	writeFile(t, filepath.Join(sdk, "platforms", "android-34", "source.properties"), "AndroidVersion.ApiLevel=34\n")
	writeFile(t, filepath.Join(sdk, "platforms", "android-35", "source.properties"), "Pkg.Desc=Android SDK Platform 35\nAndroidVersion.ApiLevel=35\n")
	image := filepath.Join(sdk, "system-images", "android-34", "google_apis", "arm64-v8a")
	writeFile(t, filepath.Join(image, "source.properties"), "AndroidVersion.ApiLevel=34\n")
	avd := filepath.Join(dir, ".android", "avd")
	writeFile(t, filepath.Join(avd, "Pixel_8.ini"), "avd.ini.encoding=UTF-8\npath="+filepath.Join(avd, "Pixel_8.avd")+"\n")
	writeFile(t, filepath.Join(avd, "Pixel_8.avd", "config.ini"), "hw.lcd.density=420\nimage.sysdir.1=system-images/android-34/google_apis/arm64-v8a/\n")
	bin := filepath.Join(dir, "bin")
	flutterRoot := filepath.Join(dir, "flutter")
	fakeExe(t, filepath.Join(flutterRoot, "bin", "flutter"))
	writeFile(t, filepath.Join(flutterRoot, "bin", "cache", "flutter.version.json"), `{"frameworkVersion": "3.27.1", "channel": "stable", "dartSdkVersion": "3.6.0"}`)
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(flutterRoot, "bin", "flutter"), filepath.Join(bin, "flutter")); err != nil {
		t.Fatal(err)
	}
	pod := fakeExe(t, filepath.Join(bin, "pod"))
	r := &run.Fake{}
	r.Stdout("1.16.2\n", pod, "--version")
	st, err := Mobile(t.Context(), r, sysenv.Env{"HOME=" + dir, "PATH=" + bin, "ANDROID_HOME=" + sdk}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := st.Mobile
	want := &inventory.AndroidSDK{Root: sdk, Var: "ANDROID_HOME",
		Platforms: []inventory.AndroidPart{
			{Name: "android-35", API: 35, Path: filepath.Join(sdk, "platforms", "android-35")},
			{Name: "android-34", API: 34, Path: filepath.Join(sdk, "platforms", "android-34")},
		},
		Images: []inventory.AndroidPart{{Name: "android-34/google_apis/arm64-v8a", API: 34, Path: image, AVDs: []string{"Pixel_8"}}},
	}
	if !reflect.DeepEqual(m.Android, want) {
		t.Errorf("android = %+v, want %+v", m.Android, want)
	}
	if f := m.Flutter; f == nil || f.Version != "3.27.1" || f.Channel != "stable" || f.Root != flutterRoot {
		t.Errorf("flutter = %+v", f)
	}
	if m.CocoaPods == nil || m.CocoaPods.Version != "1.16.2" {
		t.Errorf("cocoapods = %+v", m.CocoaPods)
	}
	for _, c := range r.Calls() {
		if filepath.Base(c.Args[0]) == "flutter" {
			t.Error("flutter was run")
		}
	}
}

// Without an SDK, Studio or a variable there is no Android; a variable
// naming no folder is told.
func TestMobileNoSDK(t *testing.T) {
	dir := root(t)
	st, err := Mobile(t.Context(), &run.Fake{}, sysenv.Env{"HOME=" + dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mobile.Android != nil || st.Mobile.Flutter != nil || st.Mobile.CocoaPods != nil {
		t.Errorf("found %+v", st.Mobile)
	}
	st, _ = Mobile(t.Context(), &run.Fake{}, sysenv.Env{"HOME=" + dir, "ANDROID_SDK_ROOT=" + filepath.Join(dir, "gone")}, nil)
	if a := st.Mobile.Android; a == nil || !a.Missing || a.Var != "ANDROID_SDK_ROOT" {
		t.Errorf("android = %+v", a)
	}
}

// Homebrew's cask puts Flutter in share/ and links Caskroom to it: the
// copy is brew's all the same.
func TestFlutterCask(t *testing.T) {
	brew := root(t)
	sdk := filepath.Join(brew, "share", "flutter")
	fakeExe(t, filepath.Join(sdk, "bin", "flutter"))
	writeFile(t, filepath.Join(sdk, "bin", "cache", "flutter.version.json"), `{"frameworkVersion": "3.47.9", "channel": "stable"}`)
	cask := filepath.Join(brew, "Caskroom", "flutter", "3.47.9")
	if err := os.MkdirAll(cask, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sdk, filepath.Join(cask, "flutter")); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(brew, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(cask, "flutter", "bin", "flutter"), filepath.Join(bin, "flutter")); err != nil {
		t.Fatal(err)
	}
	st, err := Mobile(t.Context(), &run.Fake{}, sysenv.Env{"HOME=" + brew, "PATH=" + bin, "HOMEBREW_PREFIX=" + brew}, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := st.Mobile.Flutter
	if f == nil || f.Source.Manager != "brew" || f.Source.Root != cask || f.Root != sdk || f.Version != "3.47.9" {
		t.Errorf("flutter = %+v", f)
	}
}
