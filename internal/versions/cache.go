package versions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// maxBody bounds a list read from a server.
const maxBody = 32 << 20

// Cache gets files over HTTP and keeps them in Dir: a copy younger than
// MaxAge is used as it is, an older one is checked with its ETag, and one
// the server cannot be asked about is used as it is too.
type Cache struct {
	Dir string
	// Client makes the requests; nil is a client with a 30 s timeout.
	Client *http.Client
	// MaxAge is how long a copy is used without asking; 0 is a day.
	MaxAge time.Duration
	// Now tells the time; nil is time.Now.
	Now func() time.Time

	offline atomic.Bool  // Get asks no server, see SetOffline
	mu      sync.RWMutex // Clear waits for the Gets in progress
}

// ErrOffline is why Get has no copy to return while it may not ask a
// server.
var ErrOffline = errors.New("version list downloads are turned off")

// meta is what Cache keeps beside a copy.
type meta struct {
	ETag    string
	Fetched time.Time
}

var defaultClient = &http.Client{Timeout: 30 * time.Second}

// Get returns the file at url, kept in Dir as name, and when it was last
// got from the server. Without a kept copy, a failed request is an error;
// with one, the copy comes back and the error is dropped, as an offline
// computer still shows the lists it had.
func (c *Cache) Get(ctx context.Context, name, url string) ([]byte, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, time.Time{}, err
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	maxAge := c.MaxAge
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}
	path := filepath.Join(c.Dir, name)
	kept, m, keptErr := c.read(path)
	if c.offline.Load() {
		if keptErr != nil {
			return nil, time.Time{}, fmt.Errorf("%s: %w", name, ErrOffline)
		}
		return kept, m.Fetched, nil
	}
	if keptErr == nil && now().Sub(m.Fetched) < maxAge {
		return kept, m.Fetched, nil
	}

	body, etag, err := c.fetch(ctx, url, m.ETag)
	switch {
	case err != nil && keptErr == nil:
		return kept, m.Fetched, nil
	case err != nil:
		return nil, time.Time{}, err
	case body == nil: // not modified
		body, etag = kept, m.ETag
	}
	m = meta{ETag: etag, Fetched: now()}
	if err := c.write(path, body, m); err != nil {
		return nil, time.Time{}, err
	}
	return body, m.Fetched, nil
}

// fetch gets url, asking for it only if it no longer has etag. It returns
// a nil body when it has not changed.
func (c *Cache) fetch(ctx context.Context, url, etag string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("get %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "Hearth")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	client := c.Client
	if client == nil {
		client = defaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("get %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }() // the body was read or is not wanted
	switch resp.StatusCode {
	case http.StatusNotModified:
		if etag != "" {
			return nil, etag, nil
		}
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		if err != nil {
			return nil, "", fmt.Errorf("get %s: %w", url, err)
		}
		return body, resp.Header.Get("ETag"), nil
	}
	return nil, "", fmt.Errorf("get %s: %s", url, resp.Status)
}

func (c *Cache) read(path string) ([]byte, meta, error) {
	var m meta
	data, err := os.ReadFile(path + ".meta")
	if err != nil {
		return nil, m, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, m, err
	}
	body, err := os.ReadFile(path)
	return body, m, err
}

// write keeps body and m, each replaced whole, the body first: a crash
// between them leaves the old meta, which only makes the copy look older.
func (c *Cache) write(path string, body []byte, m meta) error {
	data, err := json.Marshal(m)
	if err == nil {
		err = os.MkdirAll(c.Dir, 0o755)
	}
	if err == nil {
		err = replace(path, body)
	}
	if err == nil {
		err = replace(path+".meta", data)
	}
	if err != nil {
		return fmt.Errorf("keep %s: %w", filepath.Base(path), err)
	}
	return nil
}

func replace(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	err = errors.Join(err, f.Close())
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

// Clear removes only the version lists, waiting for active reads to finish.
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := os.RemoveAll(c.Dir); err != nil {
		return fmt.Errorf("clear version cache: %w", err)
	}
	return nil
}

// SetOffline tells whether Get may ask servers; offline, it returns the
// kept copies only, however old.
func (c *Cache) SetOffline(offline bool) { c.offline.Store(offline) }
