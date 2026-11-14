package ops

import (
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestSecretMovable(t *testing.T) {
	tests := []struct {
		s    inventory.MCPServer
		want bool
	}{
		{inventory.MCPServer{Agent: "cursor", File: "/h/.cursor/mcp.json", PlainEnv: []string{"T"}}, true},
		{inventory.MCPServer{Agent: "codex", File: "/h/.codex/config.toml", PlainHeaders: []string{"Authorization"}}, true},
		{inventory.MCPServer{Agent: "claude", File: "/h/.claude.json", PlainEnv: []string{"T"}}, true},
		// claude mcp changes a project's server only in the project's folder.
		{inventory.MCPServer{Agent: "claude", File: "/h/.claude.json", Project: "/app", PlainEnv: []string{"T"}}, false},
		// Hearth does not know how Copilot CLI takes variables.
		{inventory.MCPServer{Agent: "copilot", File: "/h/.copilot/mcp-config.json", PlainEnv: []string{"T"}}, false},
		// A secret in the arguments only stays.
		{inventory.MCPServer{Agent: "cursor", File: "/h/.cursor/mcp.json", PlainArgs: true}, false},
	}
	for _, tt := range tests {
		if got := SecretMovable(tt.s); got != tt.want {
			t.Errorf("SecretMovable(%+v) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestVarName(t *testing.T) {
	for _, tt := range [][3]string{
		{"linear", "Authorization", "LINEAR_TOKEN"},
		{"my-api", "X-Api-Key", "MY_API_X_API_KEY"},
	} {
		if got := varName(tt[0], tt[1]); got != tt[2] {
			t.Errorf("varName(%q, %q) = %q, want %q", tt[0], tt[1], got, tt[2])
		}
	}
}

// Codex takes the env's secret by name, the bearer token from its own
// key, another header from env_http_headers; the server's other keys and
// the rest of the file stay.
func TestCodexRefs(t *testing.T) {
	text := `model = "gpt-5"

[mcp_servers.api]
url = "https://api.example.com/mcp"
enabled = false

[mcp_servers.api.http_headers]
Authorization = "Bearer s3cr3t-token"
X-Api-Key = "s3cr3t-key"
X-Team = "core"

[mcp_servers.api.env]
API_TOKEN = "s3cr3t-env"
LOG = "debug"

[profiles.fast]
model = "o4"
`
	s := inventory.MCPServer{Name: "api", Agent: "codex", PlainEnv: []string{"API_TOKEN"}, PlainHeaders: []string{"Authorization", "X-Api-Key"}}
	got, err := codexRefs(text, s, map[string]string{"Authorization": "API_TOKEN_B", "X-Api-Key": "API_X_API_KEY"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "s3cr3t") {
		t.Errorf("a secret stays:\n%s", got)
	}
	for _, want := range []string{
		`model = "gpt-5"`, `enabled = false`, `bearer_token_env_var = "API_TOKEN_B"`, `env_vars = ["API_TOKEN"]`,
		`X-Api-Key = "API_X_API_KEY"`, `X-Team = "core"`, `LOG = "debug"`, "\n\n[profiles.fast]\nmodel = \"o4\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in\n%s", want, got)
		}
	}
}
