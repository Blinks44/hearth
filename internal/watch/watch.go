// Package watch follows the files that change what Hearth shows: the
// project files, which it reads again, and the shells' startup files,
// after which the computer is scanned again.
package watch

import (
	"context"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/internal/projects"
)

// Watcher is a running watch of a file or folder, as mygo.FileWatcher.
type Watcher interface {
	Next(ctx context.Context) ([]mygo.FileEvent, error)
	Close() error
}

// Service keeps the watches. Its methods are safe from any goroutine; the
// callbacks run on the watches' goroutines.
type Service struct {
	// Start starts watching the file or folder at path; main passes
	// mygo.WatchFiles, with a debounce.
	Start func(ctx context.Context, path string) (Watcher, error)
	// Project is given a project read again after its files changed.
	Project func(p projects.Project)
	// Shell is called after a shell startup file changed.
	Shell func()
	// Err is told of a watch that could not start or stopped; nil drops
	// it.
	Err func(err error)

	mu      sync.Mutex
	watches map[string]context.CancelFunc // by kind and path
	wg      sync.WaitGroup
}

// Projects watches the project folders dirs, not the folders in them, and
// stops watching the projects not among them.
func (s *Service) Projects(dirs []string) {
	s.set("project:", dirs, func(ctx context.Context, dir string) {
		s.run(ctx, dir, func(events []mygo.FileEvent) {
			if touches(events) {
				s.Project(projects.Read(dir))
			}
		})
	})
}

// StartupFiles watches the shells' startup files at paths, and stops
// watching the others. A file missing is not watched.
func (s *Service) StartupFiles(paths []string) {
	s.set("shell:", paths, func(ctx context.Context, p string) {
		if _, err := os.Stat(p); err != nil {
			return
		}
		s.run(ctx, p, func([]mygo.FileEvent) { s.Shell() })
	})
}

// Close stops every watch and waits for them.
func (s *Service) Close() {
	s.mu.Lock()
	for _, cancel := range s.watches {
		cancel()
	}
	s.watches = nil
	s.mu.Unlock()
	s.wg.Wait()
}

// set makes the watches of a kind, by its key prefix, the ones of paths:
// it starts the new ones with start and stops the rest.
func (s *Service) set(kind string, paths []string, start func(ctx context.Context, path string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.watches == nil {
		s.watches = map[string]context.CancelFunc{}
	}
	for key, cancel := range s.watches {
		if p, ok := strings.CutPrefix(key, kind); ok && !slices.Contains(paths, p) {
			cancel()
			delete(s.watches, key)
		}
	}
	for _, p := range paths {
		if _, ok := s.watches[kind+p]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		s.watches[kind+p] = cancel
		s.wg.Go(func() { start(ctx, p) })
	}
}

// run watches p until ctx ends or the watch does, giving changed every
// batch of changes.
func (s *Service) run(ctx context.Context, p string, changed func([]mygo.FileEvent)) {
	w, err := s.Start(ctx, p)
	if err != nil {
		s.report(err)
		return
	}
	defer func() { _ = w.Close() }() // ctx has ended, or the watch did
	for {
		events, err := w.Next(ctx)
		if len(events) > 0 && ctx.Err() == nil {
			changed(events)
		}
		if err != nil {
			if ctx.Err() == nil {
				s.report(err)
			}
			return
		}
	}
}

func (s *Service) report(err error) {
	if s.Err != nil {
		s.Err(err)
	}
}

// touches reports that events of a project folder, watched without the
// folders in it, change what projects.Read reads: one of its files, by
// name or by the name it was renamed from, or the folder itself.
func touches(events []mygo.FileEvent) bool {
	names := append(projects.Names(), ".")
	for _, e := range events {
		if slices.Contains(names, e.Path) || slices.Contains(names, e.OldPath) {
			return true
		}
	}
	return false
}
