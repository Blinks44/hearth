package versions

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Query keeps the validated result of a package-manager query with the same
// lifetime, offline policy and clearing lock as HTTP version lists.
func (c *Cache) Query(ctx context.Context, name string, fetch func(context.Context) ([]byte, error)) ([]byte, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, err
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	age := c.MaxAge
	if age == 0 {
		age = 24 * time.Hour
	}
	path := filepath.Join(c.Dir, name)
	kept, m, readErr := c.read(path)
	if c.offline.Load() {
		if readErr != nil {
			return nil, time.Time{}, ErrOffline
		}
		return kept, m.Fetched, nil
	}
	if readErr == nil && now().Sub(m.Fetched) < age {
		return kept, m.Fetched, nil
	}
	data, err := fetch(ctx)
	if ctx.Err() == context.Canceled {
		return nil, time.Time{}, ctx.Err()
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		if readErr == nil {
			return kept, m.Fetched, fmt.Errorf("using cached package query: %w", err)
		}
		return nil, time.Time{}, err
	}
	if c.offline.Load() {
		return nil, time.Time{}, ErrOffline
	}
	m = meta{Fetched: now()}
	if err := c.write(path, data, m); err != nil {
		return nil, time.Time{}, err
	}
	return data, m.Fetched, nil
}

// Forget removes the kept result of the query name, so that the next
// Query runs it: the packages it lists changed.
func (c *Cache) Forget(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	path := filepath.Join(c.Dir, name)
	for _, p := range []string{path, path + ".meta"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("forget %s: %w", name, err)
		}
	}
	return nil
}
