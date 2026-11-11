package ops

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/disk"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// cacheCommands are the commands a tool clears its own cache with, by
// cache ID. A cache without one, or whose tool the terminal does not
// find, is removed as a folder. Homebrew's is a folder: brew cleanup
// also removes older versions of formulae, which are not its cache;
// Yarn Berry's cache clean works only inside a project.
var cacheCommands = map[string][]string{
	"npm":       {"npm", "cache", "clean", "--force"},
	"pnpm":      {"pnpm", "store", "prune"},
	"yarn":      {"yarn", "cache", "clean"},
	"bun":       {"bun", "pm", "cache", "rm"},
	"uv":        {"uv", "cache", "clean"},
	"pip":       {"pip", "cache", "purge"},
	"go-mod":    {"go", "clean", "-modcache"},
	"go-build":  {"go", "clean", "-cache"},
	"composer":  {"composer", "clear-cache"},
	"ccache":    {"ccache", "-C"},
	"cocoapods": {"pod", "cache", "clean", "--all"},
	"pub":       {"flutter", "pub", "cache", "clean", "--force"},
}

// CacheCommand is the command the tool of the cache id clears it with;
// nil when Hearth removes its folder.
func CacheCommand(id string) []string { return cacheCommands[id] }

// cleanupTimeout ends a step of a cleanup: a big cache takes a while to
// remove.
const cleanupTimeout = 10 * time.Minute

// DockerTarget is the Docker CLI's path and the context whose engine a
// cleanup prunes.
type DockerTarget struct{ CLI, Context string }

// CountDocker asks Docker again what pruning would free, for what a
// cleanup freed.
type CountDocker func(ctx context.Context) ([]inventory.DockerUsage, error)

// Cleans reports that a cleanup plan removes c, found in s: caches and
// Docker's data; copies through their version manager or Homebrew;
// Homebrew's packages nothing needs.
func Cleans(c cleanup.Candidate, s *inventory.Snapshot) bool {
	switch c.Kind {
	case cleanup.Cache, cleanup.BuildCache, cleanup.UnusedImages, cleanup.StoppedContainers:
		return true
	case cleanup.ExtraCopy, cleanup.UnneededPackage:
		if _, _, ok := brewPackage(c, s); ok {
			return true
		}
		_, ok := copyTargets(c, s)
		return ok
	case cleanup.Leftover:
		_, ok := copyTargets(c, s)
		return ok
	}
	return false
}

// copyTargets are the versions c's copies are, one by root, as their
// version manager removes them; ok is false when it cannot remove one.
func copyTargets(c cleanup.Candidate, s *inventory.Snapshot) (ts []VersionTarget, ok bool) {
	if c.Kind != cleanup.ExtraCopy && c.Kind != cleanup.Leftover {
		return nil, false
	}
	for _, root := range c.Paths {
		i := slices.IndexFunc(c.Copies, func(in inventory.Installation) bool { return in.Source.Root == root })
		if i < 0 {
			return nil, false
		}
		in := c.Copies[i]
		if in.Source.Kind != inventory.KindVersionManager {
			return nil, false
		}
		t := VersionTarget{Tool: in.Tool, Manager: in.Source.Manager, Version: cmp.Or(in.Source.Version, in.Version), Root: root, AsdfSet: true}
		for _, st := range s.Tools {
			for _, m := range st.Managers {
				switch {
				case m.Name != t.Manager:
				case m.Name == "asdf":
					t.AsdfSet = AsdfSet(m.Version)
				default:
					t.Admin = t.Admin || m.Admin
				}
			}
		}
		if t.Version == "" || versionCommand(VersionUninstall, t) == nil {
			return nil, false
		}
		ts = append(ts, t)
	}
	return ts, len(ts) > 0
}

// brewPackage is Homebrew and its package c is: a copy in its keg, or a
// package nothing needs. brew uninstall removes every installed version
// of a formula, so one with another version is not offered.
func brewPackage(c cleanup.Candidate, s *inventory.Snapshot) (inventory.PackageManager, inventory.SystemPackage, bool) {
	switch {
	case c.Kind == cleanup.ExtraCopy && len(c.Paths) == 1 && len(c.Copies) > 0 && c.Copies[0].Source.Manager == "brew":
	case c.Kind == cleanup.UnneededPackage && c.Manager == "brew":
	default:
		return inventory.PackageManager{}, inventory.SystemPackage{}, false
	}
	for _, st := range s.Tools {
		for _, m := range st.PackageManagers {
			if m.ID != "brew" {
				continue
			}
			i := slices.IndexFunc(m.Packages, func(p inventory.SystemPackage) bool { return p.Path == c.Paths[0] })
			if i < 0 {
				continue
			}
			pkg := m.Packages[i]
			if slices.ContainsFunc(m.Packages, func(p inventory.SystemPackage) bool { return p.Name == pkg.Name && p.Path != pkg.Path }) {
				return inventory.PackageManager{}, inventory.SystemPackage{}, false
			}
			return m, pkg, true
		}
	}
	return inventory.PackageManager{}, inventory.SystemPackage{}, false
}

// dockerPrune are the docker commands that remove the Docker candidates.
var dockerPrune = map[cleanup.Kind][]string{
	cleanup.BuildCache:        {"builder", "prune", "--force"},
	cleanup.UnusedImages:      {"image", "prune", "--all", "--force"},
	cleanup.StoppedContainers: {"container", "prune", "--force"},
}

// CleanupPlan returns the plan that removes cands, found in s, titled as
// titles says each: a cache through its tool's command when the terminal
// finds it, or as its folder; Docker's through its prune commands; a
// copy through its version manager, or as its folder when the terminal
// does not find the manager; Homebrew's packages through one brew
// uninstall, first. Candidates it does not remove are left out. Its check counts
// again what they take and says what was freed; docker, nil without an
// engine, asks Docker.
func CleanupPlan(cands []cleanup.Candidate, titles []string, s *inventory.Snapshot, env sysenv.Env, target DockerTarget, docker CountDocker) (Plan, error) {
	p := Plan{Title: locales.CleanupPlanTitle(), Why: locales.CleanupPlanWhy()}
	var done []cleanup.Candidate
	var brew inventory.PackageManager
	var pkgs []inventory.SystemPackage
	removes, pub, versions, admin := false, false, false, false
	for i, c := range cands {
		if !Cleans(c, s) {
			continue
		}
		var steps []Step
		switch c.Kind {
		case cleanup.Cache:
			argv := CacheCommand(c.ID)
			if len(argv) > 0 && len(sysenv.LookPathAll(argv[0], env)) > 0 {
				steps = []Step{{Argv: argv}}
				pub = pub || c.ID == "pub"
			} else {
				steps = []Step{{Remove: c.Paths[0]}}
				removes = true
			}
		case cleanup.BuildCache, cleanup.UnusedImages, cleanup.StoppedContainers:
			if target.CLI == "" || !contextRef.MatchString(target.Context) {
				continue
			}
			steps = []Step{{Argv: append([]string{target.CLI, "--context", target.Context}, dockerPrune[c.Kind]...)}}
		default:
			if m, pkg, ok := brewPackage(c, s); ok {
				brew, pkgs = m, append(pkgs, pkg)
				break
			}
			ts, _ := copyTargets(c, s)
			for _, t := range ts {
				argv := versionCommand(VersionUninstall, t)
				switch {
				case argv[0] == "bash" && managerScriptFound(t.Manager, env):
					// A shell function: the plan shows what the script runs.
					steps = append(steps, Step{Argv: argv, Runs: argv[3:], Admin: t.Admin})
				case argv[0] != "bash" && len(sysenv.LookPathAll(argv[0], env)) > 0:
					steps = append(steps, Step{Argv: argv, Admin: t.Admin})
				default:
					steps = append(steps, Step{Remove: t.Root})
					removes = true
				}
				admin = admin || t.Admin
			}
			versions = true
		}
		for _, step := range steps {
			step.Timeout = cleanupTimeout
			p.Steps = append(p.Steps, step)
		}
		p.Changes = append(p.Changes, "− "+titles[i]+" · "+FormatSize(c.Frees()))
		done = append(done, c)
	}
	if len(pkgs) > 0 {
		// First: brew reads its API data from its cache, which a later
		// step may remove, and would download it again.
		steps, _ := packageSteps(PackageUninstall, brew, pkgs)
		for i := range steps {
			steps[i].Timeout = cleanupTimeout
		}
		p.Steps = append(steps, p.Steps...)
		p.Notes = append(p.Notes, Note{Title: locales.CleanupBrewNote(), Text: locales.CleanupBrewNoteText()})
	}
	if len(p.Steps) == 0 {
		return Plan{}, errors.New(locales.CleanupNothing())
	}
	if removes {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupRemoveNote(), Text: locales.CleanupRemoveNoteText()})
	}
	if pub {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupPubNote(), Text: locales.CleanupPubNoteText()})
	}
	if versions {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupVersionsNote(), Text: locales.CleanupVersionsNoteText()})
	}
	if admin {
		p.Notes = append(p.Notes, AdminNote())
	}
	p.Verify = func(ctx context.Context) (string, error) {
		return locales.CleanupFreed(FormatSize(freed(ctx, done, docker))), nil
	}
	return p, nil
}

// managerScripts are the scripts that define the shell functions of
// managers, as the variable that moves them and where they are without
// it, from the home folder.
var managerScripts = map[string][2]string{
	"nvm":    {"NVM_DIR", ".nvm/nvm.sh"},
	"rvm":    {"rvm_path", ".rvm/scripts/rvm"},
	"sdkman": {"SDKMAN_DIR", ".sdkman/bin/sdkman-init.sh"},
}

// managerScriptFound reports that the script of manager's shell function
// is there in env: a manager removed may leave its versions behind.
func managerScriptFound(manager string, env sysenv.Env) bool {
	s, ok := managerScripts[manager]
	if !ok {
		return false
	}
	path := filepath.Join(env.Get("HOME"), s[1])
	if dir := env.Get(s[0]); dir != "" {
		path = filepath.Join(dir, strings.Join(strings.Split(s[1], "/")[1:], "/"))
	}
	if !filepath.IsAbs(path) {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// freed counts again what cands take and returns how much less it is
// than before. What cannot be counted again counts as not freed.
func freed(ctx context.Context, cands []cleanup.Candidate, docker CountDocker) int64 {
	var rows []inventory.DockerUsage
	var dockerErr error
	if docker != nil {
		rows, dockerErr = docker(ctx)
	}
	var n int64
	for _, c := range cands {
		before := c.Frees()
		switch c.Kind {
		case cleanup.BuildCache, cleanup.UnusedImages, cleanup.StoppedContainers:
			if docker == nil || dockerErr != nil {
				continue
			}
			for _, r := range rows {
				if dockerKind(r.Type) == c.Kind {
					n += max(before-r.Reclaimable, 0)
				}
			}
		default:
			var left int64
			for _, path := range c.Paths {
				after, err := disk.Measure(ctx, path)
				switch {
				case errors.Is(err, fs.ErrNotExist):
				case err == nil:
					left += after.Bytes - after.Shared
				default:
					left = before // not counted: not freed
				}
			}
			n += max(before-left, 0)
		}
	}
	return n
}

// dockerKind is the candidate kind of a row of docker system df.
func dockerKind(rowType string) cleanup.Kind {
	switch rowType {
	case "Build Cache":
		return cleanup.BuildCache
	case "Images":
		return cleanup.UnusedImages
	case "Containers":
		return cleanup.StoppedContainers
	}
	return ""
}

// FormatSize says a count of bytes in decimal units, as Finder does:
// "6.4 GB", "512 MB", "3 KB".
func FormatSize(n int64) string {
	switch {
	case n >= 1e9:
		return locales.SizeGb(float64(n) / 1e9)
	case n >= 1e6:
		return locales.SizeMb(int(n / 1e6))
	}
	return locales.SizeKb(int((n + 999) / 1000))
}

// removeAll removes path and all in it for good. Where removing fails,
// what is in it is made writable first, as Go's module cache is
// read-only; symlinks are not followed out of it.
func removeAll(path string) error {
	if !filepath.IsAbs(path) || filepath.Dir(path) == path {
		return errors.New("refusing to remove " + path)
	}
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil || d.Type()&fs.ModeSymlink != 0:
		case d.IsDir():
			_ = os.Chmod(p, 0o700)
		default:
			_ = os.Chmod(p, 0o600)
		}
		return nil
	})
	return os.RemoveAll(path)
}
