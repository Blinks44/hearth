package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/internal/proposals"
)

// runNotify handles `hearth notify --agent <id> --event <working|waiting|finished>`.
// Invoked by opt-in AI agent lifecycle hooks. It reads ONLY strict non-sensitive
// metadata (agent, event, session_id, cwd) and discards all prompt/chat fields.
// Always exits 0 so hooks never interrupt or slow down the calling agent.
func runNotify(args []string, stdin io.Reader) int {
	fs := flag.NewFlagSet("notify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var agent, event, cwd, sessionID, dataDir string
	fs.StringVar(&agent, "agent", "", "agent id")
	fs.StringVar(&event, "event", "", "lifecycle event")
	fs.StringVar(&cwd, "cwd", "", "working directory")
	fs.StringVar(&sessionID, "session", "", "session id")
	fs.StringVar(&dataDir, "data-dir", "", "override hearth user data dir")
	if err := fs.Parse(args); err != nil {
		return 0
	}

	ev := parseHookEvent(agent, event, cwd, sessionID, readHookStdin(stdin))
	if !ev.Valid() {
		return 0
	}
	if dataDir == "" {
		dataDir = defaultNotifyDataDir()
	}
	if dataDir == "" {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	_ = proposals.SendAgentEvent(ctx, dataDir, ev)
	return 0
}

// defaultNotifyDataDir mirrors mcpserver.DefaultReader without calling
// mygo.App.Path, which would create directories on disk as a side effect.
func defaultNotifyDataDir() string {
	cfg, err := os.UserConfigDir()
	if err != nil || cfg == "" {
		return ""
	}
	appName := mygo.App.Name()
	if appName == "" {
		appName = "Hearth"
	}
	return filepath.Join(cfg, appName)
}

// hookStdinMeta is the strict allowlist of non-sensitive fields read from an
// agent hook's JSON stdin payload. All other fields (prompts, tool args, chat
// transcripts) are ignored and never retained.
type hookStdinMeta struct {
	HookEventName    string `json:"hook_event_name"`
	NotificationType string `json:"notification_type"`
	Cwd              string `json:"cwd"`
	SessionID        string `json:"session_id"`
	ConversationID   string `json:"conversation_id"`
}

func readHookStdin(r io.Reader) hookStdinMeta {
	if r == nil {
		return hookStdinMeta{}
	}
	if f, ok := r.(*os.File); ok {
		info, err := f.Stat()
		if err != nil || (info.Mode()&os.ModeCharDevice) != 0 {
			return hookStdinMeta{} // interactive TTY, nothing piped
		}
	}
	const maxStdinBytes = 512 << 10
	ch := make(chan []byte, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(r, maxStdinBytes))
		if err == nil && len(data) > 0 {
			ch <- data
			return
		}
		ch <- nil
	}()
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	select {
	case data := <-ch:
		if len(data) == 0 {
			return hookStdinMeta{}
		}
		var meta hookStdinMeta
		_ = json.Unmarshal(data, &meta)
		return meta
	case <-timer.C:
		return hookStdinMeta{}
	}
}

func parseHookEvent(agent, event, cwd, sessionID string, meta hookStdinMeta) proposals.AgentEvent {
	agent = strings.ToLower(strings.TrimSpace(agent))
	event = strings.ToLower(strings.TrimSpace(event))
	if cwd == "" {
		cwd = strings.TrimSpace(meta.Cwd)
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(meta.SessionID)
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(meta.ConversationID)
	}
	if event == "" {
		event = mapHookEventName(meta.HookEventName, meta.NotificationType)
	}
	return proposals.AgentEvent{
		Agent:     agent,
		Event:     event,
		SessionID: sessionID,
		Cwd:       cwd,
		Timestamp: time.Now().Unix(),
	}
}

func mapHookEventName(hookName, notifType string) string {
	switch strings.ToLower(strings.TrimSpace(hookName)) {
	case "notification":
		return "waiting"
	case "stop", "afteragent", "sessionend":
		return "finished"
	case "userpromptsubmit", "beforeagent", "beforesubmitprompt", "pretooluse":
		return "working"
	}
	switch strings.ToLower(strings.TrimSpace(notifType)) {
	case "permission_prompt", "idle_prompt", "elicitation_dialog":
		return "waiting"
	}
	return ""
}
