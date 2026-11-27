package inventory

import "time"

// AgentSessionStatus is the lifecycle state of a live AI coding agent session.
type AgentSessionStatus string

const (
	// AgentStatusWorking means the agent is actively processing or executing tools.
	AgentStatusWorking AgentSessionStatus = "working"
	// AgentStatusQuiet is the honest passive state when a running agent process
	// has not updated its session files recently (>30s) without opt-in hooks.
	AgentStatusQuiet AgentSessionStatus = "quiet"
	// AgentStatusWaiting means an opt-in lifecycle hook reported that the agent
	// is waiting for user approval or input.
	AgentStatusWaiting AgentSessionStatus = "waiting"
	// AgentStatusFinished means an opt-in lifecycle hook reported that the agent
	// finished its current turn.
	AgentStatusFinished AgentSessionStatus = "finished"
)

// ActiveAgentSession represents one live or recently active AI agent session.
// It carries only non-sensitive metadata (agent ID, status, working directory,
// PID, model, token counts, cost, timestamps) — never prompts or chat text.
type ActiveAgentSession struct {
	Key       string
	Agent     string
	Status    AgentSessionStatus
	Exact     bool // true when backed by an opt-in lifecycle hook
	PID       string
	Cwd       string
	SessionID string
	Model     string
	Tokens    int64
	CostUSD   float64
	UpdatedAt time.Time
}
