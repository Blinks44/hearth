package projects

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/Blinks44/hearth/internal/inventory"
)

// SDKPolicy controls the SDK muxer's selection, independent of the runtime.
type SDKPolicy struct {
	Version         string
	RollForward     string
	AllowPrerelease *bool
	Paths           []string
}

func readGlobalJSON(data []byte) ([]Requirement, error) {
	var doc struct{ SDK SDKPolicy }
	if err := json.Unmarshal(stripJSONComments(data), &doc); err != nil {
		return nil, fmt.Errorf("global.json: %w", err)
	}
	return []Requirement{{Tool: "dotnet", Want: doc.SDK.Version, Key: "sdk.version", SDK: &doc.SDK}}, nil
}

// stripJSONComments preserves strings and line positions in global.json.
func stripJSONComments(data []byte) []byte {
	out := slices.Clone(data)
	quoted, escaped := false, false
	for i := 0; i < len(out); i++ {
		b := out[i]
		if quoted {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		if b == '"' {
			quoted = true
			continue
		}
		if b != '/' || i+1 >= len(out) {
			continue
		}
		switch out[i+1] {
		case '/':
			for i < len(out) && out[i] != '\n' {
				out[i] = ' '
				i++
			}
		case '*':
			out[i], out[i+1] = ' ', ' '
			i += 2
			for i+1 < len(out) && (out[i] != '*' || out[i+1] != '/') {
				if out[i] != '\n' {
					out[i] = ' '
				}
				i++
			}
			if i+1 >= len(out) {
				return data
			}
			out[i], out[i+1] = ' ', ' '
			i++
		}
	}
	return out
}

func resolveDotnet(st *Status, ts inventory.ToolState) {
	st.Installations = nil
	active := inventory.TerminalCopy(ts.Installations)
	// The host on PATH decides; a failed list of another host does not.
	if active >= 0 && ts.Installations[active].Dotnet != nil && ts.Installations[active].Dotnet.Err != "" ||
		active < 0 && slices.ContainsFunc(ts.Installations, func(in inventory.Installation) bool { return in.OnPath == 1 && in.Source.Shim }) {
		st.Checked = false
		return
	}
	for _, host := range ts.Installations {
		if host.Dotnet == nil {
			continue
		}
		for _, sdk := range host.Dotnet.SDKs {
			in := host
			in.Version, in.Path, in.RealPath = sdk.Version, sdk.Path, sdk.Path
			st.Installations = append(st.Installations, in)
		}
	}
	// Only the SDKs belonging to the host on PATH can run in this folder.
	var visible []inventory.Installation
	if active >= 0 && ts.Installations[active].Dotnet != nil {
		host := ts.Installations[active]
		for _, sdk := range host.Dotnet.SDKs {
			in := host
			in.Version, in.Path, in.RealPath = sdk.Version, sdk.Path, sdk.Path
			visible = append(visible, in)
		}
	}
	var policy SDKPolicy
	for _, check := range st.Checks {
		if check.SDK != nil {
			policy = *check.SDK
			break
		}
	}
	selected, selectionErr := selectSDK(visible, policy)
	st.Terminal = selected
	for i := range st.Checks {
		ch := &st.Checks[i]
		if selectionErr != nil {
			ch.Err = selectionErr.Error()
			continue
		}
		var err error
		if ch.SDK != nil {
			ch.Installed, err = selectSDK(st.Installations, *ch.SDK)
			ch.Met = selected != nil
		} else {
			var match func(inventory.Installation) bool
			match, err = matcher("dotnet", ch.Want, nil)
			if err == nil {
				ch.Installed = newest(st.Installations, match)
				ch.Met = selected != nil && match(*selected)
			}
		}
		if err != nil {
			ch.Err = err.Error()
		}
	}
}

// selectSDK implements the CLI roll-forward policies. Unsupported search paths
// remain unknown rather than producing a false missing-SDK problem.
func selectSDK(ins []inventory.Installation, policy SDKPolicy) (*inventory.Installation, error) {
	if len(policy.Paths) > 0 {
		return nil, fmt.Errorf("global.json sdk.paths is not supported")
	}
	roll := policy.RollForward
	if roll == "" {
		roll = "patch"
	}
	valid := []string{"patch", "feature", "minor", "major", "latestPatch", "latestFeature", "latestMinor", "latestMajor", "disable"}
	if !slices.Contains(valid, roll) {
		return nil, fmt.Errorf("unknown SDK rollForward %q", roll)
	}
	var want *semver.Version
	if policy.Version != "" {
		var err error
		want, err = semver.StrictNewVersion(policy.Version)
		if err != nil {
			return nil, fmt.Errorf("SDK version: %w", err)
		}
	} else if policy.RollForward != "" && roll != "latestMajor" {
		return nil, fmt.Errorf("rollForward %s requires sdk.version", roll)
	}
	var best *inventory.Installation
	var bestV *semver.Version
	for i, in := range ins {
		v, err := semver.StrictNewVersion(in.Version)
		if err != nil || policy.AllowPrerelease != nil && !*policy.AllowPrerelease && v.Prerelease() != "" {
			continue
		}
		if want != nil {
			if v.LessThan(want) {
				continue
			}
			sameBand := v.Major() == want.Major() && v.Minor() == want.Minor() && v.Patch()/100 == want.Patch()/100
			switch roll {
			case "disable":
				if !v.Equal(want) {
					continue
				}
			case "patch", "latestPatch":
				if !sameBand {
					continue
				}
			case "feature", "latestFeature":
				if v.Major() != want.Major() || v.Minor() != want.Minor() {
					continue
				}
			case "minor", "latestMinor":
				if v.Major() != want.Major() {
					continue
				}
			}
			if roll == "patch" && v.Equal(want) {
				return &ins[i], nil
			}
		}
		if best == nil {
			best, bestV = &ins[i], v
			continue
		}
		// Non-latest policies choose the nearest band, then its newest patch.
		nearest := want != nil && !strings.HasPrefix(roll, "latest") && roll != "patch" && roll != "disable"
		band, bestBand := [3]uint64{v.Major(), v.Minor(), v.Patch() / 100}, [3]uint64{bestV.Major(), bestV.Minor(), bestV.Patch() / 100}
		cmp := slices.Compare(band[:], bestBand[:])
		if nearest && cmp < 0 || (!nearest || cmp == 0) && v.GreaterThan(bestV) {
			best, bestV = &ins[i], v
		}
	}
	return best, nil
}

// readInheritedGlobalJSON follows the CLI's search upwards. A local file, even
// one that could not be parsed, takes precedence over every ancestor. Only a
// .NET project inherits one: a Node folder under a repository's global.json
// does not need an SDK.
func readInheritedGlobalJSON(p *Project) {
	if slices.Contains(p.Files, "global.json") || !hasDotnetProject(p.Dir) {
		return
	}
	dir := filepath.Dir(p.Dir)
	for {
		file := filepath.Join(dir, "global.json")
		data, err := os.ReadFile(file)
		if err == nil {
			relative, relErr := filepath.Rel(p.Dir, file)
			if relErr != nil {
				p.Errs = append(p.Errs, relErr)
				return
			}
			p.Files = append(p.Files, relative)
			reqs, parseErr := readGlobalJSON(data)
			if parseErr != nil {
				p.Errs = append(p.Errs, fmt.Errorf("read %s: %w", file, parseErr))
				return
			}
			for _, req := range reqs {
				req.File = relative
				p.Requirements = append(p.Requirements, req)
			}
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			p.Errs = append(p.Errs, fmt.Errorf("read %s: %w", file, err))
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

// hasDotnetProject reports a solution or project file directly in dir.
func hasDotnetProject(dir string) bool {
	entries, _ := os.ReadDir(dir)
	return slices.ContainsFunc(entries, func(e os.DirEntry) bool {
		return !e.IsDir() && slices.Contains([]string{".sln", ".slnx", ".csproj", ".fsproj", ".vbproj"}, strings.ToLower(filepath.Ext(e.Name())))
	})
}
