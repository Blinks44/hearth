package ops

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// RemoveAndroidImagePlan returns the plan that removes an unused Android
// emulator system image through sdkmanager when available, or as its folder.
// It refuses when any AVD still references the image.
func RemoveAndroidImagePlan(sdkRoot string, img inventory.AndroidPart, env sysenv.Env, detect Detect) (Plan, error) {
	if len(img.AVDs) > 0 {
		return Plan{}, errors.New(locales.MobileRemoveImageInUse())
	}
	if img.Path == "" || !filepath.IsAbs(img.Path) {
		return Plan{}, ErrNoFix
	}
	p := Plan{
		Title:   locales.MobileRemoveImageTitle(img.API),
		Why:     locales.MobileRemoveImageWhy(),
		Changes: []string{"− " + cmp.Or(img.Name, img.Path)},
	}
	if sdkmgr := findSDKManager(sdkRoot, env); sdkmgr != "" && img.Name != "" {
		pkg := "system-images;" + strings.ReplaceAll(filepath.ToSlash(img.Name), "/", ";")
		p.Steps = []Step{sdkmanagerUninstallStep(sdkmgr, sdkRoot, pkg)}
	} else {
		p.Steps = []Step{{Remove: img.Path, Timeout: cleanupTimeout}}
		p.Notes = []Note{{Title: locales.CleanupRemoveNote(), Text: locales.CleanupRemoveNoteText()}}
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if st.Mobile != nil && st.Mobile.Android != nil {
				for _, cur := range st.Mobile.Android.Images {
					if cur.Path == img.Path {
						return "", errors.New(locales.MobileRemoveImageCheckFail(img.API))
					}
				}
			}
			return locales.MobileRemoveImageCheckOk(img.API), nil
		}
	}
	return p, nil
}

// findSDKManager finds Android's sdkmanager inside sdkRoot or on PATH.
func findSDKManager(sdkRoot string, env sysenv.Env) string {
	if sdkRoot != "" {
		candidates := []string{filepath.Join(sdkRoot, "cmdline-tools", "latest", "bin", sdkmanagerExe)}
		globbed, _ := filepath.Glob(filepath.Join(sdkRoot, "cmdline-tools", "*", "bin", sdkmanagerExe))
		candidates = append(candidates, globbed...)
		for _, p := range candidates {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p
			}
		}
	}
	if paths := sysenv.LookPathAll("sdkmanager", env); len(paths) > 0 {
		return paths[0]
	}
	return ""
}
