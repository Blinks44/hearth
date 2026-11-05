package watch

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/run"
)

// fakeWatcher gives the batches a test sends it.
type fakeWatcher struct {
	events chan []mygo.FileEvent
}

func (w *fakeWatcher) Next(ctx context.Context) ([]mygo.FileEvent, error) {
	select {
	case e := <-w.events:
		return e, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (w *fakeWatcher) Close() error { return nil }

// fakeStart starts fake watches and keeps them by path.
type fakeStart struct {
	mu      sync.Mutex
	started map[string]*fakeWatcher
	starts  int
	ctxs    map[string]context.Context
}

func (f *fakeStart) start(ctx context.Context, path string) (Watcher, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.started == nil {
		f.started, f.ctxs = map[string]*fakeWatcher{}, map[string]context.Context{}
	}
	w := &fakeWatcher{events: make(chan []mygo.FileEvent)}
	f.started[path], f.ctxs[path] = w, ctx
	f.starts++
	return w, nil
}

// watcher waits for the watch of path to start.
func (f *fakeStart) watcher(t *testing.T, path string) (*fakeWatcher, context.Context) {
	t.Helper()
	for range 200 {
		f.mu.Lock()
		w, ctx := f.started[path], f.ctxs[path]
		f.mu.Unlock()
		if w != nil {
			return w, ctx
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no watch of %s", path)
	return nil, nil
}

// A change of .nvmrc reads the project again with the version written; a
// change of another file reads nothing.
func TestProjectChange(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("18\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeStart{}
	read := make(chan projects.Project, 1)
	s := &Service{Start: f.start, Project: func(p projects.Project) { read <- p }}
	defer s.Close()
	s.Projects([]string{dir})
	w, _ := f.watcher(t, dir)

	w.events <- []mygo.FileEvent{{Op: mygo.FileWrite, Path: "README.md"}}
	if err := os.WriteFile(filepath.Join(dir, ".nvmrc"), []byte("20\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w.events <- []mygo.FileEvent{{Op: mygo.FileWrite, Path: ".nvmrc"}}
	select {
	case p := <-read:
		if p.Dir != dir || len(p.Requirements) != 1 || p.Requirements[0].Want != "20" {
			t.Errorf("read %+v, want .nvmrc 20", p)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the project was not read again")
	}
	select {
	case p := <-read:
		t.Errorf("read again for README.md: %+v", p)
	default:
	}
}

// Setting the projects again starts the new watches, keeps the ones still
// wanted and stops the rest.
func TestProjectsSet(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	f := &fakeStart{}
	s := &Service{Start: f.start, Project: func(projects.Project) {}}
	s.Projects([]string{a, b})
	_, ctxA := f.watcher(t, a)
	_, ctxB := f.watcher(t, b)
	s.Projects([]string{b})
	select {
	case <-ctxA.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("the watch of a removed project goes on")
	}
	if ctxB.Err() != nil {
		t.Error("the watch of a kept project stopped")
	}
	s.Close()
	if ctxB.Err() == nil {
		t.Error("Close left a watch")
	}
	if f.starts != 2 {
		t.Errorf("%d watches started, want 2", f.starts)
	}
}

// A change of a startup file asks for a scan; a missing one is not
// watched.
func TestStartupFileChange(t *testing.T) {
	home := t.TempDir()
	zshrc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(zshrc, []byte("eval \"$(fnm env)\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeStart{}
	scans := make(chan struct{}, 1)
	s := &Service{Start: f.start, Shell: func() { scans <- struct{}{} }}
	defer s.Close()
	s.StartupFiles([]string{zshrc, filepath.Join(home, ".bashrc")})
	w, _ := f.watcher(t, zshrc)
	w.events <- []mygo.FileEvent{{Op: mygo.FileWrite, Path: "."}}
	select {
	case <-scans:
	case <-time.After(2 * time.Second):
		t.Fatal("no scan after .zshrc changed")
	}
	time.Sleep(20 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.starts != 1 {
		t.Errorf("%d watches started, want .zshrc's alone", f.starts)
	}
}

// The selected engine's containers are followed: a burst of events
// refreshes once, and a scan that finds no engine stops following it.
func TestDocker(t *testing.T) {
	lines := make(chan string)
	var mu sync.Mutex
	var args []string
	var stopped context.Context
	changes := make(chan struct{}, 4)
	s := &Service{
		Lines: func(ctx context.Context, c run.Cmd, line func(string)) error {
			mu.Lock()
			args, stopped = c.Args, ctx
			mu.Unlock()
			for {
				select {
				case l := <-lines:
					line(l)
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		},
		Containers: func() { changes <- struct{}{} },
	}
	defer s.Close()
	snap := &inventory.Snapshot{Tools: []inventory.ToolState{{Tool: "docker",
		Installations: []inventory.Installation{{Path: "/bin/docker", OnPath: 1}},
		Docker:        &inventory.DockerState{Selected: "local", Contexts: []inventory.DockerContext{{Name: "local", Endpoint: "unix:///docker.sock", Local: true, Status: "available"}}}}}}
	s.Docker(snap, nil)
	for _, l := range []string{"create", "start", "start"} {
		lines <- l
	}
	select {
	case <-changes:
	case <-time.After(5 * time.Second):
		t.Fatal("no refresh after the events")
	}
	select {
	case <-changes:
		t.Error("refreshed for each event")
	case <-time.After(2 * dockerSettle):
	}
	mu.Lock()
	got, ctx := args, stopped
	mu.Unlock()
	if len(got) < 4 || got[0] != "/bin/docker" || got[2] != "unix:///docker.sock" || got[3] != "events" {
		t.Fatal(got)
	}
	s.Docker(snap, nil) // the same engine: the watch goes on
	if ctx.Err() != nil {
		t.Fatal("restarted for the same engine")
	}
	snap.Tools[0].Docker.Contexts[0].Status = "unavailable"
	s.Docker(snap, nil)
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("still following a stopped engine")
	}
}
