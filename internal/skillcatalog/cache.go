package skillcatalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// DefaultTTL is how long a cached catalog response is reused before re-fetching.
const DefaultTTL = 6 * time.Hour

const cacheVersion = 1

// Cache keeps fetched skill catalogs on disk so reopening the catalog or
// switching tabs does not hit GitHub rate limits.
type Cache struct {
	Dir string
	TTL time.Duration
	Now func() time.Time

	mu  sync.RWMutex
	gen uint64
}

type cacheEntry struct {
	Version int       `json:"version"`
	Fetched time.Time `json:"fetched"`
	Skills  []Skill   `json:"skills"`
}

// Fetch returns cached skills when fresh (unless force is true), otherwise
// fetches from GitHub and updates the disk cache.
func (c *Cache) Fetch(ctx context.Context, client *http.Client, repo Repository, force bool) ([]Skill, error) {
	return c.FetchWith(ctx, client, nil, nil, repo, force)
}

// FetchWith returns cached skills when fresh (unless force is true), otherwise
// fetches from GitHub archive or via local git (GitLab, SSH, private repos) and
// updates the disk cache. When a non-forced refresh fails due to network or
// rate-limit errors, a stale cached entry is returned as fallback.
func (c *Cache) FetchWith(ctx context.Context, client *http.Client, runner run.Runner, env sysenv.Env, repo Repository, force bool) ([]Skill, error) {
	if c == nil || c.Dir == "" {
		return FetchWith(ctx, client, runner, env, repo)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	ttl := c.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	path := c.path(repo)
	c.mu.RLock()
	gen := c.gen
	entry, ok := c.read(path)
	c.mu.RUnlock()
	if !force && ok && now().Sub(entry.Fetched) < ttl {
		return entry.Skills, nil
	}
	skills, err := FetchWith(ctx, client, runner, env, repo)
	if err != nil {
		if !force && ok && ctx.Err() == nil {
			return entry.Skills, nil
		}
		return nil, err
	}
	c.mu.Lock()
	if c.gen == gen {
		_ = c.write(path, cacheEntry{Version: cacheVersion, Fetched: now().UTC(), Skills: skills})
	}
	c.mu.Unlock()
	return skills, nil
}

// Clear removes all cached catalog files.
func (c *Cache) Clear() error {
	if c == nil || c.Dir == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	if err := os.RemoveAll(c.Dir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (c *Cache) path(repo Repository) string {
	sum := sha256.Sum256([]byte(repo.Key()))
	return filepath.Join(c.Dir, hex.EncodeToString(sum[:8])+".json")
}

func (c *Cache) read(path string) (cacheEntry, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return cacheEntry{}, false
	}
	var e cacheEntry
	if json.Unmarshal(data, &e) != nil || e.Version != cacheVersion || len(e.Skills) == 0 {
		return cacheEntry{}, false
	}
	for _, s := range e.Skills {
		if s.Validate() != nil {
			return cacheEntry{}, false
		}
	}
	return e, true
}

func (c *Cache) write(path string, e cacheEntry) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".catalog-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
