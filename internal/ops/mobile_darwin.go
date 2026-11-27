package ops

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// RemoveSimRuntimePlan returns the plan that removes an older iOS/watchOS/tvOS
// simulator runtime via xcrun simctl runtime delete, or as a legacy bundle when
// it is not mounted under Volumes.
func RemoveSimRuntimePlan(ctx context.Context, r run.Runner, rt inventory.SimRuntime, env sysenv.Env, detect Detect) (Plan, error) {
	if rt.Platform == "" || rt.Version == "" {
		return Plan{}, ErrNoFix
	}
	id := rt.ID
	if id == "" && r != nil {
		id = findSimRuntimeID(ctx, r, rt, env)
	}
	p := Plan{
		Title:   locales.MobileRemoveSimTitle(rt.Platform, rt.Version),
		Why:     locales.MobileRemoveSimWhy(),
		Changes: []string{"− " + rt.Platform + " " + rt.Version},
	}
	cleanPath := filepath.Clean(rt.Path)
	switch {
	case id != "":
		p.Steps = []Step{{Argv: []string{"xcrun", "simctl", "runtime", "delete", id}, Timeout: cleanupTimeout}}
	case rt.Path != "" && filepath.IsAbs(cleanPath) &&
		filepath.Dir(filepath.ToSlash(cleanPath)) == "/Library/Developer/CoreSimulator/Profiles/Runtimes" &&
		strings.HasSuffix(cleanPath, ".simruntime"):
		p.Steps = []Step{{Argv: []string{"/bin/rm", "-rf", "--", cleanPath}, Admin: true, Timeout: cleanupTimeout}}
		p.Notes = []Note{
			AdminNote(),
			{Title: locales.CleanupRemoveNote(), Text: locales.CleanupRemoveNoteText()},
		}
	default:
		return Plan{}, errors.New(locales.MobileRemoveSimNoId())
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if st.Mobile != nil {
				for _, cur := range st.Mobile.Runtimes {
					if (rt.Path != "" && cur.Path == rt.Path) || (id != "" && cur.ID == id) {
						return "", errors.New(locales.MobileRemoveSimCheckFail(rt.Platform, rt.Version))
					}
				}
			}
			return locales.MobileRemoveSimCheckOk(rt.Platform, rt.Version), nil
		}
	}
	return p, nil
}

// findSimRuntimeID asks xcrun simctl runtime list -j for the runtime UUID
// matching rt when images.plist did not provide one.
func findSimRuntimeID(ctx context.Context, r run.Runner, rt inventory.SimRuntime, env sysenv.Env) string {
	res, err := r.Run(ctx, run.Cmd{Args: []string{"xcrun", "simctl", "runtime", "list", "-j"}, Env: env})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	var list map[string]struct {
		Identifier        string `json:"identifier"`
		MountPath         string `json:"mountPath"`
		RuntimeBundlePath string `json:"runtimeBundlePath"`
		RuntimeIdentifier string `json:"runtimeIdentifier"`
		Version           string `json:"version"`
	}
	if json.Unmarshal(res.Stdout, &list) != nil {
		return ""
	}
	for key, item := range list {
		id := item.Identifier
		if id == "" {
			id = key
		}
		if rt.Path != "" && (item.RuntimeBundlePath == rt.Path || item.MountPath != "" && strings.HasPrefix(rt.Path, item.MountPath+string(filepath.Separator))) {
			return id
		}
		if item.Version == rt.Version {
			if strings.Contains(item.RuntimeIdentifier, "."+rt.Platform+"-") ||
				(rt.Platform == "visionOS" && strings.Contains(item.RuntimeIdentifier, ".xrOS-")) {
				return id
			}
		}
	}
	return ""
}

// SwitchXcodePlan returns a plan that switches the active Xcode developer
// directory via xcode-select -s.
func SwitchXcodePlan(app inventory.XcodeApp, detect Detect) (Plan, bool) {
	if app.Path == "" || strings.Contains(app.Path, "..") || hasControlChars(app.Path) {
		return Plan{}, false
	}
	clean := filepath.Clean(app.Path)
	base := filepath.Base(clean)
	if filepath.Dir(clean) != "/Applications" || !strings.HasSuffix(base, ".app") || base == ".app" {
		return Plan{}, false
	}
	devDir := filepath.Join(clean, "Contents", "Developer")
	ver := app.Version
	if ver == "" {
		ver = filepath.Base(clean)
	}
	p := Plan{
		Title:   locales.MobileXcodeSwitchTitle(ver),
		Changes: []string{locales.MobileXcodeSwitchChange(devDir)},
		Steps:   []Step{{Argv: []string{"xcode-select", "-s", devDir}, Admin: true}},
		Notes:   []Note{AdminNote()},
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if st.Mobile != nil {
				if st.Mobile.DeveloperDir == devDir {
					return locales.MobileXcodeSwitchChange(devDir), nil
				}
				for _, x := range st.Mobile.Xcodes {
					if x.Selected && filepath.Clean(x.Path) == clean {
						return locales.MobileXcodeSwitchChange(devDir), nil
					}
				}
			}
			return "", errors.New("xcode-select did not switch to " + devDir)
		}
	}
	return p, true
}
