package detect

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

// Xcodes by their version.plist, the one xcode-select points into and
// the App Store's; the simulators' runtimes mounted and bundled.
func TestXcodes(t *testing.T) {
	dir := root(t)
	apps := filepath.Join(dir, "Applications")
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>BuildVersion</key><string>2</string><key>CFBundleShortVersionString</key><string>%s</string><key>ProductBuildVersion</key><string>%s</string></dict></plist>`
	writeFile(t, filepath.Join(apps, "Xcode.app", "Contents", "version.plist"), fmt.Sprintf(plist, "16.2", "16C5032a"))
	writeFile(t, filepath.Join(apps, "Xcode.app", "Contents", "_MASReceipt", "receipt"), "")
	writeFile(t, filepath.Join(apps, "Xcode-beta.app", "Contents", "version.plist"), fmt.Sprintf(plist, "26.0", "17A5241e"))
	if err := os.MkdirAll(filepath.Join(apps, "Xcode.app", "Contents", "Developer"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "xcode_select_link")
	if err := os.Symlink(filepath.Join(apps, "Xcode.app", "Contents", "Developer"), link); err != nil {
		t.Fatal(err)
	}
	sims := filepath.Join(dir, "CoreSimulator")
	mounted := filepath.Join(sims, "Volumes", "iOS_22C150", "Library", "Developer", "CoreSimulator", "Profiles", "Runtimes", "iOS 18.2.simruntime")
	bundled := filepath.Join(sims, "Profiles", "Runtimes", "watchOS 10.5.simruntime")
	for _, p := range []string{mounted, bundled} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	imagesPlist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>images</key><array><dict>
<key>uuid</key><string>7A1B2C3D-0000-1111-2222-333344445555</string>
<key>runtimeInfo</key><dict>
<key>build</key><string>22C150</string>
<key>identifier</key><string>com.apple.CoreSimulator.SimRuntime.iOS-18-2</string>
<key>version</key><string>18.2</string>
</dict></dict></array></dict></plist>`
	writeFile(t, filepath.Join(sims, "Images", "images.plist"), imagesPlist)
	oldApps, oldSims, oldLink := applications, simulators, xcodeSelectLink
	applications, simulators, xcodeSelectLink = apps, sims, link
	t.Cleanup(func() { applications, simulators, xcodeSelectLink = oldApps, oldSims, oldLink })

	want := []inventory.XcodeApp{
		{Path: filepath.Join(apps, "Xcode-beta.app"), Version: "26.0", Build: "17A5241e"},
		{Path: filepath.Join(apps, "Xcode.app"), Version: "16.2", Build: "16C5032a", Selected: true, AppStore: true},
	}
	if got := xcodes(); !reflect.DeepEqual(got, want) {
		t.Errorf("xcodes = %+v, want %+v", got, want)
	}
	wantRT := []inventory.SimRuntime{
		{Platform: "iOS", Version: "18.2", Path: mounted, ID: "7A1B2C3D-0000-1111-2222-333344445555"},
		{Platform: "watchOS", Version: "10.5", Path: bundled},
	}
	if got := simRuntimes(); !reflect.DeepEqual(got, wantRT) {
		t.Errorf("runtimes = %+v, want %+v", got, wantRT)
	}
	wantDev, _ := filepath.EvalSymlinks(filepath.Join(apps, "Xcode.app", "Contents", "Developer"))
	if got := mobileDeveloperDir(); got != wantDev {
		t.Errorf("mobileDeveloperDir = %q, want %q", got, wantDev)
	}
}
