package pkgmgr

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
)

// HasNetworkQuery distinguishes managers with a separate online query from
// those already reporting their locally cached repository data.
func HasNetworkQuery(id string) bool {
	return slices.Contains([]string{"winget", "choco", "snap", "flatpak", "nix"}, id)
}

// Network checks availability only. Managers may refresh their own metadata;
// no install, upgrade, source agreement or profile change is requested.
func Network(ctx context.Context, c *versions.Cache, r run.Runner, env sysenv.Env, m inventory.PackageManager) inventory.PackageManager {
	if !HasNetworkQuery(m.ID) {
		return m
	}
	m.Packages = slices.Clone(m.Packages)
	identity := slices.Clone(m.Packages)
	for i := range identity {
		identity[i].Available = ""
	}
	slices.SortFunc(identity, func(a, b inventory.SystemPackage) int {
		return strings.Compare(a.Name+"\x00"+a.Version+"\x00"+a.Path+"\x00"+a.Scope+"\x00"+a.Reference, b.Name+"\x00"+b.Version+"\x00"+b.Path+"\x00"+b.Scope+"\x00"+b.Reference)
	})
	keyEnv := queryEnv(env)
	keyData, _ := json.Marshal([]any{3, m.ID, m.Path, m.Prefix, keyEnv, identity})
	key := fmt.Sprintf("packages-%x.json", sha256.Sum256(keyData))
	data, fetched, err := c.Query(ctx, key, func(ctx context.Context) ([]byte, error) {
		packages, e := queryNetwork(ctx, r, env, m)
		if e != nil && (m.ID != "winget" || len(packages) == 0) {
			return nil, e
		}
		result := networkResult{Packages: packages, Complete: e == nil}
		if e != nil {
			result.Warning = "some package sources could not be queried: " + strings.ReplaceAll(e.Error(), "\n", "; ")
		}
		if m.ID == "winget" {
			for _, local := range m.Packages {
				i := slices.IndexFunc(result.Packages, func(p inventory.SystemPackage) bool {
					return strings.EqualFold(p.Name, local.Name) && p.Version == local.Version
				})
				if i >= 0 {
					if local.Path != "" {
						result.Packages[i].Path = local.Path
					}
					if local.Kind != "" {
						result.Packages[i].Kind = local.Kind
					}
				} else {
					local.Available = ""
					result.Packages = append(result.Packages, local)
					result.Complete = false
					result.Warning = "some locally registered packages have no matched source"
				}
			}
		}
		return json.Marshal(result)
	})
	m.UpdateError = ""
	if len(data) > 0 {
		var result networkResult
		if e := json.Unmarshal(data, &result); e != nil {
			err = fmt.Errorf("read cached package query: %w", e)
		} else {
			m.Packages, m.UpdatesKnown, m.UpdatesFetched = result.Packages, result.Complete, fetched
			m.UpdateError = result.Warning
			if m.ID == "winget" {
				m.Unlisted = !result.Complete
			}
		}
	}
	if err != nil && !errors.Is(err, versions.ErrOffline) {
		m.UpdateError = err.Error()
	}
	return m
}

// queryEnv keeps the variables that select a manager's installation and
// sources. Session values such as SSH_AUTH_SOCK change on every login and
// would make the offline cache unreachable.
func queryEnv(env sysenv.Env) []string {
	var out []string
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		switch {
		case slices.Contains([]string{"PATH", "Path", "HOME", "USERPROFILE", "LOCALAPPDATA", "ProgramData", "ChocolateyInstall", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_DATA_DIRS"}, name),
			strings.HasPrefix(name, "NIX_"), strings.HasPrefix(name, "FLATPAK_"):
			out = append(out, kv)
		}
	}
	slices.Sort(out)
	return out
}

type networkResult struct {
	Packages []inventory.SystemPackage
	Complete bool
	Warning  string
}

type networkProbe func(args ...string) (run.Result, error)

func queryNetwork(ctx context.Context, r run.Runner, env sysenv.Env, m inventory.PackageManager) ([]inventory.SystemPackage, error) {
	env = append(slices.Clone(env), "LC_ALL=C", "LANG=C", "NO_COLOR=1", "TERM=dumb", "FLATPAK_FANCY_OUTPUT=0")
	probe := func(args ...string) (run.Result, error) {
		res, err := r.Run(ctx, run.Cmd{Args: append([]string{m.Path}, args...), Env: env, Timeout: 60 * time.Second, MaxOutput: 16 << 20})
		if err != nil {
			return res, err
		}
		if res.Truncated {
			return res, fmt.Errorf("package query output truncated")
		}
		if res.ExitCode != 0 && (m.ID != "choco" || res.ExitCode != 2) {
			return res, queryExitError(res)
		}
		// Nix prints evaluation warnings of nixpkgs on any successful query.
		if len(strings.TrimSpace(string(res.Stderr))) > 0 && m.ID != "nix" {
			return res, fmt.Errorf("package query: %s", strings.TrimSpace(string(res.Stderr)))
		}
		return res, nil
	}
	switch m.ID {
	case "winget":
		return queryWinget(probe)
	case "choco":
		res, err := probe("outdated", "--limit-output", "--no-color")
		if err != nil {
			return nil, err
		}
		return chocoUpdates(m.Packages, res.Stdout)
	case "snap":
		res, err := probe("refresh", "--list")
		// snap reports the no-updates case on stderr.
		if err != nil && res.ExitCode == 0 && !res.Truncated && strings.TrimSpace(string(res.Stderr)) == snapUpToDate {
			res.Stdout, err = res.Stderr, nil
		}
		if err != nil {
			return nil, err
		}
		return snapUpdates(m.Packages, res.Stdout)
	case "flatpak":
		return queryFlatpak(probe)
	case "nix":
		return queryNixNetwork(ctx, r, env, m, probe)
	}
	return nil, fmt.Errorf("unsupported package query %s", m.ID)
}

// queryExitError names a failed query by its exit code, in hex for Windows'
// HRESULTs as WinGet's 0x8a15005e, and by the last line it printed, where
// WinGet and the others say why.
func queryExitError(res run.Result) error {
	code := strconv.Itoa(res.ExitCode)
	if res.ExitCode < 0 || res.ExitCode > 255 {
		code = fmt.Sprintf("%#x", uint32(res.ExitCode))
	}
	lines := strings.Split(strings.TrimSpace(string(res.Stderr)+"\n"+string(res.Stdout)), "\n")
	if last := strings.TrimSpace(renderLine(lines[len(lines)-1])); last != "" {
		return fmt.Errorf("package query exited %s: %s", code, last)
	}
	return fmt.Errorf("package query exited %s", code)
}
