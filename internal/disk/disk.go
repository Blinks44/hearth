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
// dependencies are counted. Project discovery runs last, so a home-directory
// walk never delays the cleanup results. Missing paths disappear from the new
// count. Cancellation prevents further progress and returns the partial result.
func Count(ctx context.Context, r run.Runner, env sysenv.Env, snap *inventory.Snapshot, installs []string, progress func(inventory.Sizes)) (inventory.Sizes, error) {
	s := inventory.Sizes{Paths: map[string]inventory.Usage{}, Caches: activeCaches(ctx, r, env, snap)}
	publish := func() {
		if ctx.Err() == nil {
			next := s
			next.Paths = maps.Clone(s.Paths)
			next.Installs = slices.Clone(s.Installs)
			progress(next)
		}
	}
	publish()
	seen := map[string]bool{}
	measure := func(path string) {
		if seen[path] || ctx.Err() != nil {
			return
		}
		seen[path] = true
		u, err := Measure(ctx, path)
		if err == nil {
			s.Paths[path] = u
		}
		publish()
	}
	for _, cache := range s.Caches {
		measure(cache.Path)
	}
	if ctx.Err() != nil {
		return s, ctx.Err()
	}
	var err error
	if s.Docker, err = Docker(ctx, r, env, snap); err != nil && !errors.Is(err, ErrNoEngine) {
		s.DockerErr = err.Error()
	}
	publish()
	for _, path := range append(slices.Clone(installs), Targets(snap, nil)...) {
		measure(path)
	}
	if ctx.Err() != nil {
		return s, ctx.Err()
	}
	home := sources.ManagerDirs(env).Home
	if home != "" {
		s.Installs, err = projects.NodeModules(ctx, home)
		if err != nil {
			return s, err
		}
		publish()
		for _, in := range s.Installs {
			measure(filepath.Join(in.Project, in.Name))
		}
	}
	if ctx.Err() != nil {
		return s, ctx.Err()
	}
	s.Taken = time.Now()
	return s, nil
}

// activeCaches resolves pnpm's configured, versioned store through pnpm itself.
// Its parent may contain other store versions that this pnpm cannot prune.
// When resolution fails, omit pnpm rather than offering a different directory.
func activeCaches(ctx context.Context, r run.Runner, env sysenv.Env, snap *inventory.Snapshot) []inventory.Cache {
	caches := Caches(env, snap)
	caches = slices.DeleteFunc(caches, func(c inventory.Cache) bool { return c.ID == "pnpm" })
	paths := sysenv.LookPathAll("pnpm", env)
	if len(paths) == 0 {
		return caches
	}
	result, err := r.Run(ctx, run.Cmd{Args: []string{paths[0], "store", "path"}, Env: env})
	if err != nil || result.ExitCode != 0 || result.Truncated {
		return caches
	}
	path := strings.TrimSpace(string(result.Stdout))
	if filepath.IsAbs(path) && !strings.ContainsAny(path, "\r\n") && isDir(path) {
		caches = append(caches, inventory.Cache{ID: "pnpm", Path: path})
	}
	return caches
}
