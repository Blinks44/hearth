package ai

import (
	"cmp"
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/proposals"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// LiveAgentProcess is a currently running AI coding agent process detected
// passively from the OS process table (PID + working directory only).
type LiveAgentProcess struct {
	Agent string
	PID   string
	Cwd   string
}

// ScanLiveAgentProcesses finds running AI coding agent CLI processes owned by
// the current user. Discovery is strictly read-only and never reads prompts.
func ScanLiveAgentProcesses(ctx context.Context, r run.Runner, env sysenv.Env) []LiveAgentProcess {
	return scanLiveAgentProcesses(ctx, r, env)
}

// matchAgentProcess maps an executable basename or argv token to a Hearth agent ID.
func matchAgentProcess(comm string, args []string) string {
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(comm), ".exe"))
	// Skip Hearth itself or shell utilities.
	if base == "hearth" || base == "ps" || base == "lsof" || base == "grep" || base == "rg" {
		return ""
	}
	// Skip background server/helper subcommands (e.g. `codex app-server`, `claude mcp`, IDE LSP workers).
	for i, a := range args {
		if i == 0 || i > 4 {
			continue
		}
		switch strings.ToLower(a) {
		case "app-server", "mcp", "mcp-server", "language-server", "lsp", "serve", "daemon":
			return ""
		}
	}
	if id := exactAgentBinary(base); id != "" {
		return id
	}
	// Node/Python/Bun wrappers running an agent CLI script (e.g. `node .../claude`, `python3 .../aider`).
	if base == "node" || base == "nodejs" || base == "bun" || base == "deno" || base == "python" || base == "python3" || strings.HasPrefix(base, "python3.") {
		for i, a := range args {
			if i == 0 || i > 4 {
				continue
			}
			// Ignore MCP server helpers or hearth notify hooks.
			low := strings.ToLower(a)
			if strings.Contains(low, "mcp") || strings.Contains(low, "hearth") || strings.Contains(low, "app-server") {
				return ""
			}
			tok := strings.ToLower(strings.TrimSuffix(filepath.Base(a), ".js"))
			tok = strings.TrimSuffix(tok, ".mjs")
			tok = strings.TrimSuffix(tok, ".cjs")
			if id := exactAgentBinary(tok); id != "" {
				return id
			}
			if strings.Contains(low, "@anthropic-ai/claude-code") {
				return "claude"
			}
			if strings.Contains(low, "@openai/codex") {
				return "codex"
			}
			if strings.Contains(low, "@google/gemini-cli") {
				return "gemini"
			}
		}
	}
	return ""
}

func exactAgentBinary(name string) string {
	switch name {
	case "claude", "claude-code":
		return "claude"
	case "codex":
		return "codex"
	case "gemini":
		return "gemini"
	case "copilot", "github-copilot-cli":
		return "copilot"
	case "cursor-agent":
		return "cursor"
	case "opencode":
		return "opencode"
	case "aider":
		return "aider"
	case "kimi":
		return "kimi"
	case "grok":
		return "grok"
	case "devin":
		return "devin"
	case "antigravity":
		return "antigravity"
	}
	return ""
}

// MergeRadar combines opt-in hook events, live passive processes, and recent
// token/cost session metadata into a sorted list of active agent sessions.
// Hook events take precedence over passive heuristics when they refer to the
// same agent + directory.
func MergeRadar(now time.Time, hookEvents map[string]proposals.AgentEvent, live []LiveAgentProcess, recent []inventory.AgentSession) []inventory.ActiveAgentSession {
	const (
		hookFinishTTL  = 15 * time.Minute
		hookActiveTTL  = 2 * time.Hour
		passiveQuiet   = 30 * time.Second
		usageRecentTTL = 6 * time.Hour
	)

	// Index the freshest recent token/cost session per (agent, non-empty cleaned dir).
	type usageSnap struct {
		model   string
		tokens  int64
		costUSD float64
		updated time.Time
	}
	latestUsage := map[string]usageSnap{}
	for _, s := range recent {
		if s.Agent == "" {
			continue
		}
		dir := cleanRadarPath(s.Dir)
		if dir == "" {
			continue
		}
		if !s.Modified.IsZero() && now.Sub(s.Modified) > usageRecentTTL {
			continue
		}
		k := s.Agent + "\x00" + dir
		var tokens int64
		var cost float64
		model := ""
		for _, u := range s.Usage {
			tokens += u.Tokens()
			if c, ok := Cost(u); ok {
				cost += c
			}
			if u.Model != "" {
				model = u.Model
			}
		}
		ts := s.Modified
		if prev, ok := latestUsage[k]; !ok || ts.After(prev.updated) {
			latestUsage[k] = usageSnap{model: model, tokens: tokens, costUSD: cost, updated: ts}
		}
	}

	var out []inventory.ActiveAgentSession
	coveredAgentDir := map[string]bool{}
	usedPIDs := map[string]bool{}

	hookKeys := make([]string, 0, len(hookEvents))
	for k := range hookEvents {
		hookKeys = append(hookKeys, k)
	}
	slices.SortStableFunc(hookKeys, func(a, b string) int {
		if ta, tb := hookEvents[a].Timestamp, hookEvents[b].Timestamp; ta != tb {
			return cmp.Compare(tb, ta)
		}
		return cmp.Compare(a, b)
	})

	// 1. Exact opt-in hook events.
	for _, key := range hookKeys {
		ev := hookEvents[key]
		ts := time.Unix(ev.Timestamp, 0)
		age := now.Sub(ts)
		if ev.Event == "finished" && age > hookFinishTTL {
			continue
		}
		if age > hookActiveTTL {
			continue
		}
		status := inventory.AgentStatusWorking
		switch ev.Event {
		case "waiting":
			status = inventory.AgentStatusWaiting
		case "finished":
			status = inventory.AgentStatusFinished
		}
		dir := cleanRadarPath(ev.Cwd)
		sess := inventory.ActiveAgentSession{
			Key:       "hook:" + key,
			Agent:     ev.Agent,
			Status:    status,
			Exact:     true,
			Cwd:       ev.Cwd,
			SessionID: ev.SessionID,
			UpdatedAt: ts,
		}
		// Attach matching live PID if present: prefer exact directory match,
		// then fall back to live processes without cwd (e.g. Windows) or events without cwd.
		matchedIdx := -1
		if dir != "" {
			for i, p := range live {
				if p.Agent == ev.Agent && !usedPIDs[p.PID] && cleanRadarPath(p.Cwd) == dir {
					matchedIdx = i
					break
				}
			}
		}
		if matchedIdx < 0 {
			for i, p := range live {
				if p.Agent == ev.Agent && !usedPIDs[p.PID] && (dir == "" || cleanRadarPath(p.Cwd) == "") {
					matchedIdx = i
					break
				}
			}
		}
		if matchedIdx >= 0 {
			p := live[matchedIdx]
			sess.PID = p.PID
			usedPIDs[p.PID] = true
			if sess.Cwd == "" && p.Cwd != "" {
				sess.Cwd = p.Cwd
				dir = cleanRadarPath(sess.Cwd)
			}
		}
		if dir != "" {
			if u, ok := latestUsage[ev.Agent+"\x00"+dir]; ok {
				sess.Model = u.model
				sess.Tokens = u.tokens
				sess.CostUSD = u.costUSD
			}
		}
		coveredAgentDir[ev.Agent+"\x00"+dir] = true
		out = append(out, sess)
	}

	// 2. Passive live processes not already covered by an exact hook event or sibling worker.
	for _, p := range live {
		if usedPIDs[p.PID] {
			continue
		}
		dir := cleanRadarPath(p.Cwd)
		if coveredAgentDir[p.Agent+"\x00"+dir] {
			continue
		}
		coveredAgentDir[p.Agent+"\x00"+dir] = true
		sess := inventory.ActiveAgentSession{
			Key:       "pid:" + p.PID,
			Agent:     p.Agent,
			Status:    inventory.AgentStatusQuiet,
			Exact:     false,
			PID:       p.PID,
			Cwd:       p.Cwd,
			UpdatedAt: now,
		}
		if dir != "" {
			if u, ok := latestUsage[p.Agent+"\x00"+dir]; ok {
				sess.Model = u.model
				sess.Tokens = u.tokens
				sess.CostUSD = u.costUSD
				if now.Sub(u.updated) <= passiveQuiet {
					sess.Status = inventory.AgentStatusWorking
				}
				if !u.updated.IsZero() {
					sess.UpdatedAt = u.updated
				}
			}
		}
		out = append(out, sess)
	}

	slices.SortStableFunc(out, func(a, b inventory.ActiveAgentSession) int {
		if pa, pb := radarRank(a.Status), radarRank(b.Status); pa != pb {
			return cmp.Compare(pa, pb)
		}
		if !a.UpdatedAt.Equal(b.UpdatedAt) {
			if a.UpdatedAt.After(b.UpdatedAt) {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Key, b.Key)
	})
	return out
}

func radarRank(s inventory.AgentSessionStatus) int {
	switch s {
	case inventory.AgentStatusWaiting:
		return 0
	case inventory.AgentStatusWorking:
		return 1
	case inventory.AgentStatusQuiet:
		return 2
	case inventory.AgentStatusFinished:
		return 3
	default:
		return 4
	}
}

func cleanRadarPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	return normalizeRadarPath(filepath.ToSlash(filepath.Clean(p)))
}
