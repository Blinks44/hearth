package ops

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/Blinks44/hearth/locales"
)

// SupportedHookAgents lists the AI coding agents whose global hook files
// Hearth can configure opt-in for exact session status & notifications.
var SupportedHookAgents = []string{"claude", "codex", "gemini", "cursor"}

// AgentHookFile returns the global JSON hook file path for an agent.
func AgentHookFile(agent, home string) (string, bool) {
	if home == "" {
		return "", false
	}
	switch agent {
	case "claude":
		return filepath.Join(home, ".claude", "settings.json"), true
	case "codex":
		return filepath.Join(home, ".codex", "hooks.json"), true
	case "gemini":
		return filepath.Join(home, ".gemini", "settings.json"), true
	case "cursor":
		return filepath.Join(home, ".cursor", "hooks.json"), true
	default:
		return "", false
	}
}

// HearthHooksInstalled reports whether Hearth's lifecycle notification hooks
// are already present in the agent's global configuration file.
func HearthHooksInstalled(agent, home string) bool {
	path, ok := AgentHookFile(agent, home)
	if !ok {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return false
	}
	hooksMap, ok := root["hooks"].(map[string]any)
	if !ok {
		return false
	}
	for _, b := range agentHookBindings(agent) {
		list, ok := hooksMap[b.hookEvent].([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			if entryHasHearthNotify(item, agent) {
				return true
			}
		}
	}
	return false
}

type hookEventBinding struct {
	hookEvent  string
	radarEvent string
}

func agentHookBindings(agent string) []hookEventBinding {
	switch agent {
	case "claude", "codex":
		return []hookEventBinding{
			{hookEvent: "UserPromptSubmit", radarEvent: "working"},
			{hookEvent: "Notification", radarEvent: "waiting"},
			{hookEvent: "Stop", radarEvent: "finished"},
		}
	case "gemini":
		return []hookEventBinding{
			{hookEvent: "BeforeAgent", radarEvent: "working"},
			{hookEvent: "Notification", radarEvent: "waiting"},
			{hookEvent: "AfterAgent", radarEvent: "finished"},
		}
	case "cursor":
		return []hookEventBinding{
			{hookEvent: "beforeSubmitPrompt", radarEvent: "working"},
			{hookEvent: "stop", radarEvent: "finished"},
		}
	default:
		return nil
	}
}

func formatNotifyCommand(hearthExe, agent, event string) string {
	exe := strings.TrimSpace(hearthExe)
	if exe == "" {
		exe = "hearth"
	}
	return fmt.Sprintf("%s notify --agent %s --event %s", quoteHookExe(exe), agent, event)
}

func isHearthNotifyCmd(cmd, agent string) bool {
	return strings.Contains(cmd, "notify --agent "+agent)
}

// PlanInstallAgentHooks builds an explicit confirmation Plan (with diff preview
// and backup) to add Hearth's opt-in notification hooks for the given agent.
func PlanInstallAgentHooks(agent, agentTitle, home, hearthExe string) (Plan, error) {
	return planAgentHooks(agent, agentTitle, home, hearthExe, true)
}

// PlanRemoveAgentHooks builds an explicit confirmation Plan (with diff preview
// and backup) to remove Hearth's notification hooks for the given agent while
// preserving all user hooks and settings.
func PlanRemoveAgentHooks(agent, agentTitle, home string) (Plan, error) {
	return planAgentHooks(agent, agentTitle, home, "", false)
}

func planAgentHooks(agent, agentTitle, home, hearthExe string, install bool) (Plan, error) {
	path, ok := AgentHookFile(agent, home)
	if !ok {
		return Plan{}, fmt.Errorf("agent %q does not support Hearth notification hooks", agent)
	}
	bindings := agentHookBindings(agent)
	var before string
	existed := false
	raw, err := os.ReadFile(path)
	if err == nil {
		existed = true
		before = string(raw)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Plan{}, fmt.Errorf("read %s: %w", path, err)
	}
	root := map[string]any{}
	if strings.TrimSpace(before) != "" {
		if err := json.Unmarshal([]byte(before), &root); err != nil {
			return Plan{}, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	hooksMap := map[string]any{}
	if rawHooks, present := root["hooks"]; present && rawHooks != nil {
		existing, ok := rawHooks.(map[string]any)
		if !ok {
			return Plan{}, fmt.Errorf("parse %s: hooks must be a JSON object", path)
		}
		hooksMap = existing
	}

	after := before
	if strings.TrimSpace(after) == "" {
		after = "{}\n"
	}

	isCursor := agent == "cursor"
	changed := false
	remainingEvents := len(hooksMap)
	for _, b := range bindings {
		list, hadEvent := hooksMap[b.hookEvent].([]any)
		// Strip only Hearth's command entries while preserving any user hooks
		// sharing the same wrapper object.
		filtered, removed := stripHearthHooks(list, agent)
		if install {
			cmd := formatNotifyCommand(hearthExe, agent, b.radarEvent)
			if isCursor {
				filtered = append(filtered, map[string]any{"command": cmd})
			} else {
				filtered = append(filtered, map[string]any{
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": cmd,
						},
					},
				})
			}
			encoded, err := json.Marshal(filtered)
			if err != nil {
				return Plan{}, fmt.Errorf("encode %s: %w", path, err)
			}
			after, err = JSONSet(after, []string{"hooks", b.hookEvent}, encoded)
			if err != nil {
				return Plan{}, fmt.Errorf("update %s: %w", path, err)
			}
			if !hadEvent {
				remainingEvents++
			}
			changed = true
			continue
		}
		if !removed {
			continue
		}
		changed = true
		if len(filtered) > 0 {
			encoded, err := json.Marshal(filtered)
			if err != nil {
				return Plan{}, fmt.Errorf("encode %s: %w", path, err)
			}
			after, err = JSONSet(after, []string{"hooks", b.hookEvent}, encoded)
			if err != nil {
				return Plan{}, fmt.Errorf("update %s: %w", path, err)
			}
		} else {
			after, err = JSONDelete(after, []string{"hooks", b.hookEvent})
			if err != nil {
				return Plan{}, fmt.Errorf("update %s: %w", path, err)
			}
			if hadEvent {
				remainingEvents--
			}
		}
	}
	if install && isCursor && root["version"] == nil {
		var setErr error
		after, setErr = JSONSet(after, []string{"version"}, json.RawMessage("1"))
		if setErr != nil {
			return Plan{}, fmt.Errorf("update %s: %w", path, setErr)
		}
		changed = true
	}
	if !install && changed && remainingEvents <= 0 {
		var delErr error
		after, delErr = JSONDelete(after, []string{"hooks"})
		if delErr != nil {
			return Plan{}, fmt.Errorf("update %s: %w", path, delErr)
		}
	}
	if !strings.HasSuffix(after, "\n") {
		after += "\n"
	}
	if !changed || (existed && after == before) {
		return Plan{}, errors.New("no changes needed")
	}

	title := locales.RadarHooksInstallTitle(agentTitle)
	change := locales.RadarHooksInstallChange(filepath.Base(path))
	why := locales.RadarHooksInstallWhy()
	if !install {
		title = locales.RadarHooksRemoveTitle(agentTitle)
		change = locales.RadarHooksRemoveChange(filepath.Base(path))
		why = locales.RadarHooksRemoveWhy()
	}
	return Plan{
		Title: title,
		Steps: []Step{{Edit: &FileEdit{
			Path:   path,
			Before: before,
			After:  after,
			New:    !existed,
		}}},
		Why:     why,
		Changes: []string{change},
	}, nil
}

// stripHearthHooks removes only Hearth's notify commands from list. When a
// wrapper object contains both user hooks and a Hearth hook inside its "hooks"
// array, only the Hearth element is dropped and the wrapper is preserved.
func stripHearthHooks(list []any, agent string) ([]any, bool) {
	filtered := make([]any, 0, len(list)+1)
	removed := false
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		if cmd, ok := m["command"].(string); ok && isHearthNotifyCmd(cmd, agent) {
			removed = true
			continue
		}
		sub, hasSub := m["hooks"].([]any)
		if !hasSub {
			filtered = append(filtered, item)
			continue
		}
		keptSub := make([]any, 0, len(sub))
		subRemoved := false
		for _, h := range sub {
			if hm, ok := h.(map[string]any); ok {
				if cmd, ok := hm["command"].(string); ok && isHearthNotifyCmd(cmd, agent) {
					subRemoved = true
					continue
				}
			}
			keptSub = append(keptSub, h)
		}
		if !subRemoved {
			filtered = append(filtered, item)
			continue
		}
		removed = true
		if len(keptSub) == 0 {
			continue
		}
		cloned := maps.Clone(m)
		cloned["hooks"] = keptSub
		filtered = append(filtered, cloned)
	}
	return filtered, removed
}

func entryHasHearthNotify(item any, agent string) bool {
	m, ok := item.(map[string]any)
	if !ok {
		return false
	}
	if cmd, ok := m["command"].(string); ok && isHearthNotifyCmd(cmd, agent) {
		return true
	}
	if sub, ok := m["hooks"].([]any); ok {
		for _, h := range sub {
			if hm, ok := h.(map[string]any); ok {
				if cmd, ok := hm["command"].(string); ok && isHearthNotifyCmd(cmd, agent) {
					return true
				}
			}
		}
	}
	return false
}
