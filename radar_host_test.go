package main

import (
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/proposals"
)

func TestRadarHubRecordAndDedup(t *testing.T) {
	hub := newRadarHub()
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

	// Working transitions never notify.
	notify, _, _, _ := hub.record(t0, proposals.AgentEvent{
		Agent: "claude", Event: "working", SessionID: "s1", Cwd: "/tmp/my-app",
	})
	if notify {
		t.Fatal("working event should not trigger a notification")
	}

	// Waiting notifies once and stores notification click target.
	notify, id1, title, body := hub.record(t0.Add(5*time.Second), proposals.AgentEvent{
		Agent: "claude", Event: "waiting", SessionID: "s1", Cwd: "/tmp/my-app",
	})
	if !notify || id1 == "" || title == "" || body == "" {
		t.Fatalf("waiting should notify, got notify=%v id=%q title=%q body=%q", notify, id1, title, body)
	}
	if cwd, ok := hub.targetForNotification(id1); !ok || cwd != "/tmp/my-app" {
		t.Fatalf("targetForNotification(%q) = (%q, %v), want (/tmp/my-app, true)", id1, cwd, ok)
	}

	// Duplicate waiting within cooldown is suppressed.
	notify, _, _, _ = hub.record(t0.Add(10*time.Second), proposals.AgentEvent{
		Agent: "claude", Event: "waiting", SessionID: "s1", Cwd: "/tmp/my-app",
	})
	if notify {
		t.Fatal("duplicate waiting event within cooldown should be suppressed")
	}

	// Working resets dedup, so subsequent finished notifies.
	hub.record(t0.Add(15*time.Second), proposals.AgentEvent{
		Agent: "claude", Event: "working", SessionID: "s1", Cwd: "/tmp/my-app",
	})
	notify, id2, _, _ := hub.record(t0.Add(20*time.Second), proposals.AgentEvent{
		Agent: "claude", Event: "finished", SessionID: "s1", Cwd: "/tmp/my-app",
	})
	if !notify || id2 == "" || id2 == id1 {
		t.Fatalf("finished after working should notify with new ID, got notify=%v id=%q", notify, id2)
	}
}
