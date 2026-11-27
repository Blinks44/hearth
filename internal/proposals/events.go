package proposals

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"
)

// AgentEvent is a strict-schema lifecycle signal sent by an opt-in agent hook
// via `hearth notify`. It NEVER carries prompt text, chat content, or commands.
type AgentEvent struct {
	Agent     string `json:"agent"`
	Event     string `json:"event"`
	SessionID string `json:"session_id,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
}

var allowedAgents = map[string]bool{
	"claude":      true,
	"codex":       true,
	"gemini":      true,
	"cursor":      true,
	"antigravity": true,
	"copilot":     true,
	"opencode":    true,
	"aider":       true,
	"kimi":        true,
	"grok":        true,
	"devin":       true,
}

var allowedEvents = map[string]bool{
	"working":  true,
	"waiting":  true,
	"finished": true,
}

// Valid checks that e contains only allowlisted agent/event values and bounded,
// control-character-free identifiers.
func (e AgentEvent) Valid() bool {
	if !allowedAgents[e.Agent] || !allowedEvents[e.Event] {
		return false
	}
	if len(e.SessionID) > 128 || len(e.Cwd) > 1024 {
		return false
	}
	if hasControlChar(e.SessionID) || hasControlChar(e.Cwd) {
		return false
	}
	return true
}

func hasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// SendAgentEvent posts a strict-schema AgentEvent to Hearth's authenticated
// loopback bridge with a short timeout so agent hooks never stall.
func SendAgentEvent(ctx context.Context, dir string, ev AgentEvent) error {
	if !ev.Valid() {
		return errors.New("invalid agent event")
	}
	if ev.Timestamp == 0 {
		ev.Timestamp = time.Now().Unix()
	}
	res, err := postBridge(ctx, dir, "/agent-event", 1500*time.Millisecond, ev)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1024))
	if res.StatusCode != http.StatusOK {
		return errors.New("hearth refused the agent event")
	}
	return nil
}
