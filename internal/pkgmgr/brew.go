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
		Token     string
		Installed json.RawMessage
		// Artifacts are of many shapes; an app's names where it went.
		Artifacts []json.RawMessage
	}
}

func brew(m *inventory.PackageManager, probe func(...string) ([]byte, error)) {
	if data, err := probe("--prefix"); err == nil {
		m.Prefix = strings.TrimSpace(string(data))
	}
	var linked map[string]string
	data, err := probe("info", "--json=v2", "--installed")
	if err == nil {
		m.Packages, linked, err = parseBrew(data, m.Prefix)
		m.ReasonsKnown = err == nil
	}
	if err != nil {
		m.Warnings = append(m.Warnings, err.Error())
	}
	data, err = probe("outdated", "--json=v2")
	if err == nil {
		err = brewUpdates(m, data, linked)
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

func parseBrew(data []byte, prefix string) ([]inventory.SystemPackage, map[string]string, error) {
	var info brewInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, nil, fmt.Errorf("brew installed: %w", err)
	}
	if info.Formulae == nil && info.Casks == nil {
		return nil, nil, fmt.Errorf("brew installed: missing package lists")
	}
	var out []inventory.SystemPackage
	linked := map[string]string{}
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
			out = append(out, inventory.SystemPackage{Name: name, Version: v.Version, Kind: "formula", Path: path, Dependencies: deps,
				Dependency: !v.InstalledOnRequest})
		}
	}
	for _, c := range info.Casks {
		var version string
		if err := json.Unmarshal(c.Installed, &version); err != nil {
			var versions []string
			if err := json.Unmarshal(c.Installed, &versions); err != nil {
				return out, linked, fmt.Errorf("brew cask %s installed: %w", c.Token, err)
			}
			version = strings.Join(versions, ", ")
		}
		path := ""
		if prefix != "" {
			path = filepath.Join(prefix, "Caskroom", c.Token)
		}
		out = append(out, inventory.SystemPackage{Name: c.Token, Version: version, Kind: "cask", Path: path, Artifacts: caskApps(c.Artifacts)})
	}
	return out, linked, nil
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

func brewUpdates(m *inventory.PackageManager, data []byte, linked map[string]string) error {
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
		if updates[key] == "" || matchedLinked[key] {
			continue
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
