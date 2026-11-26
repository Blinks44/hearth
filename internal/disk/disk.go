// Package disk counts the space folders take on disk, apart from the scan:
// a walk reads every file, which takes long on big caches.
package disk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// fileID tells a file apart on its volume, for files with hard links.
type fileID struct{ dev, ino uint64 }

// Measure counts the space root takes: the files and folders in it,
// symlinks not followed past root itself, and a file with hard links once;
// one that links outside root also lead to is Shared. Entries it cannot
// read make the result Partial, not an error.
func Measure(ctx context.Context, root string) (inventory.Usage, error) {
	var u inventory.Usage
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return u, fmt.Errorf("measure %s: %w", root, err)
	}
	// linked are the files with hard links, by identity: their size, links
	// in all, and links found under root.
	type links struct {
		size         int64
		total, found int
	}
	linked := map[fileID]*links{}
	err = filepath.WalkDir(real, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if path == real && d == nil {
				return err
			}
			// A folder that cannot be listed: its own entry is counted.
			u.Partial = true
			return nil
		}
		info, err := d.Info()
		if err != nil {
			u.Partial = true
			return nil
		}
		size, id, n := stat(path, info)
		if n > 1 {
			if l, ok := linked[id]; ok {
				l.found++
				return nil
			}
			linked[id] = &links{size: size, total: n, found: 1}
		}
		u.Bytes += size
		return nil
	})
	for _, l := range linked {
		if l.found < l.total {
			u.Shared += l.size
		}
	}
	if err != nil {
		return inventory.Usage{}, fmt.Errorf("measure %s: %w", root, err)
	}
	return u, nil
}

// Count publishes independent snapshots as caches, Docker, tools and project
// dependencies are counted. Docker usage runs concurrently with cache walks so
// slow filesystem caches never delay the Docker page. Project discovery runs
// last, so a home-directory walk never delays cleanup results. Cancellation
// prevents further progress and returns the partial result.
func Count(ctx context.Context, r run.Runner, env sysenv.Env, snap *inventory.Snapshot, installs []string, progress func(inventory.Sizes)) (inventory.Sizes, error) {
	return CountWithExclude(ctx, r, env, snap, installs, nil, progress)
}

// CountWithExclude is Count with extra directories skipped when scanning home
// for project dependency folders.
func CountWithExclude(ctx context.Context, r run.Runner, env sysenv.Env, snap *inventory.Snapshot, installs, exclude []string, progress func(inventory.Sizes)) (inventory.Sizes, error) {
	var mu, progressMu sync.Mutex
	s := inventory.Sizes{Paths: map[string]inventory.Usage{}, Caches: activeCaches(ctx, r, env, snap)}
	cloneLocked := func() inventory.Sizes {
		next := s
		next.Paths = maps.Clone(s.Paths)
		next.Docker = slices.Clone(s.Docker)
		next.Installs = slices.Clone(s.Installs)
		return next
	}
	publish := func() {
		if ctx.Err() != nil || progress == nil {
			return
		}
		progressMu.Lock()
		defer progressMu.Unlock()
		if ctx.Err() != nil {
			return
		}
		mu.Lock()
		next := cloneLocked()
		mu.Unlock()
		progress(next)
	}
	publish()

	var dockerWG sync.WaitGroup
	dockerWG.Go(func() {
		rows, derr := Docker(ctx, r, env, snap)
		if ctx.Err() != nil {
			return
		}
		changed := false
		mu.Lock()
		if derr != nil && !errors.Is(derr, ErrNoEngine) {
			s.DockerErr = derr.Error()
			changed = true
		} else if len(rows) > 0 {
			s.Docker = rows
			changed = true
		}
		mu.Unlock()
		if changed {
			publish()
		}
	})
	defer dockerWG.Wait()

	seen := map[string]bool{}
	measure := func(path string) {
		if seen[path] || ctx.Err() != nil {
			return
		}
		seen[path] = true
		u, err := Measure(ctx, path)
		if err == nil {
			mu.Lock()
			s.Paths[path] = u
			mu.Unlock()
		}
		publish()
	}
	for _, cache := range s.Caches {
		for _, path := range cache.AllPaths() {
			measure(path)
		}
	}
	if ctx.Err() != nil {
		dockerWG.Wait()
		mu.Lock()
		defer mu.Unlock()
		return cloneLocked(), ctx.Err()
	}
	for _, path := range append(slices.Clone(installs), Targets(snap, nil)...) {
		measure(path)
	}
	if ctx.Err() != nil {
		dockerWG.Wait()
		mu.Lock()
		defer mu.Unlock()
		return cloneLocked(), ctx.Err()
	}
	home := sources.ManagerDirs(env).Home
	if home != "" {
		foundInstalls, err := projects.InstallsWithExclude(ctx, home, exclude)
		if err != nil {
			dockerWG.Wait()
			mu.Lock()
			defer mu.Unlock()
			return cloneLocked(), err
		}
		mu.Lock()
		s.Installs = foundInstalls
		mu.Unlock()
		publish()
		for _, in := range foundInstalls {
			measure(filepath.Join(in.Project, in.Name))
		}
	}
	dockerWG.Wait()
	mu.Lock()
	defer mu.Unlock()
	if ctx.Err() != nil {
		return cloneLocked(), ctx.Err()
	}
	s.Taken = time.Now()
	return cloneLocked(), nil
}

// activeCaches resolves pnpm's configured store through pnpm itself.
// When the active store lives under pnpm's default store root (e.g.
// ~/Library/pnpm/store/v10), the parent root is kept so older store versions
// are cleaned too; otherwise the custom path is used.
func activeCaches(ctx context.Context, r run.Runner, env sysenv.Env, snap *inventory.Snapshot) []inventory.Cache {
	caches := Caches(env, snap)
	defaultPath := ""
	for _, c := range caches {
		if c.ID == "pnpm" {
			defaultPath = c.Path
			break
		}
	}
	paths := sysenv.LookPathAll("pnpm", env)
	if len(paths) == 0 {
		return caches
	}
	caches = slices.DeleteFunc(caches, func(c inventory.Cache) bool { return c.ID == "pnpm" })
	result, err := r.Run(ctx, run.Cmd{Args: []string{paths[0], "store", "path"}, Env: env})
	if err != nil || result.ExitCode != 0 || result.Truncated {
		return caches
	}
	path := strings.TrimSpace(string(result.Stdout))
	if filepath.IsAbs(path) && !strings.ContainsAny(path, "\r\n") && isDir(path) {
		if defaultPath != "" && withinDir(path, defaultPath) {
			path = defaultPath
		}
		caches = append(caches, inventory.Cache{ID: "pnpm", Path: path})
	}
	return caches
}

func withinDir(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
