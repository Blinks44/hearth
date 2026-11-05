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
	if format == "flatpak" {
		return parseFlatpak(data)
	}
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
	if format == "zypper" || format == "zypper-updates" {
		return parseZypper(format, data)
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
		case "snap":
			if len(f) < 3 || f[0] == "Name" {
				continue
			}
			p.Name, p.Version = f[0], f[1]
		case "apk-updates":
			if strings.HasPrefix(line, "Installed:") {
				continue
			}
			if len(f) < 3 || f[1] != "<" {
				return nil, fmt.Errorf("apk updates: invalid row %q", line)
			}
			cols := apkVersion.FindStringSubmatch(f[0])
			if len(cols) != 3 {
				return nil, fmt.Errorf("apk updates: invalid package %q", f[0])
			}
			p.Name, p.Version = cols[1], f[2]
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

// parseZypper keeps the architecture so an x86_64 update cannot be applied
// to an i586 installation of the same package.
func parseZypper(format string, data []byte) ([]inventory.SystemPackage, error) {
	type entry struct {
		Name    string `xml:"name,attr"`
		Version string `xml:"edition,attr"`
		Arch    string `xml:"arch,attr"`
	}
	var doc struct {
		XMLName   xml.Name `xml:"stream"`
		Installed []entry  `xml:"search-result>solvable-list>solvable"`
		Updates   *struct {
			Entries []entry `xml:"update-list>update"`
		} `xml:"update-status"`
		Messages []struct {
			Type string `xml:"type,attr"`
			Text string `xml:",chardata"`
		} `xml:"message"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("zypper packages: %w", err)
	}
	entries := doc.Installed
	if format == "zypper-updates" {
		if doc.Updates == nil {
			return nil, fmt.Errorf("zypper updates: missing update-status")
		}
		for _, msg := range doc.Messages {
			if msg.Type == "error" || msg.Type == "warning" {
				return nil, fmt.Errorf("zypper updates: %s", strings.TrimSpace(msg.Text))
			}
		}
		entries = doc.Updates.Entries
	}
	var out []inventory.SystemPackage
	for _, e := range entries {
		if e.Name == "" || e.Version == "" {
			return nil, fmt.Errorf("zypper packages: missing name or edition")
		}
		out = append(out, inventory.SystemPackage{Name: e.Name, Version: e.Version, Architecture: e.Arch, Kind: "package"})
	}
	return out, nil
}
