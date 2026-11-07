package detect

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Mobile finds what iOS and Android development needs: Xcode and its
// simulators, the Android SDK with its platforms, emulator images and the
// AVDs made of them, CocoaPods and Flutter. Files are read; neither Xcode's
// tools nor flutter are run: simctl may install Xcode's components, and
// flutter may download its own.
func Mobile(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	m := &inventory.MobileState{Xcodes: xcodes(), Runtimes: simRuntimes()}
	home := sources.ManagerDirs(term).Home
	m.Android = androidSDK(term, home)
	pods := newFinder("pod", term)
	pods.lookPath("pod", term, gui)
	if i := inventory.TerminalCopy(pods.ins); i >= 0 {
		in := pods.ins[i]
		in.Version = gitVersion.FindString(versionOf(ctx, r, term, in.Path)) // 1.16.2
		m.CocoaPods = &in
	}
	flutter := newFinder("flutter", term)
	flutter.lookPath("flutter", term, gui)
	if i := inventory.TerminalCopy(flutter.ins); i >= 0 {
		m.Flutter = flutterSDK(flutter.ins[i])
	}
	return inventory.ToolState{Tool: "mobile", Mobile: m}, ctx.Err()
}

// flutterSDK is the SDK of the flutter command in, <root>/bin/flutter,
// with the version its tool last wrote down.
func flutterSDK(in inventory.Installation) *inventory.Flutter {
	f := &inventory.Flutter{Installation: in, Root: filepath.Dir(filepath.Dir(in.RealPath))}
	var v struct {
		Version string `json:"frameworkVersion"`
		Channel string `json:"channel"`
	}
	if data, err := os.ReadFile(filepath.Join(f.Root, "bin", "cache", "flutter.version.json")); err == nil && json.Unmarshal(data, &v) == nil {
		f.Version, f.Channel = v.Version, v.Channel
	} else if data, err := os.ReadFile(filepath.Join(f.Root, "version")); err == nil {
		f.Version = strings.TrimSpace(string(data)) // before 3.24
	}
	return f
}

// androidSDK is the SDK ANDROID_HOME, else ANDROID_SDK_ROOT, else the
// default folder names, with its platforms, system images and the AVDs.
func androidSDK(env sysenv.Env, home string) *inventory.AndroidSDK {
	sdk := &inventory.AndroidSDK{}
	for _, name := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if dir := env.Get(name); dir != "" {
			sdk.Root, sdk.Var = dir, name
			break
		}
	}
	if sdk.Root == "" && home != "" {
		sdk.Root = androidDefault(env, home)
	}
	for _, app := range androidStudios(env, home) {
		if exists(app) {
			sdk.Studio = app
			break
		}
	}
	if sdk.Root == "" || !isDir(sdk.Root) {
		if sdk.Studio == "" && sdk.Var == "" {
			return nil
		}
		sdk.Missing = true
		return sdk
	}
	platforms, _ := filepath.Glob(filepath.Join(sdk.Root, "platforms", "android-*"))
	for _, p := range platforms {
		sdk.Platforms = append(sdk.Platforms, inventory.AndroidPart{Name: filepath.Base(p), API: androidAPI(p), Path: p})
	}
	images, _ := filepath.Glob(filepath.Join(sdk.Root, "system-images", "*", "*", "*"))
	avds := androidAVDs(env, home)
	for _, p := range images {
		if !isDir(p) {
			continue
		}
		rel, _ := filepath.Rel(filepath.Join(sdk.Root, "system-images"), p)
		name := filepath.ToSlash(rel)
		sdk.Images = append(sdk.Images, inventory.AndroidPart{Name: name, API: androidAPI(p), Path: p, AVDs: avds[name]})
	}
	byAPI := func(a, b inventory.AndroidPart) int { return b.API - a.API }
	slices.SortStableFunc(sdk.Platforms, byAPI)
	slices.SortStableFunc(sdk.Images, byAPI)
	return sdk
}

// androidAPI is the API level of a platform or image folder, from its
// source.properties, else from its android-NN name.
func androidAPI(dir string) int {
	if api, err := strconv.Atoi(properties(filepath.Join(dir, "source.properties"))["AndroidVersion.ApiLevel"]); err == nil {
		return api
	}
	for part := range strings.SplitSeq(filepath.ToSlash(dir), "/") {
		if api, err := strconv.Atoi(strings.TrimPrefix(part, "android-")); err == nil && strings.HasPrefix(part, "android-") {
			return api
		}
	}
	return 0
}

// androidAVDs are the AVDs by the system image each is made with, as
// android-34/google_apis/arm64-v8a, from their config.ini.
func androidAVDs(env sysenv.Env, home string) map[string][]string {
	dir := env.Get("ANDROID_AVD_HOME")
	if dir == "" {
		user := env.Get("ANDROID_USER_HOME")
		if user == "" && home != "" {
			user = filepath.Join(home, ".android")
		}
		dir = filepath.Join(user, "avd")
	}
	inis, _ := filepath.Glob(filepath.Join(dir, "*.ini"))
	avds := map[string][]string{}
	for _, ini := range inis {
		name := strings.TrimSuffix(filepath.Base(ini), ".ini")
		path := properties(ini)["path"]
		if path == "" {
			path = filepath.Join(dir, name+".avd")
		}
		// image.sysdir.1=system-images/android-34/google_apis/arm64-v8a/
		image := properties(filepath.Join(path, "config.ini"))["image.sysdir.1"]
		image = strings.Trim(strings.TrimPrefix(filepath.ToSlash(image), "system-images/"), "/")
		if image != "" {
			avds[image] = append(avds[image], name)
		}
	}
	return avds
}

// properties reads a file of key=value lines, as the Android SDK's.
func properties(path string) map[string]string {
	values := map[string]string{}
	file, err := os.Open(path)
	if err != nil {
		return values
	}
	defer file.Close() //nolint:errcheck // read only
	s := bufio.NewScanner(file)
	for s.Scan() {
		if key, value, ok := strings.Cut(s.Text(), "="); ok && !strings.HasPrefix(strings.TrimSpace(key), "#") {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return values
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
