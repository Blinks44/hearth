package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

var usageCacheFileMu sync.Mutex

const usageCacheVersion = 2

// cachedDB is the sessions parsed from a SQLite database while both the
// main file and its -wal sidecar stay unchanged.
type cachedDB struct {
	size     int64
	mod      int64
	walSize  int64
	walMod   int64
	sessions []inventory.AgentSession
}

type usageCacheFile struct {
	Version int                       `json:"version"`
	Files   map[string]cachedFileJSON `json:"files,omitempty"`
	DBs     map[string]cachedDBJSON   `json:"dbs,omitempty"`
}

type cachedFileJSON struct {
	Size  int64                  `json:"size"`
	Mod   int64                  `json:"mod"`
	ID    string                 `json:"id,omitempty"`
	Dir   string                 `json:"dir,omitempty"`
	Usage []inventory.TokenUsage `json:"usage,omitempty"`
}

type cachedDBJSON struct {
	Size     int64                    `json:"size"`
	Mod      int64                    `json:"mod"`
	WalSize  int64                    `json:"walSize,omitempty"`
	WalMod   int64                    `json:"walMod,omitempty"`
	Sessions []inventory.AgentSession `json:"sessions,omitempty"`
}

// LoadUsageCache loads previously parsed session transcripts and SQLite
// summaries from path into the in-memory cache. A missing or incompatible
// file is ignored.
func LoadUsageCache(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read usage cache: %w", err)
	}
	var f usageCacheFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse usage cache: %w", err)
	}
	if f.Version != usageCacheVersion {
		return nil
	}
	transcripts.Lock()
	defer transcripts.Unlock()
	for p, c := range f.Files {
		if _, exists := transcripts.files[p]; !exists {
			transcripts.files[p] = cached{
				size: c.Size,
				mod:  time.Unix(0, c.Mod),
				t:    transcript{id: c.ID, dir: c.Dir, usage: c.Usage},
			}
		}
	}
	for p, c := range f.DBs {
		if _, exists := transcripts.dbs[p]; !exists {
			transcripts.dbs[p] = cachedDB{
				size:     c.Size,
				mod:      c.Mod,
				walSize:  c.WalSize,
				walMod:   c.WalMod,
				sessions: c.Sessions,
			}
		}
	}
	return nil
}

// SaveUsageCache writes the in-memory parsed session cache atomically to
// path, pruning entries whose source files no longer exist on disk.
func SaveUsageCache(path string) error {
	if path == "" {
		return nil
	}
	transcripts.Lock()
	if !transcripts.dirty {
		transcripts.Unlock()
		return nil
	}
	gen := transcripts.gen
	filesSnap := maps.Clone(transcripts.files)
	dbsSnap := maps.Clone(transcripts.dbs)
	transcripts.dirty = false
	transcripts.Unlock()

	out := usageCacheFile{
		Version: usageCacheVersion,
		Files:   make(map[string]cachedFileJSON, len(filesSnap)),
		DBs:     make(map[string]cachedDBJSON, len(dbsSnap)),
	}
	var missingFiles, missingDBs []string
	for p, c := range filesSnap {
		if _, err := os.Stat(p); err != nil {
			missingFiles = append(missingFiles, p)
			continue
		}
		out.Files[p] = cachedFileJSON{
			Size:  c.size,
			Mod:   c.mod.UnixNano(),
			ID:    c.t.id,
			Dir:   c.t.dir,
			Usage: c.t.usage,
		}
	}
	for p, c := range dbsSnap {
		if _, err := os.Stat(p); err != nil {
			missingDBs = append(missingDBs, p)
			continue
		}
		out.DBs[p] = cachedDBJSON{
			Size:     c.size,
			Mod:      c.mod,
			WalSize:  c.walSize,
			WalMod:   c.walMod,
			Sessions: c.sessions,
		}
	}
	if len(missingFiles) > 0 || len(missingDBs) > 0 {
		transcripts.Lock()
		if transcripts.gen == gen {
			for _, p := range missingFiles {
				delete(transcripts.files, p)
			}
			for _, p := range missingDBs {
				delete(transcripts.dbs, p)
			}
		}
		transcripts.Unlock()
	}

	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal usage cache: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir usage cache: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".usage-cache-*.tmp")
	if err != nil {
		return fmt.Errorf("temp usage cache: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write usage cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close usage cache: %w", err)
	}

	usageCacheFileMu.Lock()
	defer usageCacheFileMu.Unlock()
	transcripts.Lock()
	stale := transcripts.gen != gen
	transcripts.Unlock()
	if stale {
		_ = os.Remove(tmpName)
		return nil
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename usage cache: %w", err)
	}
	return nil
}

// ClearUsageCache empties the in-memory session cache and deletes path if
// non-empty.
func ClearUsageCache(path string) error {
	usageCacheFileMu.Lock()
	defer usageCacheFileMu.Unlock()
	transcripts.Lock()
	transcripts.gen++
	clear(transcripts.files)
	clear(transcripts.dbs)
	transcripts.dirty = false
	transcripts.Unlock()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove usage cache: %w", err)
	}
	return nil
}

// dbStat returns the size and mtime of a SQLite file and its -wal sidecar.
func dbStat(path string) (size, mod, walSize, walMod int64, ok bool) {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return 0, 0, 0, 0, false
	}
	size, mod = fi.Size(), fi.ModTime().UnixNano()
	if wfi, err := os.Stat(path + "-wal"); err == nil && !wfi.IsDir() {
		walSize, walMod = wfi.Size(), wfi.ModTime().UnixNano()
	}
	return size, mod, walSize, walMod, true
}

// readDB parses sessions from a SQLite file at path, or reuses cached
// sessions while the database and its -wal sidecar remain unchanged.
func readDB(path string, parse func(string) ([]inventory.AgentSession, error)) ([]inventory.AgentSession, error) {
	size, mod, walSize, walMod, ok := dbStat(path)
	if !ok {
		return nil, os.ErrNotExist
	}
	transcripts.Lock()
	c, hit := transcripts.dbs[path]
	transcripts.Unlock()
	if hit && c.size == size && c.mod == mod && c.walSize == walSize && c.walMod == walMod {
		return cloneSessions(c.sessions), nil
	}
	sessions, err := parse(path)
	if err != nil {
		return nil, err
	}
	transcripts.Lock()
	transcripts.dbs[path] = cachedDB{
		size:     size,
		mod:      mod,
		walSize:  walSize,
		walMod:   walMod,
		sessions: cloneSessions(sessions),
	}
	transcripts.dirty = true
	transcripts.Unlock()
	return sessions, nil
}

func cloneSessions(in []inventory.AgentSession) []inventory.AgentSession {
	if len(in) == 0 {
		return nil
	}
	out := make([]inventory.AgentSession, len(in))
	for i, s := range in {
		if len(s.Files) > 0 {
			s.Files = append([]string(nil), s.Files...)
		}
		if len(s.Usage) > 0 {
			s.Usage = append([]inventory.TokenUsage(nil), s.Usage...)
		}
		out[i] = s
	}
	return out
}
