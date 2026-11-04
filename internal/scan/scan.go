// Package scan runs the detectors and puts what they find together in a
// snapshot, in the background, a newer scan replacing the one under way.
package scan

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Detector finds the copies of one tool.
type Detector struct {
	Tool   string
	Detect func(ctx context.Context, r run.Runner, term, gui sysenv.Env) (inventory.ToolState, error)
}

// Detectors are the ones Hearth runs.
var Detectors = []Detector{
	{"node", detect.Node},
	{"bun", detect.Bun},
	{"go", detect.Go},
}

// parallel is how many detectors run at once: each may start commands.
const parallel = 4

// Scanner scans the computer in the background.
type Scanner struct {
	Runner    run.Runner
	Detectors []Detector
	// Env returns the terminal's and the GUI's environment; nil reads this
	// computer's.
	Env func(ctx context.Context, r run.Runner) (term, gui sysenv.Env)

	mu     sync.Mutex
	gen    int // counts the scans started, so that an older one is dropped
	cancel context.CancelFunc
}

// Start cancels the scan under way, if any, and scans again. done gets the
// snapshot on the scan's goroutine, unless a later Start overtook the scan;
// it must not block, as Window.Update does not.
func (s *Scanner) Start(done func(inventory.Snapshot)) {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.gen++
	gen := s.gen
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer cancel()
		env := s.Env
		if env == nil {
			env = systemEnv
		}
		term, gui := env(ctx, s.Runner)
		snap := Run(ctx, s.Runner, term, gui, s.Detectors)
		// Under the lock, so that a Start meanwhile cannot let this older
		// snapshot arrive after the newer one.
		s.mu.Lock()
		defer s.mu.Unlock()
		if gen == s.gen {
			done(snap)
		}
	}()
}

// Run runs the detectors, a few at once, and returns what they found in
// their order, with the AI agents and the user's skills and MCP servers,
// found beside them. A detector's error
// leaves the others alone: its tool gets the error and whatever it found
// before it.
func Run(ctx context.Context, r run.Runner, term, gui sysenv.Env, ds []Detector) inventory.Snapshot {
	tools := make([]inventory.ToolState, len(ds))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
	var agents []inventory.Agent
	var skills []inventory.Skill
	var mcp []inventory.MCPServer
	var mcpErrs []string
	wg.Go(func() { agents = ai.Detect(ctx, r, term, gui) })
	wg.Go(func() { skills = ai.Skills(term) })
	wg.Go(func() { mcp, mcpErrs = ai.MCP(term) })
	for i, d := range ds {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			st, err := d.Detect(ctx, r, term, gui)
			st.Tool = d.Tool
			if err != nil {
				st.Err = err.Error()
			}
			tools[i] = st
		})
	}
	wg.Wait()
	return inventory.Snapshot{Taken: time.Now(), Tools: tools, Agents: agents, Skills: skills, MCP: mcp, MCPErrs: mcpErrs,
		TerminalPath: pathEntries(term), GUIPath: pathEntries(gui)}
}

// pathEntries lists the PATH of env, telling which directories are not
// there. A relative entry is not looked for: it depends on the folder.
func pathEntries(env sysenv.Env) []inventory.PathEntry {
	var out []inventory.PathEntry
	for _, dir := range filepath.SplitList(env.Get("PATH")) {
		e := inventory.PathEntry{Dir: dir}
		if filepath.IsAbs(dir) {
			_, err := os.Stat(dir)
			e.Missing = errors.Is(err, os.ErrNotExist)
		}
		out = append(out, e)
	}
	return out
}

// systemEnv reads the terminal's environment from the user's shell, and
// falls back to the app's own when the shell fails.
func systemEnv(ctx context.Context, r run.Runner) (term, gui sysenv.Env) {
	gui = sysenv.GUI()
	term, err := sysenv.Terminal(ctx, r)
	if err != nil {
		log.Printf("scan: terminal environment: %v", err)
		term = gui
	}
	return term, gui
}
