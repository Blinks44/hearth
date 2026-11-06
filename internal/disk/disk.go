// Package disk counts the space folders take on disk, apart from the scan:
// a walk reads every file, which takes long on big caches.
package disk

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// fileID tells a file apart on its volume, for files with hard links.
type fileID struct{ dev, ino uint64 }

// Measure counts the space root takes: the files and folders in it,
// symlinks not followed past root itself, and a file with hard links once.
// Entries it cannot read make the result Partial, not an error.
func Measure(ctx context.Context, root string) (inventory.Usage, error) {
	var u inventory.Usage
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return u, fmt.Errorf("measure %s: %w", root, err)
	}
	seen := map[fileID]bool{}
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
		size, id, linked := stat(path, info)
		if linked {
			if seen[id] {
				return nil
			}
			seen[id] = true
		}
		u.Bytes += size
		return nil
	})
	if err != nil {
		return inventory.Usage{}, fmt.Errorf("measure %s: %w", root, err)
	}
	return u, nil
}

// Count counts what snap's copies, packages and caches take, one path
// after another, telling progress each path as it is counted, and then
// asks Docker. A path that cannot be counted is left out. When ctx ends
// it stops, with the error.
func Count(ctx context.Context, r run.Runner, env sysenv.Env, snap *inventory.Snapshot, progress func(path string, u inventory.Usage)) (inventory.Sizes, error) {
	s := inventory.Sizes{Paths: map[string]inventory.Usage{}, Caches: Caches(env, snap)}
	for _, path := range Targets(snap, s.Caches) {
		u, err := Measure(ctx, path)
		if ctx.Err() != nil {
			return s, ctx.Err()
		}
		if err != nil {
			continue
		}
		s.Paths[path] = u
		progress(path, u)
	}
	var err error
	if s.Docker, err = Docker(ctx, r, env, snap); err != nil && !errors.Is(err, ErrNoEngine) {
		s.DockerErr = err.Error()
	}
	s.Taken = time.Now()
	return s, ctx.Err()
}
