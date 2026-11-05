package pkgmgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// nixProfiles reads modern user profiles without evaluating flakes, contacting
// the daemon or creating profile links. False leaves the classic nix-env probe
// in charge. An unreadable modern profile must not look like an empty success.
func nixProfiles(m *inventory.PackageManager, env sysenv.Env, probe func(...string) ([]byte, error)) bool {
	home := env.Get("HOME")
	if !filepath.IsAbs(home) {
		return false
	}
	state := env.Get("XDG_STATE_HOME")
	if !filepath.IsAbs(state) {
		state = filepath.Join(home, ".local", "state")
	}
	found := false
	seen := map[string]bool{}
	profiles := map[string]bool{}
	for _, profile := range []string{filepath.Join(home, ".nix-profile"), filepath.Join(state, "nix", "profile")} {
		real, err := sysenv.RealPath(profile)
		if err == nil && profiles[real] {
			continue
		}
		if err == nil {
			profiles[real] = true
		}
		file := filepath.Join(profile, "manifest.json")
		f, err := os.Open(file)
		if errors.Is(err, os.ErrNotExist) {
			if _, classicErr := os.Stat(filepath.Join(profile, "manifest.nix")); classicErr == nil {
				found = true
				data, queryErr := probe("--profile", profile, "--query", "--installed", "--json")
				var packages []inventory.SystemPackage
				if queryErr == nil {
					packages, queryErr = parse("nix", data)
				}
				if queryErr != nil {
					m.Warnings = append(m.Warnings, fmt.Sprintf("nix profile %s: %v", profile, queryErr))
				} else {
					m.Packages = append(m.Packages, packages...)
				}
			}
			continue
		}
		found = true
		if err != nil {
			m.Warnings = append(m.Warnings, fmt.Sprintf("nix profile %s: %v", profile, err))
			continue
		}
		const limit = 16 << 20
		data, err := io.ReadAll(io.LimitReader(f, limit+1))
		_ = f.Close()
		if err == nil && len(data) > limit {
			err = fmt.Errorf("manifest exceeds %d bytes", limit)
		}
		var packages []inventory.SystemPackage
		if err == nil {
			packages, err = parseNixProfile(data)
		}
		if err != nil {
			m.Warnings = append(m.Warnings, fmt.Sprintf("nix profile %s: %v", profile, err))
			continue
		}
		for _, p := range packages {
			if !seen[p.Path] {
				seen[p.Path] = true
				m.Packages = append(m.Packages, p)
			}
		}
	}
	return found
}

// Nix manifest versions 1/2 use an array, version 3 uses named elements.
// Names and versions come from store paths, as in Nix's DrvName parser. An
// element is one package: extra outputs (hello-2.12.1-man) only add a suffix
// to the main output's name, so the shortest store path name represents it.
func parseNixProfile(data []byte) ([]inventory.SystemPackage, error) {
	var manifest struct {
		Version  int
		Elements json.RawMessage
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	type element struct{ StorePaths []string }
	var entries []element
	switch manifest.Version {
	case 1, 2:
		if err := json.Unmarshal(manifest.Elements, &entries); err != nil {
			return nil, fmt.Errorf("decode elements: %w", err)
		}
	case 3:
		var named map[string]element
		if err := json.Unmarshal(manifest.Elements, &named); err != nil {
			return nil, fmt.Errorf("decode elements: %w", err)
		}
		for _, e := range named {
			entries = append(entries, e)
		}
	default:
		return nil, fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	if len(manifest.Elements) == 0 || string(manifest.Elements) == "null" {
		return nil, fmt.Errorf("missing profile elements")
	}
	var out []inventory.SystemPackage
	for _, e := range entries {
		if len(e.StorePaths) == 0 {
			return nil, fmt.Errorf("profile element has no store paths")
		}
		mainPath, name := "", ""
		for _, storePath := range e.StorePaths {
			hash, n, ok := strings.Cut(path.Base(storePath), "-")
			if !strings.HasPrefix(storePath, "/") || !ok || len(hash) != 32 || n == "" {
				return nil, fmt.Errorf("invalid store path %q", storePath)
			}
			if name == "" || len(n) < len(name) || len(n) == len(name) && n < name {
				mainPath, name = storePath, n
			}
		}
		version := ""
		for i := 0; i+1 < len(name); i++ {
			c := name[i+1]
			if name[i] == '-' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
				name, version = name[:i], name[i+1:]
				break
			}
		}
		out = append(out, inventory.SystemPackage{Name: name, Version: version, Path: mainPath, Kind: "profile"})
	}
	slices.SortFunc(out, func(a, b inventory.SystemPackage) int { return strings.Compare(a.Path, b.Path) })
	return slices.CompactFunc(out, func(a, b inventory.SystemPackage) bool { return a.Path == b.Path }), nil
}
