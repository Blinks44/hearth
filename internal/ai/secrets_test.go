package ai

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestPlainSecret(t *testing.T) {
	tests := []struct {
		name, value string
		want        bool
	}{
		{"GITHUB_TOKEN", "ghp_abc123", true},
		{"X", "sk-abc", true}, // a token by its prefix, whatever its name
		{"Authorization", "Bearer s3cr3tvalue", true},
		{"OPENAI_KEY", "abcdef12", true},
		{"DB_PASSWORD", "hunter22", true},
		{"GITHUB_TOKEN", "${GITHUB_TOKEN}", false},
		{"GITHUB_TOKEN", "$GITHUB_TOKEN", false},
		{"Authorization", "Bearer ${env:LINEAR_TOKEN}", false},
		{"API_KEY", "{env:API_KEY}", false},
		{"API_KEY", "%API_KEY%", false},
		{"AUTH_FILE", "/home/me/.auth", false},
		{"AUTH_URL", "https://auth.example.com", false},
		{"USE_AUTH", "true", false},
		{"API_KEY", "short", false},
		{"NODE_ENV", "production", false},
		{"MONKEY", "bananas1", false},
	}
	for _, tt := range tests {
		if got := plainSecret(tt.name, tt.value); got != tt.want {
			t.Errorf("plainSecret(%q, %q) = %v, want %v", tt.name, tt.value, got, tt.want)
		}
	}
}

func TestURLSecret(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://user:s3cr3t@example.com/mcp", true},
		{"https://example.com/mcp?api_key=abcdef12", true},
		{"https://example.com/mcp?team=core&page=2", false},
		{"https://example.com/mcp?api_key=${API_KEY}", false},
		{"https://user@example.com/mcp", false},
	}
	for _, tt := range tests {
		if got := urlSecret(tt.url); got != tt.want {
			t.Errorf("urlSecret(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

// Claude Code's settings set an API key in "env"; a variable taken from
// the environment and the settings that are no secret do not count.
func TestDetectSettingsSecrets(t *testing.T) {
	home := fixtureHome(t)
	term := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + t.TempDir()}
	claude := byID(Detect(t.Context(), &run.Fake{}, term, nil))["claude"]
	if want := filepath.Join(home, ".claude", "settings.json"); claude.Settings != want {
		t.Errorf("Settings %q, want %q", claude.Settings, want)
	}
	if want := []string{"ANTHROPIC_AUTH_TOKEN"}; !reflect.DeepEqual(claude.PlainEnv, want) {
		t.Errorf("PlainEnv %q, want %q", claude.PlainEnv, want)
	}
}

func TestExpandVars(t *testing.T) {
	env := sysenv.Env{"TOKEN=abc"}
	for in, want := range map[string]string{
		"${TOKEN}":             "abc",
		"Bearer ${env:TOKEN}":  "Bearer abc",
		"{env:TOKEN}":          "abc",
		"${MISSING:-fallback}": "fallback",
		"$TOKEN":               "$TOKEN",
	} {
		if got := expandVars(in, env); got != want {
			t.Errorf("expandVars(%q) = %q, want %q", in, got, want)
		}
	}
}
