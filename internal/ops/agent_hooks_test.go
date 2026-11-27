package ops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanInstallAndRemoveAgentHooksPreservesUserHooks(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	initial := `{
  "theme": "dark",
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "echo user-hook"
          }
        ]
      }
    ]
  }
}
`
	if err := os.WriteFile(settingsPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	if HearthHooksInstalled("claude", home) {
		t.Fatal("expected hooks not installed yet")
	}

	plan, err := PlanInstallAgentHooks("claude", "Claude Code", home, "/Applications/Hearth.app/Contents/MacOS/Hearth")
	if err != nil {
		t.Fatalf("PlanInstallAgentHooks: %v", err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Edit == nil {
		t.Fatalf("unexpected plan steps: %+v", plan.Steps)
	}
	after := plan.Steps[0].Edit.After
	if !strings.Contains(after, "echo user-hook") {
		t.Fatalf("user hook was lost:\n%s", after)
	}
	if !strings.Contains(after, "notify --agent claude --event waiting") {
		t.Fatalf("missing hearth waiting hook:\n%s", after)
	}
	if !strings.Contains(after, `"theme": "dark"`) {
		t.Fatalf("existing theme setting lost:\n%s", after)
	}

	// Simulate applying the install edit, then test removal.
	if err := os.WriteFile(settingsPath, []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	if !HearthHooksInstalled("claude", home) {
		t.Fatal("expected hooks to be detected as installed")
	}

	rmPlan, err := PlanRemoveAgentHooks("claude", "Claude Code", home)
	if err != nil {
		t.Fatalf("PlanRemoveAgentHooks: %v", err)
	}
	rmAfter := rmPlan.Steps[0].Edit.After
	if strings.Contains(rmAfter, "notify --agent claude") {
		t.Fatalf("hearth hook still present after remove:\n%s", rmAfter)
	}
	if !strings.Contains(rmAfter, "echo user-hook") {
		t.Fatalf("user hook lost during remove:\n%s", rmAfter)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(rmAfter), &parsed); err != nil {
		t.Fatalf("invalid JSON after remove: %v", err)
	}
}

func TestPlanAgentHooksPreservesUnrelatedSecretsAndSharedWrapper(t *testing.T) {
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	// Non-canonical 4-space indentation + secret env block + user hook sharing wrapper with Hearth hook.
	initial := "{\n    \"env\": {\n        \"API_KEY\": \"sk-secret-value\"\n    },\n    \"hooks\": {\n        \"Stop\": [\n            {\n                \"matcher\": \"*\",\n                \"hooks\": [\n                    {\"type\": \"command\", \"command\": \"echo keep-me\"},\n                    {\"type\": \"command\", \"command\": \"hearth notify --agent claude --event finished\"}\n                ]\n            }\n        ]\n    }\n}\n"
	if err := os.WriteFile(settingsPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	rmPlan, err := PlanRemoveAgentHooks("claude", "Claude Code", home)
	if err != nil {
		t.Fatalf("PlanRemoveAgentHooks: %v", err)
	}
	edit := rmPlan.Steps[0].Edit
	for _, line := range edit.PreviewDiff() {
		if strings.Contains(line.Text, "sk-secret-value") {
			t.Fatalf("diff leaked unrelated secret in line %q", line.Text)
		}
	}
	if !strings.Contains(edit.After, "echo keep-me") {
		t.Fatalf("user hook inside shared wrapper was deleted:\n%s", edit.After)
	}
	if strings.Contains(edit.After, "notify --agent claude") {
		t.Fatalf("hearth hook not removed:\n%s", edit.After)
	}

	// Removing again on a non-canonically formatted file without Hearth hooks must report no changes.
	if err := os.WriteFile(settingsPath, []byte(edit.After), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanRemoveAgentHooks("claude", "Claude Code", home); err == nil {
		t.Fatal("expected no-changes error when Hearth hooks are already absent")
	}
}

func TestPlanInstallCursorHooksNewFile(t *testing.T) {
	home := t.TempDir()
	plan, err := PlanInstallAgentHooks("cursor", "Cursor", home, "/usr/local/bin/hearth")
	if err != nil {
		t.Fatalf("PlanInstallAgentHooks cursor: %v", err)
	}
	if !plan.Steps[0].Edit.New {
		t.Fatal("expected New=true for missing hooks.json")
	}
	if !strings.Contains(plan.Steps[0].Edit.After, `"stop"`) || !strings.Contains(plan.Steps[0].Edit.After, "notify --agent cursor --event finished") {
		t.Fatalf("unexpected cursor hooks.json:\n%s", plan.Steps[0].Edit.After)
	}
}
