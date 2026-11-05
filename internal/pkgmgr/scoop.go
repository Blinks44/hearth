package pkgmgr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// scoopManagers reads manifests only. Global apps use the user's buckets too;
// install.json records the bucket the package actually came from.
func scoopManagers(env sysenv.Env) []inventory.PackageManager {
	user := env.Get("SCOOP")
	if user == "" && env.Get("USERPROFILE") != "" {
		user = filepath.Join(env.Get("USERPROFILE"), "scoop")
	}
	global := env.Get("SCOOP_GLOBAL")
	if global == "" && env.Get("ProgramData") != "" {
		global = filepath.Join(env.Get("ProgramData"), "scoop")
	}
	var out []inventory.PackageManager
	seen := map[string]bool{}
	for _, root := range []string{user, global} {
		if !filepath.IsAbs(root) {
			continue
		}
		real, err := filepath.EvalSymlinks(root)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		dirs, err := os.ReadDir(filepath.Join(root, "apps"))
		if err != nil {
			continue
		}
		m := inventory.PackageManager{ID: "scoop", Name: "Scoop", Prefix: root, Path: filepath.Join(root, "shims", "scoop.ps1"), UpdatesKnown: true}
		for _, dir := range dirs {
			if !dir.IsDir() || dir.Name() == "scoop" {
				continue
			}
			current := filepath.Join(root, "apps", dir.Name(), "current")
			version, err := scoopManifest(current)
			if err != nil {
				m.Warnings = append(m.Warnings, dir.Name()+": "+err.Error())
				m.UpdatesKnown = false
				continue
			}
			p := inventory.SystemPackage{Name: dir.Name(), Version: version, Path: current, Kind: "app"}
			available, err := scoopUpdate(user, current, p)
			if err != nil {
				m.UpdatesKnown = false
				m.Warnings = append(m.Warnings, p.Name+": "+err.Error())
			} else {
				p.Available = available
			}
			m.Packages = append(m.Packages, p)
		}
		slices.SortFunc(m.Packages, func(a, b inventory.SystemPackage) int { return strings.Compare(a.Name, b.Name) })
		out = append(out, m)
	}
	return out
}

func scoopManifest(dir string) (string, error) {
	var manifest struct{ Version string }
	if err := scoopJSON(dir, "manifest.json", &manifest); err != nil {
		return "", fmt.Errorf("read manifest: %w", err)
	}
	if manifest.Version == "" {
		return "", fmt.Errorf("manifest has no version")
	}
	return manifest.Version, nil
}

// Scoop stores its own metadata under scoop-* names when an app ships files
// with the same names. Prefer that metadata, just as Scoop does.
func scoopJSON(dir, name string, dst any) error {
	data, err := os.ReadFile(filepath.Join(dir, "scoop-"+name))
	if os.IsNotExist(err) {
		data, err = os.ReadFile(filepath.Join(dir, name))
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func scoopUpdate(user, current string, p inventory.SystemPackage) (string, error) {
	var install struct {
		Bucket string
		Hold   bool
	}
	if err := scoopJSON(current, "install.json", &install); err != nil {
		return "", fmt.Errorf("read installation source: %w", err)
	}
	if install.Hold {
		return "", nil
	}
	if install.Bucket == "" || strings.ContainsAny(install.Bucket, `/\:`) || install.Bucket == "." || install.Bucket == ".." {
		return "", fmt.Errorf("local bucket unknown")
	}
	latest, err := scoopManifestFile(filepath.Join(user, "buckets", install.Bucket, "bucket", p.Name+".json"))
	if os.IsNotExist(err) {
		latest, err = scoopManifestFile(filepath.Join(user, "buckets", install.Bucket, p.Name+".json"))
	}
	if err != nil {
		return "", fmt.Errorf("read local bucket: %w", err)
	}
	if strings.HasPrefix(strings.ToLower(p.Version), "nightly") || strings.EqualFold(latest, "nightly") {
		return "", fmt.Errorf("nightly updates need a fresh manifest")
	}
	if latest == p.Version {
		return "", nil
	}
	cmp, ok := compareDottedVersions(latest, p.Version)
	if !ok {
		return "", fmt.Errorf("cannot compare local versions %q and %q", p.Version, latest)
	}
	if cmp > 0 {
		return latest, nil
	}
	return "", nil
}

func scoopManifestFile(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	var m struct{ Version string }
	if err = json.Unmarshal(data, &m); err != nil {
		return "", err
	}
	if m.Version == "" {
		return "", fmt.Errorf("manifest has no version")
	}
	return m.Version, nil
}

// compareDottedVersions compares numeric versions of any length without integer
// overflow. Other Scoop version schemes remain unknown rather than suggesting
// a downgrade; equal opaque versions are handled before this call.
func compareDottedVersions(a, b string) (int, bool) {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for _, parts := range [][]string{left, right} {
		for _, p := range parts {
			if p == "" || strings.ContainsFunc(p, func(r rune) bool { return r < '0' || r > '9' }) {
				return 0, false
			}
		}
	}
	for i := 0; i < max(len(left), len(right)); i++ {
		x, y := "", ""
		if i < len(left) {
			x = strings.TrimLeft(left[i], "0")
		}
		if i < len(right) {
			y = strings.TrimLeft(right[i], "0")
		}
		if len(x) != len(y) {
			if len(x) > len(y) {
				return 1, true
			}
			return -1, true
		}
		if c := strings.Compare(x, y); c != 0 {
			return c, true
		}
	}
	return 0, true
}
