package main

import (
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/ai"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/proposals"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const radarNotifCooldown = 45 * time.Second

type radarNotifRecord struct {
	event string
	at    time.Time
}

// radarHub holds live opt-in agent hook events, notification deduplication
// state, and notification click routing targets off the UI thread.
type radarHub struct {
	mu           sync.Mutex
	events       map[string]proposals.AgentEvent
	lastNotified map[string]radarNotifRecord
	notifTargets map[string]string
	notifOrder   []string
	notifSeq     uint64
}

func newRadarHub() *radarHub {
	return &radarHub{
		events:       map[string]proposals.AgentEvent{},
		lastNotified: map[string]radarNotifRecord{},
		notifTargets: map[string]string{},
	}
}

func radarEventKey(ev proposals.AgentEvent) string {
	if ev.SessionID != "" {
		return ev.Agent + "\x00id:" + ev.SessionID
	}
	if ev.Cwd != "" {
		return ev.Agent + "\x00dir:" + filepath.Clean(ev.Cwd)
	}
	return ev.Agent
}

func radarAgentDisplayName(id string) string {
	switch id {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "copilot":
		return "Copilot CLI"
	case "antigravity":
		return "Antigravity CLI"
	case "gemini":
		return "Gemini CLI"
	case "cursor":
		return "Cursor"
	case "opencode":
		return "OpenCode"
	case "devin":
		return "Devin"
	case "grok":
		return "Grok Build"
	case "kimi":
		return "Kimi Code"
	case "aider":
		return "Aider"
	default:
		return id
	}
}

// record stores an incoming hook event and reports whether Hearth should emit
// a desktop notification for this transition.
func (h *radarHub) record(now time.Time, ev proposals.AgentEvent) (notify bool, notifID, title, body string) {
	if ev.Timestamp == 0 {
		ev.Timestamp = now.Unix()
	}
	key := radarEventKey(ev)

	h.mu.Lock()
	defer h.mu.Unlock()

	h.pruneLocked(now)
	h.events[key] = ev
	if ev.Event != "waiting" && ev.Event != "finished" {
		// Clear notification dedup on working so the next waiting/finished notifies again.
		delete(h.lastNotified, key)
		return false, "", "", ""
	}
	if prev, ok := h.lastNotified[key]; ok && prev.event == ev.Event && now.Sub(prev.at) < radarNotifCooldown {
		return false, "", "", ""
	}
	h.lastNotified[key] = radarNotifRecord{event: ev.Event, at: now}

	h.notifSeq++
	notifID = fmt.Sprintf("hearth-agent-%s-%d", ev.Agent, h.notifSeq)
	h.notifTargets[notifID] = ev.Cwd
	h.notifOrder = append(h.notifOrder, notifID)
	// FIFO-evict oldest notification click targets past 64 entries.
	for len(h.notifOrder) > 64 {
		oldest := h.notifOrder[0]
		h.notifOrder = h.notifOrder[1:]
		delete(h.notifTargets, oldest)
	}

	name := radarAgentDisplayName(ev.Agent)
	if ev.Event == "waiting" {
		title = locales.RadarNotifWaitingTitle(name)
	} else {
		title = locales.RadarNotifFinishedTitle(name)
	}
	if cwd := strings.TrimSpace(ev.Cwd); cwd != "" {
		body = locales.RadarNotifBody(filepath.Base(cwd))
	} else {
		body = locales.RadarNotifBodyNoProject()
	}
	return true, notifID, title, body
}

func (h *radarHub) pruneLocked(now time.Time) {
	const maxAge = 2 * time.Hour
	for k, ev := range h.events {
		if ev.Timestamp > 0 && now.Sub(time.Unix(ev.Timestamp, 0)) > maxAge {
			delete(h.events, k)
		}
	}
	for k, rec := range h.lastNotified {
		if now.Sub(rec.at) > maxAge {
			delete(h.lastNotified, k)
		}
	}
}

func (h *radarHub) snapshotEvents(now time.Time) map[string]proposals.AgentEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneLocked(now)
	return maps.Clone(h.events)
}

func (h *radarHub) targetForNotification(id string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cwd, ok := h.notifTargets[id]
	return cwd, ok
}

// refresh runs passive OS process detection and hook-file inspection off the
// UI thread and merges everything with live hook events and recent usage sessions.
func (h *radarHub) refresh(ctx context.Context, runner run.Runner, env sysenv.Env, home string, recent []inventory.AgentSession) ([]inventory.ActiveAgentSession, map[string]bool) {
	now := time.Now()
	live := ai.ScanLiveAgentProcesses(ctx, runner, env)
	events := h.snapshotEvents(now)
	sessions := ai.MergeRadar(now, events, live, recent)

	hooks := make(map[string]bool, len(ops.SupportedHookAgents))
	for _, agent := range ops.SupportedHookAgents {
		hooks[agent] = ops.HearthHooksInstalled(agent, home)
	}
	return sessions, hooks
}
