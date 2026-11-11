package ops

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

// Cleans reports that a cleanup plan removes candidates of the kind.
func Cleans(k cleanup.Kind) bool {
	switch k {
	case cleanup.Cache, cleanup.BuildCache, cleanup.UnusedImages, cleanup.StoppedContainers:
		return true
	}
	return false
}

// dockerPrune are the docker commands that remove the Docker candidates.
var dockerPrune = map[cleanup.Kind][]string{
	cleanup.BuildCache:        {"builder", "prune", "--force"},
	cleanup.UnusedImages:      {"image", "prune", "--all", "--force"},
	cleanup.StoppedContainers: {"container", "prune", "--force"},
}

// CleanupPlan returns the plan that removes cands, titled as titles says
// each, a step each: a cache through its tool's command when the
// terminal finds it, or as its folder; Docker's through its prune
// commands. Candidates it does not remove are left out. Its check counts
// again what they take and says what was freed; docker, nil without an
// engine, asks Docker.
func CleanupPlan(cands []cleanup.Candidate, titles []string, env sysenv.Env, target DockerTarget, docker CountDocker) (Plan, error) {
	p := Plan{Title: locales.CleanupPlanTitle(), Why: locales.CleanupPlanWhy()}
	var done []cleanup.Candidate
	removes, pub := false, false
	for i, c := range cands {
		if !Cleans(c.Kind) {
			continue
		}
		var step Step
		switch c.Kind {
		case cleanup.Cache:
			argv := CacheCommand(c.ID)
			if len(argv) > 0 && len(sysenv.LookPathAll(argv[0], env)) > 0 {
				step = Step{Argv: argv}
				pub = pub || c.ID == "pub"
			} else {
				step = Step{Remove: c.Paths[0]}
				removes = true
			}
		default:
			if target.CLI == "" || !contextRef.MatchString(target.Context) {
				continue
			}
			step = Step{Argv: append([]string{target.CLI, "--context", target.Context}, dockerPrune[c.Kind]...)}
		}
		step.Timeout = cleanupTimeout
		p.Steps = append(p.Steps, step)
		p.Changes = append(p.Changes, "− "+titles[i]+" · "+FormatSize(c.Frees()))
		done = append(done, c)
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
	p.Verify = func(ctx context.Context) (string, error) {
		return locales.CleanupFreed(FormatSize(freed(ctx, done, docker))), nil
	}
	return p, nil
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
		case cleanup.Cache:
			after, err := disk.Measure(ctx, c.Paths[0])
			switch {
			case errors.Is(err, fs.ErrNotExist):
				n += before
			case err == nil:
				n += max(before-(after.Bytes-after.Shared), 0)
			}
		default:
			if docker == nil || dockerErr != nil {
				continue
			}
			for _, r := range rows {
				if dockerKind(r.Type) == c.Kind {
					n += max(before-r.Reclaimable, 0)
				}
			}
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
