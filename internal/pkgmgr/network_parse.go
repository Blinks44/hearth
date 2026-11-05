package pkgmgr

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

func chocoUpdates(installed []inventory.SystemPackage, data []byte) ([]inventory.SystemPackage, error) {
	out := slices.Clone(installed)
	for i := range out {
		out[i].Available = ""
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "chocolatey v") || strings.HasPrefix(strings.ToLower(line), "output is package name") {
			continue
		}
		cols := strings.Split(line, "|")
		if len(cols) != 4 || cols[0] == "" || cols[2] == "" || cols[3] != "true" && cols[3] != "false" {
			return nil, fmt.Errorf("invalid Chocolatey updates row")
		}
		matched := false
		for i := range out {
			if strings.EqualFold(out[i].Name, cols[0]) && out[i].Version == cols[1] {
				out[i].Available = cols[2]
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("chocolatey installed packages changed; scan again")
		}
	}
	return out, nil
}

const snapUpToDate = "All snaps up to date."

func snapUpdates(installed []inventory.SystemPackage, data []byte) ([]inventory.SystemPackage, error) {
	text := strings.TrimSpace(string(data))
	out := slices.Clone(installed)
	for i := range out {
		out[i].Available = ""
	}
	if text == snapUpToDate {
		return out, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "Name ") {
		return nil, fmt.Errorf("invalid Snap updates table")
	}
	for _, line := range lines[1:] {
		cols := strings.Fields(line)
		if len(cols) == 0 {
			continue
		}
		if len(cols) < 4 {
			return nil, fmt.Errorf("invalid Snap updates row")
		}
		matched := false
		for i := range out {
			if out[i].Name == cols[0] {
				out[i].Available = cols[1] + " (" + cols[2] + ")"
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("snap installed packages changed; scan again")
		}
	}
	return out, nil
}

func parseFlatpak(data []byte) ([]inventory.SystemPackage, error) {
	var out []inventory.SystemPackage
	for line := range strings.SplitSeq(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != 5 {
			return nil, fmt.Errorf("invalid Flatpak installed row")
		}
		ref := strings.Split(cols[0], "/")
		if len(ref) != 4 || (ref[0] != "app" && ref[0] != "runtime") || cols[2] == "" {
			return nil, fmt.Errorf("invalid Flatpak reference")
		}
		version := cols[1]
		if version == "" {
			version = cols[4]
		}
		out = append(out, inventory.SystemPackage{Name: ref[1], Version: version, Kind: ref[0], Architecture: ref[2], Reference: cols[0], Scope: cols[2], Origin: cols[3]})
	}
	return out, nil
}

func queryFlatpak(probe networkProbe) ([]inventory.SystemPackage, error) {
	res, err := probe("list", "--columns=ref,version,installation,origin,commit")
	if err != nil {
		return nil, err
	}
	out, err := parseFlatpak(res.Stdout)
	if err != nil {
		return nil, err
	}
	scopes := map[string]bool{}
	for _, p := range out {
		scopes[p.Scope] = true
	}
	for scope := range scopes {
		where := "--installation=" + scope
		if scope == "user" {
			where = "--user"
		}
		if scope == "system" {
			where = "--system"
		}
		res, err = probe("remote-ls", where, "--updates", "--all", "--arch=*", "--columns=ref,version,commit,origin")
		if err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(strings.TrimRight(string(res.Stdout), "\n"), "\n") {
			if line == "" {
				continue
			}
			cols := strings.Split(line, "\t")
			if len(cols) != 4 || cols[0] == "" || cols[2] == "" {
				return nil, fmt.Errorf("invalid Flatpak updates row")
			}
			for i, p := range out {
				if p.Scope == scope && p.Reference == cols[0] && p.Origin == cols[3] {
					available := cols[1]
					if available == "" || available == p.Version {
						available = cols[2]
					}
					out[i].Available = available
				}
			}
		}
	}
	return out, nil
}
