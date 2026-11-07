package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
)

// The page shows Ollama's and LM Studio's models with their size, the one
// in memory and the one an MCP server asks for, whether the servers
// listen, and leads to the models in Cleanup.
func TestModelsPage(t *testing.T) {
	a := NewApp()
	a.Home = home
	const gb = 1_000_000_000
	tt := ui.NewTester(a.View, 1280, 820)
	now := tt.Now()
	a.Snapshot = &inventory.Snapshot{Taken: now, Tools: []inventory.ToolState{
		{Tool: "models", Models: &inventory.ModelsState{
			Ollama: &inventory.ModelApp{Path: "/opt/homebrew/bin/ollama", Version: "0.5.4", Port: "11434", Dir: home + "/.ollama/models",
				Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"},
				Models: []inventory.Model{
					{Name: "qwen2.5-coder:32b", Format: "GGUF", Quant: "Q4_K_M", Size: 19 * gb, Frees: 19 * gb, Loaded: true, Downloaded: now.Add(-24 * time.Hour)},
					{Name: "llama3.2:3b", Size: 2 * gb, Frees: 2 * gb, Downloaded: now.Add(-48 * time.Hour)},
					{Name: "nomic-embed-text:latest", Size: 274_000_000, Frees: 274_000_000, Downloaded: now.Add(-72 * time.Hour)},
				}},
			LMStudio: &inventory.ModelApp{Port: "1234", Dir: home + "/.lmstudio/models", Models: []inventory.Model{
				{Name: "Mistral-Nemo-12B-GGUF", Format: "GGUF", Size: 7_500_000_000, Frees: 7_500_000_000, Downloaded: now.Add(-96 * time.Hour)},
			}},
		}},
		{Tool: "databases", Databases: &inventory.DatabaseState{Listeners: []inventory.DatabaseListener{{Address: "127.0.0.1", Port: "11434"}}}},
	}, MCP: []inventory.MCPServer{{Name: "memory", Agent: "claude", Args: []string{"--embed-model", "nomic-embed-text"}}}}
	a.Sizes = &inventory.Sizes{Taken: now}
	a.router.Push(modelsPage)
	tt.Advance(time.Second)
	screenshot(t, tt, "models")
	hasTexts(t, tt, "AI · models on this computer", "Ollama and LM Studio: which models are downloaded and how much space they take",
		"in 2 apps", "28.8 GB", "running", "localhost:11434",
		"OLLAMA 0.5.4 · BREW", "qwen2.5-coder:32b", "in memory now · Q4_K_M", "19.0 GB",
		"nomic-embed-text:latest", "MCP memory of Claude Code asks for it",
		"LM STUDIO", "Mistral-Nemo-12B-GGUF", "GGUF",
		"Ollama · localhost:11434", "brew · running", "LM Studio · localhost:1234", "server off",
		"OLLAMA_MODELS", "not set · ~/.ollama/models",
		"MCP memory · Claude Code", "asks for nomic-embed-text:latest", "Open in Cleanup")
	if err := tt.Click("Open in Cleanup"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.router.Path() != cleanupPage || cleanupGroups[a.cleanupTab-1] != cleanup.Models {
		t.Fatalf("at %s, tab %d", a.router.Path(), a.cleanupTab)
	}
	screenshot(t, tt, "models-cleanup")
	hasTexts(t, tt, "Local models", "llama3.2:3b · Ollama", "Mistral-Nemo-12B-GGUF · LM Studio", "no agent's MCP server names it")
	if tt.HasText("qwen2.5-coder:32b · Ollama") || tt.HasText("nomic-embed-text:latest · Ollama") {
		t.Error("a model in memory or asked for is offered")
	}
}
