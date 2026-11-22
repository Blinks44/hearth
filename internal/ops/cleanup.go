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
// Yarn Berry's cache clean works only inside a project; pnpm's store is
// removed as a folder because pnpm store prune leaves packages linked by
// older projects, while existing node_modules keep working via hard links
// or clones and pnpm re-downloads on the next install.
var cacheCommands = map[string][]string{
	"npm":       {"npm", "cache", "clean", "--force"},
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
// Homebrew's packages nothing needs; projects' installs, extensions' old
// versions, agents' old sessions, LM Studio's models; Ollama's when the
// terminal has ollama.
func Cleans(c cleanup.Candidate, s *inventory.Snapshot) bool {
	switch c.Kind {
	case cleanup.Cache, cleanup.BuildCache, cleanup.UnusedImages, cleanup.StoppedContainers,
		cleanup.ProjectInstalls, cleanup.OldExtensions, cleanup.AgentHistory:
		return true
	case cleanup.Model:
		st, _ := s.Tool("models")
		return c.Manager == "LM Studio" || c.Manager == "Ollama" && st.Models != nil && st.Models.Ollama != nil && st.Models.Ollama.Path != ""
	case cleanup.ExtraCopy, cleanup.UnneededPackage:
		if _, _, ok := cleanupPackage(c, s); ok {
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

// cleanupPackage is the system package manager and its package c is:
// a Homebrew copy or unneeded formula, or an orphaned APT/Pacman package.
// brew uninstall removes every installed version of a formula, so one
// with another version is not offered.
func cleanupPackage(c cleanup.Candidate, s *inventory.Snapshot) (inventory.PackageManager, inventory.SystemPackage, bool) {
	if s == nil {
		return inventory.PackageManager{}, inventory.SystemPackage{}, false
	}
	switch {
	case c.Kind == cleanup.ExtraCopy && len(c.Paths) == 1 && len(c.Copies) > 0 && c.Copies[0].Source.Manager == "brew":
	case c.Kind == cleanup.UnneededPackage && c.Manager == "brew" && len(c.Paths) > 0:
	case c.Kind == cleanup.UnneededPackage && (c.Manager == "apt" || c.Manager == "pacman"):
		wantName, wantArch, hasArch := strings.Cut(c.ID, ":")
		for _, st := range s.Tools {
			for _, m := range st.PackageManagers {
				if m.ID != c.Manager {
					continue
				}
				i := slices.IndexFunc(m.Packages, func(p inventory.SystemPackage) bool {
					return p.Orphan && p.Name == wantName && (!hasArch || p.Architecture == wantArch)
				})
				if i < 0 {
					continue
				}
				pkg := m.Packages[i]
				if m.ID == "apt" && hasArch {
					pkg.Reference = c.ID
				}
				if !packageName.MatchString(packageRef(m.ID, pkg)) || slices.Contains(managerPackages[m.ID], strings.ToLower(pkg.Name)) {
					return inventory.PackageManager{}, inventory.SystemPackage{}, false
				}
				return m, pkg, true
			}
		}
		return inventory.PackageManager{}, inventory.SystemPackage{}, false
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
// uninstall, first; APT and Pacman orphans through dpkg --remove and
// pacman -R; Ollama's models through ollama rm; projects' installs,
// extensions' old versions and LM Studio's models as their folders.
// Candidates it does not remove are left out. Its check counts again
// what they take and says what was freed; docker, nil without an
// engine, asks Docker.
func CleanupPlan(cands []cleanup.Candidate, _ []string, s *inventory.Snapshot, env sysenv.Env, target DockerTarget, docker CountDocker) (Plan, error) {
	p := Plan{Title: locales.CleanupPlanTitle(), Why: locales.CleanupPlanWhy()}
	var done []cleanup.Candidate
	var brew inventory.PackageManager
	var brewPkgs []inventory.SystemPackage
	var otherManagers []inventory.PackageManager
	otherPkgs := map[string][]inventory.SystemPackage{}
	removes, pub, versions, admin, installs, models, history, pnpm := false, false, false, false, false, false, false, false
	for _, c := range cands {
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
				for _, path := range c.Paths {
					steps = append(steps, Step{Remove: path})
				}
				removes = true
			}
			pnpm = pnpm || c.ID == "pnpm"
		case cleanup.BuildCache, cleanup.UnusedImages, cleanup.StoppedContainers:
			if target.CLI == "" || !contextRef.MatchString(target.Context) {
				continue
			}
			steps = []Step{{Argv: append([]string{target.CLI, "--context", target.Context}, dockerPrune[c.Kind]...)}}
		case cleanup.ProjectInstalls, cleanup.OldExtensions, cleanup.AgentHistory:
			for _, path := range c.Paths {
				steps = append(steps, Step{Remove: path})
			}
			// The note on folders says what goes comes back: not so for
			// sessions, which have a note of their own.
			removes = removes || c.Kind != cleanup.AgentHistory
			installs = installs || c.Kind == cleanup.ProjectInstalls
			history = history || c.Kind == cleanup.AgentHistory
		case cleanup.Model:
			if c.Manager == "Ollama" {
				steps = []Step{{Argv: []string{"ollama", "rm", c.ID}}}
			} else {
				steps, removes = []Step{{Remove: c.Paths[0]}}, true
			}
			models = true
		default:
			if m, pkg, ok := cleanupPackage(c, s); ok {
				if m.ID == "brew" {
					brew, brewPkgs = m, append(brewPkgs, pkg)
				} else {
					key := m.ID + "\x00" + m.Path
					if _, seen := otherPkgs[key]; !seen {
						otherManagers = append(otherManagers, m)
					}
					otherPkgs[key] = append(otherPkgs[key], pkg)
				}
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
		done = append(done, c)
	}
	if len(brewPkgs) > 0 {
		// First: brew reads its API data from its cache, which a later
		// step may remove, and would download it again.
		steps, _ := packageSteps(PackageUninstall, brew, brewPkgs)
		for i := range steps {
			steps[i].Timeout = cleanupTimeout
		}
		p.Steps = append(steps, p.Steps...)
		p.Notes = append(p.Notes, Note{Title: locales.CleanupBrewNote(), Text: locales.CleanupBrewNoteText()})
	}
	for _, m := range otherManagers {
		key := m.ID + "\x00" + m.Path
		steps, notes := packageSteps(PackageUninstall, m, otherPkgs[key])
		for i := range steps {
			steps[i].Timeout = cleanupTimeout
			admin = admin || steps[i].Admin
		}
		p.Steps = append(p.Steps, steps...)
		p.Notes = append(p.Notes, notes...)
	}
	if len(p.Steps) == 0 {
		return Plan{}, errors.New(locales.CleanupNothing())
	}
	if removes {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupRemoveNote(), Text: locales.CleanupRemoveNoteText()})
	}
	if pnpm {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupPnpmNote(), Text: locales.CleanupPnpmNoteText()})
	}
	if pub {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupPubNote(), Text: locales.CleanupPubNoteText()})
	}
	if installs {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupInstallsNote(), Text: locales.CleanupInstallsNoteText()})
	}
	if models {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupModelsNote(), Text: locales.CleanupModelsNoteText()})
	}
	if history {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupHistoryNote(), Text: locales.CleanupHistoryNoteText()})
	}
	if versions {
		p.Notes = append(p.Notes, Note{Title: locales.CleanupVersionsNote(), Text: locales.CleanupVersionsNoteText()})
	}
	if admin {
		p.Notes = append(p.Notes, AdminNote())
	}
	p.Verify = func(ctx context.Context) (string, error) {
		n := freed(ctx, done, docker)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if n == 0 {
			return locales.CleanupFreedNone(), nil
		}
		return locales.CleanupFreed(FormatSize(n)), nil
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
		if c.Kind == cleanup.Model && c.Manager == "Ollama" {
			// ollama rm succeeded, as the check runs only then; the Linux
			// service's folder cannot be counted again.
			n += before
			continue
		}
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
