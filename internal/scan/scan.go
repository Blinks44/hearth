// Package scan runs the detectors and puts what they find together in a
// snapshot, in the background, a newer scan replacing the one under way.
package scan

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/pkgmgr"
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
	{"npm", detect.Npm},
	{"pnpm", detect.Pnpm},
	{"yarn", detect.Yarn},
	{"corepack", detect.Corepack},
	{"go", detect.Go},
	{"python", detect.Python},
	{"php", detect.PHP},
	{"composer", detect.Composer},
	{"rust", detect.Rust},
	{"java", detect.Java},
	{"ruby", detect.Ruby},
	{"dotnet", detect.Dotnet},
	{"git", detect.Git},
	{"cpp", detect.Cpp},
	{"models", detect.Models},
	{"mise", detect.MiseTools},
	{"cli", detect.Cli},
	{"mobile", detect.Mobile},
	{"editors", detect.Editors},
	{"docker", detect.Docker},
	{"databases", detect.Databases},
	{"packages", pkgmgr.Detect},
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

	mu         sync.Mutex
	gen        int // counts the scans started, so that an older one is dropped
	cancel     context.CancelFunc
	refreshGen int        // counts the refreshes, as gen the scans
	term, gui  sysenv.Env // of the last scan, for refreshes
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
		s.mu.Lock()
		s.term, s.gui = term, gui
		s.mu.Unlock()
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

// Terminal returns the terminal's environment the last scan read, nil
// before one did.
func (s *Scanner) Terminal() sysenv.Env {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.term
}

// Refresh runs again only the detectors of tools, in the environment of
// the last scan, and gives done what they found, in the detectors' order:
// it is for a change one part of the computer reported. done runs on the
// refresh's goroutine, unless a later Refresh overtook it.
func (s *Scanner) Refresh(tools []string, done func([]inventory.ToolState)) {
	s.mu.Lock()
	s.refreshGen++
	gen, term, gui := s.refreshGen, s.term, s.gui
	s.mu.Unlock()
	go func() {
		ctx := context.Background()
		if term == nil {
			env := s.Env
			if env == nil {
				env = systemEnv
			}
			term, gui = env(ctx, s.Runner)
		}
		var ds []Detector
		for _, d := range s.Detectors {
			if slices.Contains(tools, d.Tool) {
				ds = append(ds, d)
			}
		}
		states := detectTools(ctx, s.Runner, term, gui, ds)
		s.mu.Lock()
		defer s.mu.Unlock()
		if gen == s.refreshGen {
			done(states)
		}
	}()
}

// Replace returns snap with the tools' states replaced by those of
// states. snap itself is left alone, as another goroutine may read it.
func Replace(snap inventory.Snapshot, states []inventory.ToolState) inventory.Snapshot {
	snap.Tools = slices.Clone(snap.Tools)
	for _, st := range states {
		if i := slices.IndexFunc(snap.Tools, func(t inventory.ToolState) bool { return t.Tool == st.Tool }); i >= 0 {
			snap.Tools[i] = st
		}
	}
	return snap
}

// Run runs the detectors, a few at once, and returns what they found in
// their order, with the AI agents and the user's skills and MCP servers,
// found beside them. A detector's error
// leaves the others alone: its tool gets the error and whatever it found
// before it.
func Run(ctx context.Context, r run.Runner, term, gui sysenv.Env, ds []Detector) inventory.Snapshot {
	var wg sync.WaitGroup
	var agents []inventory.Agent
	var skills []inventory.Skill
	var mcp []inventory.MCPServer
	var mcpErrs []string
	wg.Go(func() { agents = ai.Detect(ctx, r, term, gui) })
	wg.Go(func() { skills = ai.Skills(term) })
	wg.Go(func() { mcp, mcpErrs = ai.MCP(term) })
	tools := detectTools(ctx, r, term, gui, ds)
	wg.Wait()
	return inventory.Snapshot{Taken: time.Now(), Tools: tools, Agents: agents, Skills: skills, MCP: mcp, MCPErrs: mcpErrs,
		TerminalPath: sysenv.Paths(term), GUIPath: sysenv.Paths(gui), Variables: sysenv.Variables(term, gui)}
}

// detectTools runs the detectors, a few at once, and returns their states
// in their order.
func detectTools(ctx context.Context, r run.Runner, term, gui sysenv.Env, ds []Detector) []inventory.ToolState {
	tools := make([]inventory.ToolState, len(ds))
	sem := make(chan struct{}, parallel)
	var wg sync.WaitGroup
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
	return tools
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
