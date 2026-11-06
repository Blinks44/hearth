package pkgmgr

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

type brewInfo struct {
	Formulae []struct {
		Name      string
		FullName  string `json:"full_name"`
		Installed []struct {
			Version            string
			InstalledOnRequest bool `json:"installed_on_request"`
		}
		Dependencies []string
	}
	Casks []struct {
		Token     string
		Installed json.RawMessage
	}
}

func brew(m *inventory.PackageManager, probe func(...string) ([]byte, error)) {
	if data, err := probe("--prefix"); err == nil {
		m.Prefix = strings.TrimSpace(string(data))
	}
	data, err := probe("info", "--json=v2", "--installed")
	if err == nil {
		m.Packages, err = parseBrew(data, m.Prefix)
		m.ReasonsKnown = err == nil
	}
	if err != nil {
		m.Warnings = append(m.Warnings, err.Error())
	}
	data, err = probe("outdated", "--json=v2")
	if err == nil {
		err = brewUpdates(m, data)
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

func parseBrew(data []byte, prefix string) ([]inventory.SystemPackage, error) {
	var info brewInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("brew installed: %w", err)
	}
	if info.Formulae == nil && info.Casks == nil {
		return nil, fmt.Errorf("brew installed: missing package lists")
	}
	var out []inventory.SystemPackage
	for _, f := range info.Formulae {
		for _, v := range f.Installed {
			path := ""
			if prefix != "" {
				path = filepath.Join(prefix, "Cellar", f.Name, v.Version)
			}
			// A tap's formula keeps its full name, as brew outdated reports it.
			name := f.FullName
			if name == "" {
				name = f.Name
			}
			out = append(out, inventory.SystemPackage{Name: name, Version: v.Version, Kind: "formula", Path: path, Dependencies: f.Dependencies,
				Dependency: !v.InstalledOnRequest})
		}
	}
	for _, c := range info.Casks {
		var version string
		if err := json.Unmarshal(c.Installed, &version); err != nil {
			var versions []string
			if err := json.Unmarshal(c.Installed, &versions); err != nil {
				return out, fmt.Errorf("brew cask %s installed: %w", c.Token, err)
			}
			version = strings.Join(versions, ", ")
		}
		path := ""
		if prefix != "" {
			path = filepath.Join(prefix, "Caskroom", c.Token)
		}
		out = append(out, inventory.SystemPackage{Name: c.Token, Version: version, Kind: "cask", Path: path})
	}
	return out, nil
}

func brewUpdates(m *inventory.PackageManager, data []byte) error {
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
	for i, p := range m.Packages {
		m.Packages[i].Available = updates[[2]string{p.Kind, p.Name}]
	}
	return nil
}
