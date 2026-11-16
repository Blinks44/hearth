// Package watch follows the files that change what Hearth shows: the
// project files, which it reads again, and the shells' startup files,
// after which the computer is scanned again. It also follows the
// containers of the Docker engine a scan found answering.
package watch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
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
	// Lines runs docker events; main passes run.Lines.
	Lines func(ctx context.Context, c run.Cmd, line func(string)) error
	// Containers is called a moment after the Docker engine's containers
	// changed, or after it stopped answering.
	Containers func()
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
// watching the others. Watching their directory also catches files created
// after Hearth starts and files replaced by an editor's atomic save.
func (s *Service) StartupFiles(paths []string) {
	s.set("shell:", paths, func(ctx context.Context, p string) {
		for ctx.Err() == nil {
			dir := filepath.Dir(p)
			for {
				if info, err := os.Stat(dir); err == nil && info.IsDir() {
					break
				}
				parent := filepath.Dir(dir)
				if parent == dir {
					return
				}
				dir = parent
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				s.report(err)
				return
			}
			rel = filepath.ToSlash(rel)
			w, err := s.Start(ctx, dir)
			if err != nil {
				s.report(err)
				return
			}
			restart := false
			for !restart && ctx.Err() == nil {
				events, nextErr := w.Next(ctx)
				for _, e := range events {
					matches := func(path string) bool {
						return path == "." || path == rel || path != "" && strings.HasPrefix(rel, path+"/")
					}
					if ctx.Err() == nil && (matches(e.Path) || matches(e.OldPath)) {
						s.Shell()
						// A missing parent may now exist: follow the closer directory.
						restart = dir != filepath.Dir(p)
						break
					}
				}
				if nextErr != nil {
					_ = w.Close()
					if ctx.Err() == nil {
						s.report(nextErr)
					}
					return
				}
			}
			_ = w.Close()
		}
	})
}

// How long docker events stays quiet before Containers is called, so that
// compose up starting several containers refreshes once; and the first
// wait before following the engine again after the stream ended.
const (
	dockerSettle = 300 * time.Millisecond
	dockerRetry  = 5 * time.Second
)

// Docker follows the containers of the selected local Docker engine snap
// found answering, by docker events in the terminal's environment env;
// with no such engine it stops following. When the stream ends, as the
// engine stops, Containers is called, and the engine followed again after
// a wait that grows while it keeps failing.
func (s *Service) Docker(snap *inventory.Snapshot, env sysenv.Env) {
	var keys []string
	if exe, endpoint := dockerEngine(snap); exe != "" {
		keys = []string{exe + "\n" + endpoint}
	}
	s.set("docker:", keys, func(ctx context.Context, key string) {
		exe, endpoint, _ := strings.Cut(key, "\n")
		cmd := run.Cmd{Args: []string{exe, "--host", endpoint, "events", "--format", "{{.Action}}",
			"--filter", "type=container", "--filter", "event=create", "--filter", "event=start", "--filter", "event=die",
			"--filter", "event=destroy", "--filter", "event=pause", "--filter", "event=unpause", "--filter", "event=rename"},
			Env: append(slices.Clone(env), "DOCKER_CONTEXT=")}
		retry := dockerRetry
		for {
			start := time.Now()
			var settle *time.Timer
			err := s.Lines(ctx, cmd, func(string) {
				if settle == nil {
					settle = time.AfterFunc(dockerSettle, s.Containers)
				} else {
					settle.Reset(dockerSettle)
				}
			})
			if settle != nil {
				settle.Stop()
			}
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				s.report(err)
			}
			s.Containers()
			if time.Since(start) > retry {
				retry = dockerRetry
			} else {
				retry = min(2*retry, 5*time.Minute)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(retry):
			}
		}
	})
}

// dockerEngine returns the Docker CLI and the address of the selected
// local engine, as the scan probed them, or "" when it did not answer.
func dockerEngine(snap *inventory.Snapshot) (exe, endpoint string) {
	if snap == nil {
		return "", ""
	}
	st, ok := snap.Tool("docker")
	if !ok || st.Docker == nil {
		return "", ""
	}
	active := inventory.TerminalCopy(st.Installations)
	if active < 0 {
		return "", ""
	}
	for _, c := range st.Docker.Contexts {
		if c.Name == st.Docker.Selected && c.Local && c.Status == "available" {
			return st.Installations[active].Path, c.Endpoint
		}
	}
	return "", ""
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
