package ops

import (
	"context"
	"os"
	"path/filepath"
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

func TestOllamaStopPlan(t *testing.T) {
	m := inventory.Model{Name: "qwen2.5-coder:32b", Loaded: true}
	unloaded := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "models", Models: &inventory.ModelsState{
			Ollama: &inventory.ModelApp{Models: []inventory.Model{{Name: m.Name, Loaded: false}}},
		}}, nil
	}
	stillLoaded := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "models", Models: &inventory.ModelsState{
			Ollama: &inventory.ModelApp{Models: []inventory.Model{{Name: m.Name, Loaded: true}}},
		}}, nil
	}
	p := OllamaStopPlan(m, unloaded)
	if !slices.Equal(p.Steps[0].Argv, []string{"ollama", "stop", m.Name}) {
		t.Fatalf("plan %+v", p)
	}
	if got, err := p.Verify(context.Background()); err != nil || got != "qwen2.5-coder:32b is unloaded from memory" {
		t.Errorf("unloaded verify: %q, %v", got, err)
	}
	p = OllamaStopPlan(m, stillLoaded)
	if _, err := p.Verify(context.Background()); err == nil || err.Error() != "qwen2.5-coder:32b is still loaded in memory" {
		t.Errorf("still loaded verify: %v", err)
	}
}

func TestLMStudioRemovePlan(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if _, err := LMStudioRemovePlan(root, inventory.Model{Name: "bad", Path: "relative/path"}, nil, nil); err == nil {
		t.Fatal("accepted relative path")
	}
	if _, err := LMStudioRemovePlan(root, inventory.Model{Name: "root", Path: root}, nil, nil); err == nil {
		t.Fatal("accepted models root itself")
	}
	if _, err := LMStudioRemovePlan(root, inventory.Model{Name: "outside", Path: outside}, nil, nil); err == nil {
		t.Fatal("accepted path outside models root")
	}
	link := filepath.Join(root, "escape-link")
	if err := os.Symlink(outside, link); err == nil {
		pEsc, err := LMStudioRemovePlan(root, inventory.Model{Name: "escape", Path: link}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if pEsc.Before == nil || pEsc.Before(context.Background()) == nil {
			t.Fatal("Before accepted symlink escaping models root")
		}
	}
	dir := filepath.Join(root, "lmstudio-community", "Mistral-Nemo-12B-GGUF")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := inventory.Model{Name: "Mistral-Nemo-12B-GGUF", Path: dir}
	gone := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "models", Models: &inventory.ModelsState{
			LMStudio: &inventory.ModelApp{Dir: root},
		}}, nil
	}
	still := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "models", Models: &inventory.ModelsState{
			LMStudio: &inventory.ModelApp{Dir: root, Models: []inventory.Model{m}},
		}}, nil
	}
	p, err := LMStudioRemovePlan(root, m, []string{"local · Claude Code"}, gone)
	if err != nil {
		t.Fatal(err)
	}
	if p.Before == nil || p.Before(context.Background()) != nil {
		t.Fatalf("Before rejected valid model dir %s", dir)
	}
	if len(p.Steps) != 1 || p.Steps[0].Remove != dir || len(p.Notes) != 1 {
		t.Fatalf("plan %+v", p)
	}
	if got, err := p.Verify(context.Background()); err != nil || got != "Mistral-Nemo-12B-GGUF is removed" {
		t.Errorf("gone verify: %q, %v", got, err)
	}
	p, _ = LMStudioRemovePlan(root, m, nil, still)
	if _, err := p.Verify(context.Background()); err == nil || err.Error() != "LM Studio still has Mistral-Nemo-12B-GGUF" {
		t.Errorf("still verify: %v", err)
	}
}

func TestOllamaServerPlan(t *testing.T) {
	listening := func(up bool) Detect {
		return func(context.Context) (inventory.ToolState, error) {
			var listeners []inventory.DatabaseListener
			if up {
				listeners = []inventory.DatabaseListener{{Address: "127.0.0.1", Port: "11434"}}
			}
			return inventory.ToolState{Tool: "databases", Databases: &inventory.DatabaseState{Listeners: listeners}}, nil
		}
	}
	brewApp := inventory.ModelApp{Path: "/opt/homebrew/bin/ollama", Port: "11434", Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"}}
	p, ok := OllamaServerPlan(ServiceStart, brewApp, listening(true))
	if !ok || !slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "start", "ollama"}) {
		t.Fatalf("brew start plan: ok=%v %+v", ok, p)
	}
	if got, err := p.Verify(context.Background()); err != nil || got != "Ollama is listening on localhost:11434" {
		t.Errorf("start verify: %q, %v", got, err)
	}
	p, ok = OllamaServerPlan(ServiceStart, brewApp, listening(false))
	if !ok {
		t.Fatal("brew start plan not ok")
	}
	if _, err := p.Verify(context.Background()); err == nil {
		t.Error("expected verify error when not listening")
	}

	p, ok = OllamaServerPlan(ServiceStop, brewApp, listening(false))
	if !ok || !slices.Equal(p.Steps[0].Argv, []string{"brew", "services", "stop", "ollama"}) || len(p.Notes) != 1 {
		t.Fatalf("brew stop plan: ok=%v %+v", ok, p)
	}
	if got, err := p.Verify(context.Background()); err != nil || got != "Ollama stopped" {
		t.Errorf("stop verify: %q, %v", got, err)
	}
	p, _ = OllamaServerPlan(ServiceStop, brewApp, listening(true))
	if _, err := p.Verify(context.Background()); err == nil {
		t.Error("expected verify error when still listening")
	}

	appBundle := inventory.ModelApp{Path: "/Applications/Ollama.app/Contents/Resources/ollama", Port: "11434"}
	if p, ok := OllamaServerPlan(ServiceStart, appBundle, nil); !ok || len(p.Steps) != 1 || len(p.Steps[0].Argv) == 0 {
		t.Fatalf("platform start plan: ok=%v %+v", ok, p)
	}
}

func TestOllamaInstallPlan(t *testing.T) {
	if _, ok := OllamaInstallPlan(nil, nil); ok {
		t.Fatal("expected no install plan without supported package managers")
	}
	p, ok := OllamaInstallPlan([]string{"brew"}, ollamaState())
	if !ok || !slices.Equal(p.Steps[0].Argv, []string{"brew", "install", "ollama"}) {
		t.Fatalf("brew install plan: ok=%v %+v", ok, p)
	}
	if got, err := p.Verify(context.Background()); err != nil || got != "Ollama is installed" {
		t.Errorf("install verify: %q, %v", got, err)
	}
	missing := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Tool: "models", Models: &inventory.ModelsState{}}, nil
	}
	p, _ = OllamaInstallPlan([]string{"winget"}, missing)
	if len(p.Steps) != 1 || p.Steps[0].Argv[0] != "winget" {
		t.Fatalf("winget install plan: %+v", p)
	}
	if _, err := p.Verify(context.Background()); err == nil {
		t.Error("expected verify error when ollama missing")
	}
	p, ok = OllamaInstallPlan([]string{"pacman"}, nil)
	if !ok || !p.Steps[0].Admin || !slices.Equal(p.Steps[0].Argv, []string{"pacman", "-S", "--noconfirm", "ollama"}) {
		t.Fatalf("pacman install plan: ok=%v %+v", ok, p)
	}
}
