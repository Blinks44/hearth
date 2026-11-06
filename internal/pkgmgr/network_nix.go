package pkgmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func queryNixNetwork(ctx context.Context, r run.Runner, env sysenv.Env, m inventory.PackageManager, probe networkProbe) ([]inventory.SystemPackage, error) {
	out := slices.Clone(m.Packages)
	for i := range out {
		out[i].Available = ""
	}
	home := env.Get("HOME")
	if !filepath.IsAbs(home) {
		return nil, fmt.Errorf("nix home directory unavailable")
	}
	state := env.Get("XDG_STATE_HOME")
	if !filepath.IsAbs(state) {
		state = filepath.Join(home, ".local", "state")
	}
	refs := map[string]string{}
	modern := false
	for _, profile := range []string{filepath.Join(home, ".nix-profile"), filepath.Join(state, "nix", "profile")} {
		data, err := os.ReadFile(filepath.Join(profile, "manifest.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read Nix profile: %w", err)
		}
		entries, err := nixUpdateRefs(data)
		if err != nil {
			return nil, err
		}
		modern = true
		maps.Copy(refs, entries)
	}
	nix := filepath.Join(filepath.Dir(m.Path), "nix")

	for i, p := range out {
		ref, found := refs[p.Path]
		if !found {
			continue
		}
		if ref == "" {
			continue
		} // A store-path-only element has no upgrade source.
		res, err := r.Run(ctx, run.Cmd{Args: []string{nix, "eval", "--json", "--refresh", "--no-write-lock-file", "--option", "allow-import-from-derivation", "false", "--option", "accept-flake-config", "false", "--apply", `p: { version = p.version or ""; outPath = p.outPath or ""; }`, "--", ref}, Env: env, Timeout: 60 * time.Second, MaxOutput: 1 << 20})
		if err != nil {
			return nil, fmt.Errorf("nix version query: %w", err)
		}
		if res.ExitCode != 0 || res.Truncated {
			return nil, fmt.Errorf("nix version query failed (exit %d)", res.ExitCode)
		}
		var candidate struct {
			Version string
			OutPath string
		}
		if err := json.Unmarshal(res.Stdout, &candidate); err != nil {
			return nil, fmt.Errorf("read Nix version: %w", err)
		}
		cmp, ok := packageVersionCompare(candidate.Version, p.Version)
		if !ok {
			return nil, fmt.Errorf("cannot compare Nix package versions")
		}
		if cmp > 0 {
			out[i].Available = candidate.Version
		} else if cmp == 0 && candidate.OutPath != "" && candidate.OutPath != p.Path {
			revision, _, _ := strings.Cut(filepath.Base(candidate.OutPath), "-")
			if len(revision) > 12 {
				revision = revision[:12]
			}
			out[i].Available = candidate.Version + " (" + revision + ")"
		}
	}
	classic := !modern
	for _, p := range out {
		if _, ok := refs[p.Path]; !ok {
			classic = true
		}
	}
	if classic {
		// --available --json would print all of nixpkgs, tens of megabytes.
		res, err := probe("--option", "allow-import-from-derivation", "false", "--query", "--installed", "--compare-versions")
		if err != nil {
			return nil, err
		}
		for line := range strings.Lines(string(res.Stdout)) {
			f := strings.Fields(line) // hello-2.12.1  < 2.13.0
			if len(f) != 3 || f[1] != "<" {
				continue
			}
			for i, p := range out {
				if _, ok := refs[p.Path]; ok || p.Name+"-"+p.Version != f[0] {
					continue
				}
				if current := out[i].Available; current != "" {
					if cmp, ok := packageVersionCompare(f[2], current); !ok || cmp <= 0 {
						continue
					}
				}
				out[i].Available = f[2]
			}
		}
	}
	return out, nil
}

func nixUpdateRefs(data []byte) (map[string]string, error) {
	type element struct {
		StorePaths  []string
		OriginalURL string
		OriginalURI string
		URL         string
		URI         string
		AttrPath    string
	}
	var doc struct {
		Version  int
		Elements json.RawMessage
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("read Nix manifest: %w", err)
	}
	var entries []element
	switch doc.Version {
	case 1, 2:
		if err := json.Unmarshal(doc.Elements, &entries); err != nil {
			return nil, fmt.Errorf("read Nix elements: %w", err)
		}
	case 3:
		var named map[string]element
		if err := json.Unmarshal(doc.Elements, &named); err != nil {
			return nil, fmt.Errorf("read Nix elements: %w", err)
		}
		for _, e := range named {
			entries = append(entries, e)
		}
	default:
		return nil, fmt.Errorf("unsupported Nix manifest version %d", doc.Version)
	}
	refs := map[string]string{}
	for _, e := range entries {
		ref := ""
		source := e.OriginalURL
		if source == "" {
			source = e.OriginalURI
		}
		if source == "" {
			source = e.URL
		}
		if source == "" {
			source = e.URI
		}
		if source != "" && e.AttrPath != "" {
			ref = source + "#" + e.AttrPath
		}
		for _, path := range e.StorePaths {
			refs[path] = ref
		}
	}
	return refs, nil
}

func packageVersionCompare(a, b string) (int, bool) {
	if a == b {
		return 0, true
	}
	if cmp, ok := compareDottedVersions(a, b); ok {
		return cmp, true
	}
	av, ae := semver.NewVersion(a)
	bv, be := semver.NewVersion(b)
	if ae != nil || be != nil {
		return 0, false
	}
	return av.Compare(bv), true
}
