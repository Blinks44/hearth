package proposals

import (
	"context"
	"testing"
)

func TestAgentEventRoundTripAndValidation(t *testing.T) {
	dir := t.TempDir()
	gotCh := make(chan AgentEvent, 1)
	stop, err := ListenWithEvents(dir, nil, func(ev AgentEvent) {
		gotCh <- ev
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	want := AgentEvent{
		Agent:     "claude",
		Event:     "waiting",
		SessionID: "sess-123",
		Cwd:       "/dev/hearth",
	}
	if err := SendAgentEvent(context.Background(), dir, want); err != nil {
		t.Fatalf("SendAgentEvent: %v", err)
	}
	got := <-gotCh
	if got.Agent != want.Agent || got.Event != want.Event || got.SessionID != want.SessionID || got.Cwd != want.Cwd || got.Timestamp == 0 {
		t.Fatalf("got %+v, want %+v", got, want)
	}

	// Reject unknown agent or event.
	if err := SendAgentEvent(context.Background(), dir, AgentEvent{Agent: "evil", Event: "waiting"}); err == nil {
		t.Fatal("expected invalid agent error")
	}
	if err := SendAgentEvent(context.Background(), dir, AgentEvent{Agent: "claude", Event: "prompt_dump"}); err == nil {
		t.Fatal("expected invalid event error")
	}
}
