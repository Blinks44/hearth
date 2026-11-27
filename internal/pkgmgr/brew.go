package pkgmgr

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

type brewInfo struct {
	Formulae []struct {
		Name      string
		FullName  string `json:"full_name"`
		Tap       string `json:"tap"`
		Desc      string `json:"desc"`
		Homepage  string `json:"homepage"`
		License   string `json:"license"`
		KegOnly   bool   `json:"keg_only"`
		LinkedKeg string `json:"linked_keg"`
		Installed []struct {
			Version            string
			InstalledOnRequest bool `json:"installed_on_request"`
			// RuntimeDependencies are what the keg was built with, from its
			// install receipt, as brew uses --installed reads them: a tap's
			// formula may list no Dependencies, as the tap's optional ones.
			RuntimeDependencies []struct {
				FullName string `json:"full_name"`
			} `json:"runtime_dependencies"`
		}
		Dependencies []string
	}
	Casks []struct {
		Token              string
		Tap                string `json:"tap"`
		Desc               string `json:"desc"`
		Homepage           string `json:"homepage"`
		Installed          json.RawMessage
		BundleShortVersion string `json:"bundle_short_version"`
		// Artifacts are of many shapes; an app's names where it went.
		Artifacts []json.RawMessage
	}
}

func brew(m *inventory.PackageManager, probe func(...string) ([]byte, error)) {
	if data, err := probe("--prefix"); err == nil {
		m.Prefix = strings.TrimSpace(string(data))
	}
	var linked map[string]string
	var selfUpdated map[string]bool
	data, err := probe("info", "--json=v2", "--installed")
	if err == nil {
		m.Packages, linked, selfUpdated, err = parseBrew(data, m.Prefix)
		m.ReasonsKnown = err == nil
	}
	if err != nil {
		m.Warnings = append(m.Warnings, err.Error())
	}
	data, err = probe("outdated", "--json=v2")
	if err == nil {
		err = brewUpdates(m, data, linked, selfUpdated)
	}
	if err != nil {
		m.Warnings = append(m.Warnings, err.Error())
	} else {
		m.UpdatesKnown = true
	}
	if data, err := probe("tap"); err == nil {
		m.Taps = strings.Fields(string(data))
	} else {
		m.Warnings = append(m.Warnings, err.Error())
	}
}

func parseBrew(data []byte, prefix string) ([]inventory.SystemPackage, map[string]string, map[string]bool, error) {
	var info brewInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, nil, nil, fmt.Errorf("brew installed: %w", err)
	}
	if info.Formulae == nil && info.Casks == nil {
		return nil, nil, nil, fmt.Errorf("brew installed: missing package lists")
	}
	var out []inventory.SystemPackage
	linked := map[string]string{}
	selfUpdated := map[string]bool{}
	for _, f := range info.Formulae {
		// A tap's formula keeps its full name, as brew outdated reports it.
		name := f.FullName
		if name == "" {
			name = f.Name
		}
		if f.LinkedKeg != "" {
			linked[name] = f.LinkedKeg
		}
		for _, v := range f.Installed {
			path := ""
			if prefix != "" {
				path = filepath.Join(prefix, "Cellar", f.Name, v.Version)
			}
			deps := slices.Clone(f.Dependencies)
			for _, d := range v.RuntimeDependencies {
				if !slices.Contains(deps, d.FullName) {
					deps = append(deps, d.FullName)
				}
			}
			out = append(out, inventory.SystemPackage{
				Name:         name,
				Version:      v.Version,
				Kind:         "formula",
				Path:         path,
				Dependencies: deps,
				Dependency:   !v.InstalledOnRequest,
				Description:  f.Desc,
				Homepage:     f.Homepage,
				License:      f.License,
				Tap:          f.Tap,
				KegOnly:      f.KegOnly,
			})
		}
	}
	for _, c := range info.Casks {
		var version string
		if err := json.Unmarshal(c.Installed, &version); err != nil {
			var versions []string
			if err := json.Unmarshal(c.Installed, &versions); err != nil {
				return out, linked, selfUpdated, fmt.Errorf("brew cask %s installed: %w", c.Token, err)
			}
			version = strings.Join(versions, ", ")
		}
		if bv := strings.TrimSpace(c.BundleShortVersion); bv != "" {
			if cmp, ok := compareDotVersion(bv, version); ok && cmp > 0 {
				version = bv
				selfUpdated[c.Token] = true
			}
		}
		path := ""
		if prefix != "" {
			path = filepath.Join(prefix, "Caskroom", c.Token)
		}
		out = append(out, inventory.SystemPackage{
			Name:        c.Token,
			Version:     version,
			Kind:        "cask",
			Path:        path,
			Artifacts:   caskApps(c.Artifacts),
			Description: c.Desc,
			Homepage:    c.Homepage,
			Tap:         c.Tap,
		})
	}
	return out, linked, selfUpdated, nil
}

// caskApps are where a cask's apps went, as /Applications/OrbStack.app;
// its other artifacts lead into them or into the Caskroom. Homebrew's
// paths are POSIX ones on every system it runs on.
func caskApps(artifacts []json.RawMessage) []string {
	var out []string
	for _, raw := range artifacts {
		var a struct {
			App    []json.RawMessage
			Target string
		}
		if json.Unmarshal(raw, &a) == nil && len(a.App) > 0 && path.IsAbs(a.Target) {
			out = append(out, a.Target)
		}
	}
	return out
}

func brewUpdates(m *inventory.PackageManager, data []byte, linked map[string]string, selfUpdated map[string]bool) error {
	// Both lists name a package by "name": a formula's full name and a
	// cask's token.
	type outdated struct {
		Name    string
		Current string `json:"current_version"`
	}
	var info struct {
		Formulae, Casks []outdated
	}
	if err := json.Unmarshal(data, &info); err != nil {
		return fmt.Errorf("brew outdated: %w", err)
	}
	if info.Formulae == nil && info.Casks == nil {
		return fmt.Errorf("brew outdated: missing package lists")
	}
	updates := map[[2]string]string{}
	for _, f := range info.Formulae {
		updates[[2]string{"formula", f.Name}] = f.Current
	}
	for _, c := range info.Casks {
		updates[[2]string{"cask", c.Name}] = c.Current
	}
	// A formula with several uncleaned kegs in the Cellar has one upgrade:
	// attach it to the linked keg, or to the last installed keg when
	// unlinked (as for keg-only formulae).
	target := map[[2]string]int{}
	matchedLinked := map[[2]string]bool{}
	for i, p := range m.Packages {
		m.Packages[i].Available = ""
		key := [2]string{p.Kind, p.Name}
		avail := updates[key]
		if avail == "" || matchedLinked[key] {
			continue
		}
		if p.Kind == "cask" {
			if cmp, ok := compareDotVersion(p.Version, avail); ok && (cmp > 0 || (cmp == 0 && selfUpdated[p.Name])) {
				continue
			}
		}
		target[key] = i
		if p.Kind == "formula" && linked[p.Name] != "" && p.Version == linked[p.Name] {
			matchedLinked[key] = true
		}
	}
	for key, idx := range target {
		m.Packages[idx].Available = updates[key]
	}
	return nil
}

// compareDotVersion compares two dotted numeric versions (ignoring Homebrew's
// ",build" suffix and an optional leading "v"). Returns ok=false if either
// string contains non-numeric parts so unusual version schemes fall back to
// Homebrew's own outdated comparison.
func compareDotVersion(a, b string) (int, bool) {
	pa, ok := parseDotVersion(a)
	if !ok {
		return 0, false
	}
	pb, ok := parseDotVersion(b)
	if !ok {
		return 0, false
	}
	n := max(len(pa), len(pb))
	for i := range n {
		var va, vb int
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1, true
		}
		if va > vb {
			return 1, true
		}
	}
	return 0, true
}

func parseDotVersion(s string) ([]int, bool) {
	s = strings.TrimSpace(s)
	s, _, _ = strings.Cut(s, ",")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if s == "" {
		return nil, false
	}
	parts := strings.Split(s, ".")
	out := make([]int, len(parts))
	for i, part := range parts {
		if part == "" {
			return nil, false
		}
		n := 0
		for j := range len(part) {
			ch := part[j]
			if ch < '0' || ch > '9' {
				return nil, false
			}
			n = n*10 + int(ch-'0')
			if n < 0 {
				return nil, false
			}
		}
		out[i] = n
	}
	return out, true
}
