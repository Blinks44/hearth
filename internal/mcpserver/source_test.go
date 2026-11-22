package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/store"
)

func TestReader(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cached bool
	}{{"existing cache", true}, {"no cache", false}} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			cache := filepath.Join(base, "cache")
			settings := filepath.Join(base, "settings")
			if tc.cached {
				if err := os.MkdirAll(cache, 0755); err != nil {
					t.Fatal(err)
				}
				if err := store.SaveSnapshot(cache, fixture().Snapshot); err != nil {
					t.Fatal(err)
				}
			}
			scans := 0
			r := &Reader{CacheDir: cache, SettingsDir: settings, Scan: func(context.Context) (inventory.Snapshot, bool, error) {
				scans++
				return fixture().Snapshot, false, nil
			}}
			st, err := r.Get(t.Context(), false)
			if err != nil || st.Cached != tc.cached {
				t.Fatalf("state %+v err %v", st, err)
			}
			if _, err = r.Get(t.Context(), false); err != nil {
				t.Fatal(err)
			}
			initial := 0
			if !tc.cached {
				initial = 1
			}
			if scans != initial {
				t.Fatalf("unexpected scans %d", scans)
			}
			st, err = r.Get(t.Context(), true)
			if err != nil || st.Cached || scans != initial+1 {
				t.Fatalf("refresh %+v %v scans %d", st, err, scans)
			}
			if _, err := os.Stat(settings); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("created settings directory")
			}
			if !tc.cached {
				if _, err := os.Stat(cache); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("created snapshot directory")
				}
			}
		})
	}
}

func TestInvalidSettingsStayUnchanged(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(file, []byte("broken file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readProjects(dir, nil); err == nil {
		t.Fatal("invalid settings accepted")
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "broken file" {
		t.Fatal("settings changed")
	}
	if _, err := os.Stat(file + ".bad"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("renamed settings")
	}
}

func TestReaderCancellation(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	r := &Reader{CacheDir: t.TempDir(), SettingsDir: t.TempDir(), Scan: func(ctx context.Context) (inventory.Snapshot, bool, error) {
		close(started)
		select {
		case <-release:
			return fixture().Snapshot, false, nil
		case <-ctx.Done():
			return inventory.Snapshot{}, false, ctx.Err()
		}
	}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := r.Get(ctx, true); done <- err }()
	<-started
	waiting, stop := context.WithTimeout(t.Context(), time.Second)
	stop()
	if _, err := r.Get(waiting, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting request did not cancel: %v", err)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("scan did not cancel: %v", err)
	}
}

func TestReadRegisteredProjects(t *testing.T) {
	base := t.TempDir()
	project := filepath.Join(base, "project")
	if err := os.Mkdir(project, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".nvmrc"), []byte("20\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(map[string]any{"format": 1, "projects": []map[string]string{{"path": project}}})
	if err := os.WriteFile(filepath.Join(base, "settings.json"), cfg, 0644); err != nil {
		t.Fatal(err)
	}
	ps, err := readProjects(base, nil)
	if err != nil || len(ps) != 1 || len(ps[0].Requirements) != 1 || ps[0].Requirements[0].Want != "20" {
		t.Fatalf("projects %+v err %v", ps, err)
	}
}
