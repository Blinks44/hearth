package versions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/@anthropic-ai/claude-code/latest":
			_, _ = w.Write([]byte(`{"version":"2.2.0"}`))
		case "/@openai/codex/latest":
			http.Error(w, "down", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	orig := agentRegistryURL
	t.Cleanup(func() { agentRegistryURL = orig })
	agentRegistryURL = srv.URL

	c := &Cache{Dir: t.TempDir()}
	got, err := FetchAgents(context.Background(), c, map[string]string{
		"claude": "@anthropic-ai/claude-code",
		"codex":  "@openai/codex",
	})
	if err == nil {
		t.Fatal("expected error from failing agent package")
	}
	if got.Latest["claude"].Version != "2.2.0" {
		t.Errorf("claude latest = %+v, want 2.2.0", got.Latest["claude"])
	}
	if _, ok := got.Latest["codex"]; ok {
		t.Errorf("unexpected codex version: %+v", got.Latest)
	}
	if got.Fetched.IsZero() {
		t.Error("expected Fetched timestamp from successful package")
	}
}
