// Package mcpserver exposes a read-only projection of Hearth through MCP.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/scan"
	"github.com/Blinks44/hearth/internal/store"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// State is private to the server; only explicitly selected fields reach clients.
type State struct {
	Snapshot                    inventory.Snapshot
	Projects                    []projects.Project
	Cached, TerminalUnavailable bool
}

// Source provides one immutable snapshot per request. Refresh is always explicit.
type Source interface {
	Get(context.Context, bool) (State, error)
}

// Reader serializes loads and scans, while allowing a waiting request to cancel.
// It never saves data or changes the user's settings.
type Reader struct {
	CacheDir, SettingsDir string
	Scan                  func(context.Context) (inventory.Snapshot, bool, error)
	once                  sync.Once
	gate                  chan struct{}
	state                 *State
}

func (r *Reader) Get(ctx context.Context, refresh bool) (State, error) {
	r.once.Do(func() { r.gate = make(chan struct{}, 1) })
	select {
	case r.gate <- struct{}{}:
		defer func() { <-r.gate }()
	case <-ctx.Done():
		return State{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	if r.state != nil && !refresh {
		return *r.state, nil
	}
	var st State
	var err error
	if !refresh {
		st.Snapshot, err = store.LoadSnapshot(r.CacheDir)
	}
	if refresh || err != nil || st.Snapshot.Taken.IsZero() {
		scan := r.Scan
		if scan == nil {
			scan = scanSystem
		}
		st.Snapshot, st.TerminalUnavailable, err = scan(ctx)
		if err != nil {
			return State{}, fmt.Errorf("scan environment: %w", err)
		}
	} else {
		st.Cached = true
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	st.Projects, err = readProjects(r.SettingsDir, sysenv.GUI())
	if err != nil {
		return State{}, err
	}
	r.state = &st
	return st, nil
}

// DefaultReader uses the same platform directories as the desktop app, without
// calling mygo.App.Path, which would create directories even in read-only mode.
func DefaultReader(appName string) (*Reader, error) {
	if appName == "" {
		appName = "Hearth"
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate settings: %w", err)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("locate snapshot: %w", err)
	}
	return &Reader{CacheDir: filepath.Join(cache, appName), SettingsDir: filepath.Join(config, appName)}, nil
}

func readProjects(dir string, env sysenv.Env) ([]projects.Project, error) {
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read project list: %w", err)
	}
	// Do not use LoadSettings: it renames invalid settings to a .bad copy.
	var cfg struct {
		Format   int             `json:"format"`
		Projects []store.Project `json:"projects"`
	}
	if err = json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("read project list: %w", err)
	}
	if cfg.Format > 1 {
		return nil, errors.New("settings format is newer than this server")
	}
	var out []projects.Project
	for _, p := range cfg.Projects {
		if filepath.IsAbs(p.Path) {
			out = append(out, projects.ReadWithEnv(p.Path, env))
		}
	}
	return out, nil
}

func scanSystem(ctx context.Context) (inventory.Snapshot, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	runner := run.Exec{}
	term, err := sysenv.Terminal(ctx, runner)
	unavailable := err != nil
	if unavailable {
		term = sysenv.GUI()
	}
	snap := scan.Run(ctx, runner, term, sysenv.GUI(), scan.Detectors)
	return snap, unavailable, ctx.Err()
}
