package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

const fakeToken = "s3cret-token"

// TestMain makes the test binary a fake MCP server over stdio when
// HEARTH_FAKE_MCP is set: "ok" serves one tool if FAKE_TOKEN is right
// and exits with it in stderr if not, "hang" never answers.
func TestMain(m *testing.M) {
	switch os.Getenv("HEARTH_FAKE_MCP") {
	case "":
		os.Exit(m.Run())
	case "hang":
		time.Sleep(time.Hour)
	case "ok":
		if token := os.Getenv("FAKE_TOKEN"); token != fakeToken {
			fmt.Fprintln(os.Stderr, "Error: bad FAKE_TOKEN "+token)
			os.Exit(1)
		}
		_ = fakeServer().Run(context.Background(), &mcpsdk.StdioTransport{})
	}
	os.Exit(0)
}

func fakeServer() *mcpsdk.Server {
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fake", Version: "1.2.3"}, nil)
	type in struct {
		Text string `json:"text"`
	}
	mcpsdk.AddTool(s, &mcpsdk.Tool{Name: "echo", Description: "Echo the text.\nIt comes back as is."},
		func(context.Context, *mcpsdk.CallToolRequest, in) (*mcpsdk.CallToolResult, any, error) {
			return nil, nil, nil
		})
	return s
}

// declare writes server as the only one of a project's .mcp.json.
func declare(t *testing.T, server map[string]any) inventory.MCPServer {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	data, err := json.Marshal(map[string]any{"mcpServers": map[string]any{"fake": server}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	s := inventory.MCPServer{Name: "fake", Agent: "claude", File: path, Project: dir, Transport: "stdio"}
	if _, ok := server["url"]; ok {
		s.Transport = "http"
	}
	return s
}

func TestCheckMCP(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stdio := func(mode string) map[string]any {
		return map[string]any{"command": self, "env": map[string]any{"HEARTH_FAKE_MCP": mode, "FAKE_TOKEN": "${TOKEN}"}}
	}
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return fakeServer() }, nil)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeToken {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer web.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	http := func(url string) map[string]any {
		return map[string]any{"type": "http", "url": url, "headers": map[string]any{"Authorization": "Bearer ${TOKEN}"}}
	}

	tests := []struct {
		name   string
		server map[string]any
		token  string
		// fail is the failure; MCPFailed with an empty detail means none.
		fail   MCPFailure
		detail string
	}{
		{name: "stdio answers with the declared variable", server: stdio("ok"), token: fakeToken},
		{name: "stdio exits, its stderr told without the secret", server: stdio("ok"), token: "wrong-token",
			fail: MCPExited, detail: "Error: bad FAKE_TOKEN " + inventory.Hidden},
		{name: "command not found", server: map[string]any{"command": "no-such-mcp-server"}, fail: MCPNotFound, detail: "no-such-mcp-server"},
		{name: "http answers with the declared header", server: http(web.URL), token: fakeToken},
		{name: "http refuses a wrong token", server: http(web.URL), token: "wrong-token", fail: MCPDenied},
		{name: "nothing at the URL", server: http(closed.URL), fail: MCPUnreachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			term := sysenv.Env{"PATH=" + t.TempDir(), "TOKEN=" + tt.token}
			got, err := CheckMCP(t.Context(), declare(t, tt.server), term)
			if tt.fail == MCPFailed && tt.detail == "" {
				if err != nil {
					t.Fatal(err)
				}
				want := []MCPTool{{Name: "echo", Description: "Echo the text."}}
				if got.Server != "fake" || got.Version != "1.2.3" || fmt.Sprint(got.Tools) != fmt.Sprint(want) {
					t.Errorf("got %+v, want fake 1.2.3 with %+v", got, want)
				}
				return
			}
			var e *MCPError
			if !errors.As(err, &e) {
				t.Fatalf("err = %v, want an *MCPError", err)
			}
			if e.Kind != tt.fail || !strings.Contains(e.Detail, tt.detail) {
				t.Errorf("got %d %q, want %d with %q", e.Kind, e.Detail, tt.fail, tt.detail)
			}
			if strings.Contains(e.Detail, "wrong-token") {
				t.Errorf("detail %q shows the token", e.Detail)
			}
		})
	}
}

func TestCheckMCPTimesOut(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	s := declare(t, map[string]any{"command": self, "env": map[string]any{"HEARTH_FAKE_MCP": "hang"}})
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	_, err = CheckMCP(ctx, s, sysenv.Env{"PATH=" + t.TempDir()})
	if e, ok := errors.AsType[*MCPError](err); !ok || e.Kind != MCPTimedOut {
		t.Errorf("err = %v, want timed out", err)
	}
}

// Codex passes variables by name and sets headers from variables.
func TestReadMCPDefCodex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	text := `[mcp_servers.sentry]
url = "https://mcp.sentry.dev/mcp"
bearer_token_env_var = "SENTRY_TOKEN"
env_http_headers = { "X-Org" = "SENTRY_ORG" }
env_vars = ["HOME_DIR"]
`
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	def, err := ReadMCPDef(inventory.MCPServer{Name: "sentry", Agent: "codex", File: path, Transport: "http"})
	if err != nil {
		t.Fatal(err)
	}
	if def.URL != "https://mcp.sentry.dev/mcp" || def.BearerVar != "SENTRY_TOKEN" ||
		def.EnvHeaders["X-Org"] != "SENTRY_ORG" || fmt.Sprint(def.EnvVars) != "[HOME_DIR]" {
		t.Errorf("got %+v", def)
	}
}

func TestSecretRedactorURLAndArgs(t *testing.T) {
	secrets := append(urlSecrets("https://user:url-pass@example.test/mcp?api_key=query-secret-123&mode=fast"),
		argSecrets([]string{
			"--token=arg-secret-456",
			"--api-key", "arg-secret-789",
			"https://bot:arg-pass@host.test/x",
			"--dsn=postgres://dbuser:flag-dsn-pass@db.local/app",
			"--url=https://api.test/mcp?token=flag-query-secret",
			"Authorization: Bearer bare-header-tok",
			"--header=Authorization: Bearer flag-header-tok",
		})...)
	if isLikelySecretEnv("DEBUG", "false") {
		t.Fatal("DEBUG=false should not be treated as a secret")
	}
	if !isLikelySecretEnv("API_KEY", "env-secret-000") {
		t.Fatal("API_KEY should be treated as a secret")
	}
	secrets = append(secrets, "env-secret-000", stripAuthPrefix("bearer header-secret-999"))
	got := failure(t.Context(), errors.New("false url-pass query-secret-123 arg-secret-456 arg-secret-789 arg-pass flag-dsn-pass flag-query-secret bare-header-tok flag-header-tok env-secret-000 header-secret-999"), nil, &tail{}, 0, secrets).Detail
	for _, leaked := range []string{"url-pass", "query-secret-123", "arg-secret-456", "arg-secret-789", "arg-pass", "flag-dsn-pass", "flag-query-secret", "bare-header-tok", "flag-header-tok", "env-secret-000", "header-secret-999"} {
		if strings.Contains(got, leaked) {
			t.Errorf("secret %q leaked in %q", leaked, got)
		}
	}
	if !strings.HasPrefix(got, "false ") {
		t.Errorf("benign env value was redacted: %q", got)
	}
}
