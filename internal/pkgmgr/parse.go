package pkgmgr

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

var apkVersion = regexp.MustCompile(`^(.+)-(\d[^\s]*-r\d+)$`)

func parse(format string, data []byte) ([]inventory.SystemPackage, error) {
	var out []inventory.SystemPackage
	if format == "nix" {
		var entries map[string]struct {
			Name, Version, Pname string
			Outputs              map[string]string
		}
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, fmt.Errorf("nix installed: %w", err)
		}
		for _, e := range entries {
			name := e.Pname
			if name == "" {
				name = e.Name
			}
			out = append(out, inventory.SystemPackage{Name: name, Version: e.Version, Path: e.Outputs["out"], Kind: "profile"})
		}
		return out, nil
	}
	if format == "zypper" {
		var doc struct {
			Entries []struct {
				Name    string `xml:"name,attr"`
				Version string `xml:"edition,attr"`
			} `xml:"search-result>solvable-list>solvable"`
		}
		if err := xml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("zypper installed: %w", err)
		}
		for _, e := range doc.Entries {
			out = append(out, inventory.SystemPackage{Name: e.Name, Version: e.Version, Kind: "package"})
		}
		return out, nil
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		p := inventory.SystemPackage{Kind: "package"}
		f := strings.Fields(line)
		switch format {
		case "apt":
			name, _, ok := strings.Cut(f[0], "/")
			if !ok || len(f) < 2 {
				continue
			}
			p.Name, p.Version = name, f[1]
		case "pairs":
			if len(f) < 2 {
				continue
			}
			p.Name, p.Version = f[0], f[1]
			if len(f) == 4 && f[2] == "->" {
				p.Version = f[3]
			}
		case "rpm":
			if len(f) < 3 || !strings.Contains(f[0], ".") || f[1] == "Packages" {
				continue
			}
			p.Name = f[0][:strings.LastIndex(f[0], ".")]
			p.Version = f[1]
		case "tsv":
			cols := strings.Split(line, "\t")
			if len(cols) < 2 {
				continue
			}
			p.Name, p.Version = cols[0], cols[1]
		case "choco":
			cols := strings.Split(line, "|")
			if len(cols) != 2 {
				continue
			}
			p.Name, p.Version = cols[0], cols[1]
		case "snap":
			if len(f) < 3 || f[0] == "Name" {
				continue
			}
			p.Name, p.Version = f[0], f[1]
		case "apk":
			cols := apkVersion.FindStringSubmatch(line)
			if len(cols) != 3 {
				continue
			}
			p.Name, p.Version = cols[1], cols[2]
		default:
			return nil, fmt.Errorf("unknown package format %s", format)
		}
		if p.Name != "" {
			out = append(out, p)
		}
	}
	return out, nil
}
