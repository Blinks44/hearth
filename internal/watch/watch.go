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
	// Env returns the terminal environment used when re-reading projects.
	Env func() sysenv.Env

	mu             sync.Mutex
	closed         bool
	watches        map[string]context.CancelFunc // by kind and path
	startupPaths   []string
	startupTargets map[string][]string // watched dir -> full startup file paths
	wg             sync.WaitGroup
}

// Projects watches the project folders dirs, not the folders in them, and
// stops watching the projects not among them.
func (s *Service) Projects(dirs []string) {
	s.set("project:", dirs, func(ctx context.Context, dir string) {
		s.run(ctx, dir, func(events []mygo.FileEvent) {
			if touches(events) {
				var env sysenv.Env
				if s.Env != nil {
					env = slices.Clone(s.Env())
				}
				s.Project(projects.ReadWithEnv(dir, env))
			}
		})
	})
}

// StartupFiles watches the shells' startup files at paths, and stops
// watching the others. Watching their directory also catches files created
// after Hearth starts and files replaced by an editor's atomic save.
// Paths sharing the same existing parent directory share a single watcher.
func (s *Service) StartupFiles(paths []string) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.startupPaths = slices.Clone(paths)
	s.mu.Unlock()
	s.syncStartupFiles()
}

func (s *Service) syncStartupFiles() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	paths := slices.Clone(s.startupPaths)
	s.mu.Unlock()

	targets := make(map[string][]string)
	for _, p := range paths {
		dir := existingWatchDir(p)
		if dir == "" {
			continue
		}
		if !slices.Contains(targets[dir], p) {
			targets[dir] = append(targets[dir], p)
		}
	}
	dirs := make([]string, 0, len(targets))
	for dir := range targets {
		dirs = append(dirs, dir)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !slices.Equal(s.startupPaths, paths) {
		return
	}
	s.startupTargets = targets
	s.setLocked("shell:", dirs, s.watchStartupDir)
}

func (s *Service) watchStartupDir(ctx context.Context, dir string) {
	s.run(ctx, dir, func(events []mygo.FileEvent) {
		s.mu.Lock()
		targets := slices.Clone(s.startupTargets[dir])
		s.mu.Unlock()

		matched := false
		resync := false
		for _, p := range targets {
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				s.report(err)
				continue
			}
			rel = filepath.ToSlash(rel)
			for _, e := range events {
				if matchesStartupRel(e.Path, rel) || matchesStartupRel(e.OldPath, rel) {
					matched = true
					// A missing parent may now exist: follow the closer directory.
					if dir != filepath.Dir(p) && existingWatchDir(p) != dir {
						resync = true
					}
					break
				}
			}
		}
		if matched && ctx.Err() == nil {
			s.Shell()
		}
		if resync {
			s.syncStartupFiles()
		}
	})
}

func existingWatchDir(p string) string {
	dir := filepath.Dir(p)
	for {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func matchesStartupRel(eventPath, rel string) bool {
	return eventPath == "." || eventPath == rel || eventPath != "" && strings.HasPrefix(rel, eventPath+"/")
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
	s.closed = true
	for _, cancel := range s.watches {
		cancel()
	}
	s.watches = nil
	s.startupPaths = nil
	s.startupTargets = nil
	s.mu.Unlock()
	s.wg.Wait()
}

// set makes the watches of a kind, by its key prefix, the ones of paths:
// it starts the new ones with start and stops the rest.
func (s *Service) set(kind string, paths []string, start func(ctx context.Context, path string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.setLocked(kind, paths, start)
}

func (s *Service) setLocked(kind string, paths []string, start func(ctx context.Context, path string)) {
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
