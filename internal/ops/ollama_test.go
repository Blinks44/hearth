package ops

import (
	"context"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestOllamaModelName(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"llama3.2", "llama3.2:latest", true},
		{" qwen3:8b ", "qwen3:8b", true},
		{"user/model", "user/model:latest", true},
		{"hf.co/bartowski/Llama-3.2-1B-Instruct-GGUF:Q4_K_M", "hf.co/bartowski/Llama-3.2-1B-Instruct-GGUF:Q4_K_M", true},
		{"localhost:5000/model", "localhost:5000/model:latest", true},
		{"", "", false},
		{"llama 3", "", false},
		{"--insecure", "", false},
	}
	for _, tt := range tests {
		got, ok := OllamaModelName(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%q: %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

// ollamaState is the models state with Ollama's models named.
func ollamaState(names ...string) Detect {
	return func(context.Context) (inventory.ToolState, error) {
		app := &inventory.ModelApp{}
		for _, n := range names {
			app.Models = append(app.Models, inventory.Model{Name: n})
		}
		return inventory.ToolState{Tool: "models", Models: &inventory.ModelsState{Ollama: app}}, nil
	}
}

// Downloading runs ollama pull on the full name and checks the model is
// there after.
func TestOllamaPullPlan(t *testing.T) {
	if _, ok := OllamaPullPlan(" ", nil); ok {
		t.Error("a plan for no name")
	}
	p, ok := OllamaPullPlan("qwen3", ollamaState("qwen3:latest"))
	if !ok || !slices.Equal(p.Steps[0].Argv, []string{"ollama", "pull", "qwen3:latest"}) || p.Steps[0].Timeout != ollamaPullTimeout {
		t.Fatalf("plan %+v", p)
	}
	if got, err := p.Verify(context.Background()); err != nil || got != "Ollama has qwen3:latest" {
		t.Errorf("found: %q, %v", got, err)
	}
	p, _ = OllamaPullPlan("qwen3", ollamaState("llama3.2:3b"))
	if _, err := p.Verify(context.Background()); err == nil || err.Error() != "Ollama does not have qwen3:latest yet" {
		t.Errorf("missing: %v", err)
	}
}

// Removing runs ollama rm, notes each MCP server that asks for the
// model, and checks it is gone after.
func TestOllamaRemovePlan(t *testing.T) {
	m := inventory.Model{Name: "nomic-embed-text:latest"}
	p := OllamaRemovePlan(m, []string{"memory · Claude Code"}, ollamaState("llama3.2:3b"))
	if !slices.Equal(p.Steps[0].Argv, []string{"ollama", "rm", "nomic-embed-text:latest"}) || len(p.Notes) != 1 || p.Notes[0].Title != "MCP memory · Claude Code asks for it" {
		t.Fatalf("plan %+v", p)
	}
	if got, err := p.Verify(context.Background()); err != nil || got != "nomic-embed-text:latest is removed" {
		t.Errorf("gone: %q, %v", got, err)
	}
	p = OllamaRemovePlan(m, nil, ollamaState("nomic-embed-text:latest"))
	if _, err := p.Verify(context.Background()); err == nil || err.Error() != "Ollama still has nomic-embed-text:latest" {
		t.Errorf("left: %v", err)
	}
}
