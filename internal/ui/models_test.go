package ui

import (
	"path/filepath"
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

// "Download model…" asks for a name and proposes ollama pull, refusing
// a model Ollama has; a model's menu proposes ollama rm, noting the MCP
// server that asks for it.
func TestModelsOperations(t *testing.T) {
	a := NewApp()
	a.Home = home
	tt := ui.NewTester(a.View, 1280, 820)
	a.Snapshot = &inventory.Snapshot{Taken: tt.Now(), Tools: []inventory.ToolState{
		{Tool: "models", Models: &inventory.ModelsState{Ollama: &inventory.ModelApp{Path: "/usr/local/bin/ollama", Port: "11434", Models: []inventory.Model{
			{Name: "llama3.2:3b", Size: 2_000_000_000},
			{Name: "nomic-embed-text:latest", Size: 274_000_000},
		}}}},
	}, MCP: []inventory.MCPServer{{Name: "memory", Agent: "claude", Args: []string{"--embed-model", "nomic-embed-text"}}}}
	a.router.Push(modelsPage)
	tt.Advance(time.Second)
	if err := tt.Click("Download model…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Type("llama3.2:3b")
	tt.Advance(time.Second)
	if !tt.HasText("Ollama already has llama3.2:3b") {
		t.Errorf("texts %q", tt.Texts())
	}
	for range len("llama3.2:3b") {
		tt.Key(0, ui.KeyBackspace)
	}
	tt.Type("qwen3")
	tt.Advance(time.Second)
	screenshot(t, tt, "models-pull")
	if err := tt.Click("Next"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Download qwen3:latest" {
		t.Fatalf("pull plan %+v", a.op)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Key(0, ui.KeySlash)
	tt.Type("nomic")
	tt.Advance(time.Second)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Remove…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Remove nomic-embed-text:latest" {
		t.Fatalf("remove plan %+v", a.op)
	}
	hasTexts(t, tt, "ollama rm nomic-embed-text:latest", "MCP memory · Claude Code asks for it")
}

// Without Ollama's command there is nothing to download through.
func TestModelsNoOllamaCommand(t *testing.T) {
	a := NewApp()
	tt := ui.NewTester(a.View, 1280, 820)
	a.Snapshot = &inventory.Snapshot{Taken: tt.Now(), Tools: []inventory.ToolState{
		{Tool: "models", Models: &inventory.ModelsState{Ollama: &inventory.ModelApp{Port: "11434", Models: []inventory.Model{{Name: "llama3.2:3b"}}}}},
	}}
	a.router.Push(modelsPage)
	tt.Advance(time.Second)
	if tt.HasText("Download model…") || tt.HasText("More") {
		t.Error("offers operations without ollama")
	}
}

func TestModelsUnloadAndLMStudioRemove(t *testing.T) {
	dir := t.TempDir()
	lmDir := filepath.Join(dir, ".lmstudio", "models")
	lmModel := filepath.Join(lmDir, "lmstudio-community", "Mistral-Nemo-12B-GGUF")
	a := NewApp()
	a.Home = dir
	tt := ui.NewTester(a.View, 1280, 820)
	a.Snapshot = &inventory.Snapshot{Taken: tt.Now(), Tools: []inventory.ToolState{
		{Tool: "models", Models: &inventory.ModelsState{
			Ollama: &inventory.ModelApp{Path: "/usr/local/bin/ollama", Port: "11434", Models: []inventory.Model{
				{Name: "qwen2.5-coder:32b", Loaded: true, Size: 19_000_000_000},
			}},
			LMStudio: &inventory.ModelApp{Port: "4321", Dir: lmDir, Models: []inventory.Model{
				{Name: "Mistral-Nemo-12B-GGUF", Path: lmModel, Size: 7_500_000_000},
			}},
		}},
	}}
	a.router.Push(modelsPage)
	tt.Advance(time.Second)
	hasTexts(t, tt, "LM Studio · localhost:4321")
	tt.Key(0, ui.KeySlash)
	tt.Type("qwen2.5")
	tt.Advance(time.Second)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Unload from memory…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Unload qwen2.5-coder:32b from memory" {
		t.Fatalf("unload plan %+v", a.op)
	}
	hasTexts(t, tt, "ollama stop qwen2.5-coder:32b")
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	a.modelsFilter = ""
	if err := tt.Click("LM Studio"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Remove…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Remove Mistral-Nemo-12B-GGUF" {
		t.Fatalf("lm studio remove plan %+v", a.op)
	}
	hasTexts(t, tt, "remove "+tilde(lmModel, dir))
}

func TestModelsPolishControls(t *testing.T) {
	// Empty state with Homebrew offers Install Ollama…
	a := NewApp()
	scans := 0
	a.OnScan = func() { scans++ }
	tt := ui.NewTester(a.View, 1280, 820)
	a.Snapshot = &inventory.Snapshot{Taken: tt.Now(), Tools: []inventory.ToolState{
		{Tool: "models", Models: &inventory.ModelsState{}},
		{Tool: "packages", PackageManagers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew"}}},
	}}
	a.router.Push(modelsPage)
	tt.Advance(time.Second)
	hasTexts(t, tt, "Refresh", "Install Ollama…")
	if err := tt.Click("Refresh"); err != nil {
		t.Fatal(err)
	}
	if scans != 1 {
		t.Fatalf("scans = %d, want 1", scans)
	}
	a.Scanning = false
	if err := tt.Click("Install Ollama…"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Install Ollama" {
		t.Fatalf("install ollama plan: %+v", a.op)
	}
	a.op = nil

	// Without package manager, empty state falls back to Download Ollama.
	a.Snapshot.Tools[1].PackageManagers = nil
	tt.Frame()
	hasTexts(t, tt, "Download Ollama ↗")

	// With Ollama stopped: Start button, Copy model name button, and Nothing matches on filter miss.
	a.Snapshot.Tools[0].Models = &inventory.ModelsState{
		Ollama: &inventory.ModelApp{
			Path: "/opt/homebrew/bin/ollama", Port: "11434",
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"},
			Models: []inventory.Model{{Name: "llama3.2:3b", Size: 2_000_000_000}},
		},
	}
	tt.Frame()
	if _, ok := tt.Find("Copy model name"); !ok {
		t.Fatal("missing Copy model name button")
	}
	if err := tt.Click("Start service"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.op == nil || a.op.plan.Title != "Start Ollama" {
		t.Fatalf("start ollama plan: %+v", a.op)
	}
	a.op = nil
	a.modelsFilter = "nonexistent-model"
	tt.Frame()
	hasTexts(t, tt, "Nothing matches")
}
