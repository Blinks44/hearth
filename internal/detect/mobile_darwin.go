package detect

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// applications is where Xcode is looked for; simulators where Xcode keeps
// the simulators' runtimes: mounted disk images under Volumes since Xcode
// 15, bundles in Profiles before.
var (
	applications = "/Applications"
	simulators   = "/Library/Developer/CoreSimulator"
)

// mobileDeveloperDir returns the active developer directory xcode-select uses,
// or "" when neither Xcode nor Command Line Tools is configured.
func mobileDeveloperDir() string {
	dir, _ := developerDir()
	return dir
}

// xcodes are the Xcode apps in /Applications, by their version.plist.
func xcodes() []inventory.XcodeApp {
	apps, _ := filepath.Glob(filepath.Join(applications, "Xcode*.app"))
	selected := mobileDeveloperDir()
	var out []inventory.XcodeApp
	for _, app := range apps {
		plist := plistStrings(filepath.Join(app, "Contents", "version.plist"))
		if plist["CFBundleShortVersionString"] == "" {
			continue
		}
		out = append(out, inventory.XcodeApp{Path: app, Version: plist["CFBundleShortVersionString"], Build: plist["ProductBuildVersion"],
			Selected: within(selected, app), AppStore: exists(filepath.Join(app, "Contents", "_MASReceipt"))})
	}
	return out
}

// simRuntimes are the simulators' runtimes, by their bundles' names:
// "iOS 18.2.simruntime".
func simRuntimes() []inventory.SimRuntime {
	mounted, _ := filepath.Glob(filepath.Join(simulators, "Volumes", "*", "Library", "Developer", "CoreSimulator", "Profiles", "Runtimes", "*.simruntime"))
	bundled, _ := filepath.Glob(filepath.Join(simulators, "Profiles", "Runtimes", "*.simruntime"))
	ids := simRuntimeIDs()
	var out []inventory.SimRuntime
	for _, p := range append(mounted, bundled...) {
		name := strings.TrimSuffix(filepath.Base(p), ".simruntime")
		platform, version, ok := strings.CutLast(name, " ")
		if !ok {
			continue
		}
		rt := inventory.SimRuntime{Platform: platform, Version: version, Path: p}
		if build := simVolumeBuild(p); build != "" {
			rt.ID = ids[build]
		}
		if rt.ID == "" {
			rt.ID = ids[platform+"\x00"+version]
		}
		out = append(out, rt)
	}
	return out
}

// simVolumeBuild extracts the build tag from a mounted runtime volume path
// such as ".../Volumes/iOS_22C150/...".
func simVolumeBuild(path string) string {
	rel, err := filepath.Rel(filepath.Join(simulators, "Volumes"), path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	vol, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	_, build, ok := strings.CutLast(vol, "_")
	if !ok {
		return ""
	}
	return build
}

// simRuntimeIDs reads CoreSimulator's XML images.plist when present and
// maps build tags (and platform+version) to runtime image UUIDs.
func simRuntimeIDs() map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(simulators, "Images", "images.plist"))
	if err != nil {
		return out
	}
	var root struct {
		Dict struct {
			Items []struct {
				XMLName xml.Name
				Value   string `xml:",chardata"`
				Dicts   []struct {
					Items []struct {
						XMLName xml.Name
						Value   string `xml:",chardata"`
						Inner   []struct {
							XMLName xml.Name
							Value   string `xml:",chardata"`
						} `xml:",any"`
					} `xml:",any"`
				} `xml:"dict"`
			} `xml:",any"`
		} `xml:"dict"`
	}
	if xml.Unmarshal(data, &root) != nil {
		return out
	}
	key := ""
	for _, top := range root.Dict.Items {
		switch top.XMLName.Local {
		case "key":
			key = top.Value
		case "array":
			if key != "images" {
				continue
			}
			for _, img := range top.Dicts {
				var uuid, build, version, runtimeID string
				k := ""
				for _, it := range img.Items {
					switch it.XMLName.Local {
					case "key":
						k = it.Value
					case "string":
						if k == "uuid" {
							uuid = strings.TrimSpace(it.Value)
						}
					case "dict":
						if k != "runtimeInfo" {
							continue
						}
						rk := ""
						for _, sub := range it.Inner {
							switch sub.XMLName.Local {
							case "key":
								rk = sub.Value
							case "string":
								switch rk {
								case "build":
									build = strings.TrimSpace(sub.Value)
								case "version":
									version = strings.TrimSpace(sub.Value)
								case "identifier":
									runtimeID = strings.TrimSpace(sub.Value)
								}
							}
						}
					}
				}
				if uuid == "" {
					continue
				}
				if build != "" {
					out[build] = uuid
				}
				if version != "" {
					for _, plat := range []string{"iOS", "watchOS", "tvOS", "xrOS", "visionOS"} {
						if strings.Contains(runtimeID, "."+plat+"-") {
							out[plat+"\x00"+version] = uuid
							if plat == "xrOS" {
								out["visionOS\x00"+version] = uuid
							}
						}
					}
				}
			}
		}
	}
	return out
}

// plistStrings reads the string values of an XML property list's top
// dictionary, as Xcode's version.plist.
func plistStrings(path string) map[string]string {
	values := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return values
	}
	var plist struct {
		Dict struct {
			Items []struct {
				XMLName xml.Name
				Value   string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"dict"`
	}
	if xml.Unmarshal(data, &plist) != nil {
		return values
	}
	key := ""
	for _, it := range plist.Dict.Items {
		switch it.XMLName.Local {
		case "key":
			key = it.Value
		case "string":
			values[key] = it.Value
		}
	}
	return values
}

// androidDefault is where Android Studio puts the SDK.
func androidDefault(_ sysenv.Env, home string) string {
	return filepath.Join(home, "Library", "Android", "sdk")
}

func androidStudios(_ sysenv.Env, home string) []string {
	return []string{filepath.Join(applications, "Android Studio.app"), filepath.Join(home, "Applications", "Android Studio.app")}
}
