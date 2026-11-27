package detect

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Dotnet lists SDKs and shared frameworks separately for each host. It never
// invokes an SDK command, which could initialize state or restore packages.
func Dotnet(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error) {
	f := newFinder("dotnet", term)
	f.lookPath(dotnetExe, term, gui)
	d := sources.ManagerDirs(term)
	roots := dotnetRoots(term)
	for _, env := range []sysenv.Env{term, gui} {
		for _, key := range []string{"DOTNET_ROOT", "DOTNET_ROOT_ARM64", "DOTNET_ROOT_X64", "DOTNET_ROOT_X86"} {
			if root := env.Get(key); root != "" {
				roots = append(roots, root)
			}
		}
	}
	if d.Home != "" {
		roots = append(roots, filepath.Join(d.Home, ".dotnet"))
	}
	for _, base := range []string{d.Mise, d.Asdf} {
		if base == "" {
			continue
		}
		for _, tool := range []string{"dotnet", "dotnet-core"} {
			paths, _ := filepath.Glob(filepath.Join(base, "installs", tool, "*"))
			roots = append(roots, paths...)
		}
	}
	for _, root := range roots {
		if path := filepath.Join(root, dotnetExe); exists(path) {
			f.add(path)
		}
	}
	st := inventory.ToolState{Tool: "dotnet", Installations: f.ins, Vars: map[string]string{}}
	for _, key := range []string{"DOTNET_ROOT", "DOTNET_ROOT_ARM64", "DOTNET_ROOT_X64", "DOTNET_ROOT_X86", "DOTNET_CLI_TELEMETRY_OPTOUT"} {
		st.Vars[key] = term.Get(key)
	}
	for i := range st.Installations {
		in := &st.Installations[i]
		if in.Source.Shim {
			continue
		}
		// mise's one root holds all its SDKs: its shim runs that dotnet.
		in.Default = in.Source.Manager == "mise"
		info := &inventory.Dotnet{}
		in.Dotnet = info
		for _, probe := range []struct {
			flag   string
			target *[]inventory.DotnetEntry
			sdk    bool
		}{
			{"--list-sdks", &info.SDKs, true}, {"--list-runtimes", &info.Runtimes, false},
		} {
			res, err := r.Run(ctx, run.Cmd{Args: []string{in.RealPath, probe.flag}, Env: term})
			if err != nil {
				info.Err += fmt.Sprintf("%s: %v; ", probe.flag, err)
				continue
			}
			if res.ExitCode != 0 || res.Truncated {
				info.Err += fmt.Sprintf("%s: exit %d, truncated %t; ", probe.flag, res.ExitCode, res.Truncated)
				continue
			}
			*probe.target = dotnetList(string(res.Stdout), probe.sdk)
		}
		for _, sdk := range info.SDKs {
			v, err := semver.StrictNewVersion(sdk.Version)
			old, _ := semver.StrictNewVersion(in.Version)
			if err == nil && (old == nil || v.GreaterThan(old)) {
				in.Version = sdk.Version
			}
		}
	}
	if slices.ContainsFunc(st.Installations, func(in inventory.Installation) bool { return in.Source.Manager == "mise" }) {
		st.Managers = append(st.Managers, miseManager(ctx, r, term, gui, d))
	}
	if slices.ContainsFunc(st.Installations, func(in inventory.Installation) bool { return in.Source.Manager == "asdf" }) {
		st.Managers = append(st.Managers, inventory.Manager{Name: "asdf", SwitchOnCD: true, Files: []string{".tool-versions"}})
	}
	if active := inventory.TerminalCopy(st.Installations); active >= 0 {
		st.Installations[active].Packages = dotnetTools(d.Home)
	}
	return st, ctx.Err()
}

func dotnetList(out string, sdk bool) []inventory.DotnetEntry {
	var entries []inventory.DotnetEntry
	for line := range strings.SplitSeq(out, "\n") {
		left, path, ok := strings.Cut(strings.TrimSpace(line), " [")
		if !ok || !strings.HasSuffix(path, "]") {
			continue
		}
		fields := strings.Fields(left)
		name, version := "SDK", ""
		if sdk && len(fields) == 1 {
			version = fields[0]
		}
		if !sdk && len(fields) == 2 {
			name, version = fields[0], fields[1]
		}
		if _, err := semver.StrictNewVersion(version); err != nil {
			continue
		}
		entry := inventory.DotnetEntry{Name: name, Version: version, Path: filepath.Join(strings.TrimSuffix(path, "]"), version)}
		if !slices.Contains(entries, entry) {
			entries = append(entries, entry)
		}
	}
	slices.SortFunc(entries, func(a, b inventory.DotnetEntry) int {
		av, _ := semver.StrictNewVersion(a.Version)
		bv, _ := semver.StrictNewVersion(b.Version)
		return bv.Compare(av)
	})
	return entries
}

// dotnetTools reads tool manifests rather than invoking an SDK and triggering
// first-run setup. A failed manifest does not hide the other tools.
func dotnetTools(home string) []inventory.Package {
	if home == "" {
		return nil
	}
	var packages []inventory.Package
	store := filepath.Join(home, ".dotnet", "tools", ".store")
	files, _ := filepath.Glob(filepath.Join(store, "*", "*", "*", "*", "tools", "*", "any", "DotnetToolSettings.xml"))
	seen := map[string]bool{}
	for _, file := range files {
		rel, err := filepath.Rel(store, file)
		if err != nil {
			continue
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 2 {
			continue
		}
		key := parts[0] + "@" + parts[1]
		if seen[key] {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		var settings struct {
			Commands []struct {
				Name string `xml:"Name,attr"`
			} `xml:"Commands>Command"`
		}
		if xml.Unmarshal(data, &settings) != nil {
			continue
		}
		p := inventory.Package{Name: parts[0], Version: parts[1], Manager: "dotnet"}
		for _, cmd := range settings.Commands {
			if cmd.Name != "" {
				p.Commands = append(p.Commands, cmd.Name)
			}
		}
		packages = append(packages, p)
		seen[key] = true
	}
	return packages
}
