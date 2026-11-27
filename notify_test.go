package main

import (
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/proposals"
)

func TestRunNotifyStrictMetadataOnly(t *testing.T) {
	dir := t.TempDir()
	gotCh := make(chan proposals.AgentEvent, 1)
	stop, err := proposals.ListenWithEvents(dir, nil, func(ev proposals.AgentEvent) {
		gotCh <- ev
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// Stdin contains sensitive prompt text alongside allowed metadata;
	// Hearth must extract only cwd/session_id/event and discard prompt.
	stdin := strings.NewReader(`{
		"hook_event_name": "Notification",
		"notification_type": "permission_prompt",
		"cwd": "/dev/hearth",
		"session_id": "abc-123",
		"prompt": "SUPER SECRET PROMPT TEXT",
		"transcript": "DO NOT READ"
	}`)

	code := runNotify([]string{"--agent", "claude", "--data-dir", dir}, stdin)
	if code != 0 {
		t.Fatalf("runNotify exited %d, want 0", code)
	}
	got := <-gotCh
	if got.Agent != "claude" || got.Event != "waiting" || got.Cwd != "/dev/hearth" || got.SessionID != "abc-123" {
		t.Fatalf("unexpected event: %+v", got)
	}
}
