package detect

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// writeSized writes a file of n bytes.
func writeSized(t *testing.T, path string, n int) {
	t.Helper()
	writeFile(t, path, string(make([]byte, n)))
}

// Ollama's models by their manifests: names as ollama list says them, a
// blob two share freeing nothing alone, format and quantization from the
// config; LM Studio's a folder each under a publisher's.
func TestModels(t *testing.T) {
	orig := ollamaServiceDirs
	ollamaServiceDirs = nil
	t.Cleanup(func() { ollamaServiceDirs = orig })
	home := root(t)
	models := filepath.Join(home, ".ollama", "models")
	lib := filepath.Join(models, "manifests", "registry.ollama.ai", "library")
	writeFile(t, filepath.Join(lib, "llama3.2", "3b"), `{"config":{"digest":"sha256:c1","size":10},"layers":[{"digest":"sha256:w1","size":2000},{"digest":"sha256:lic","size":5}]}`)
	writeFile(t, filepath.Join(lib, "llama3.2", "1b"), `{"config":{"digest":"sha256:c2","size":10},"layers":[{"digest":"sha256:w2","size":1000},{"digest":"sha256:lic","size":5}]}`)
	writeFile(t, filepath.Join(models, "manifests", "registry.ollama.ai", "ann", "tiny", "latest"), `{"config":{"digest":"sha256:c3","size":1},"layers":[]}`)
	writeFile(t, filepath.Join(models, "manifests", "hf.co", "org", "repo", "Q8_0"), `not json`)
	writeFile(t, filepath.Join(models, "blobs", "sha256-c1"), `{"model_format":"gguf","file_type":"Q4_K_M"}`)
	day := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(lib, "llama3.2", "3b"), day, day); err != nil {
		t.Fatal(err)
	}
	lms := filepath.Join(home, ".lmstudio", "models")
	writeSized(t, filepath.Join(lms, "lmstudio-community", "Qwen2.5-Coder-14B-GGUF", "Qwen2.5-Coder-14B-Q4_K_M.gguf"), 300)
	writeSized(t, filepath.Join(lms, "lmstudio-community", "Qwen2.5-Coder-14B-GGUF", "mmproj-model-f16.gguf"), 20)
	writeSized(t, filepath.Join(lms, "mlx-community", "Mistral-MLX", "model.safetensors"), 100)
	writeSized(t, filepath.Join(lms, "mlx-community", "Mistral-MLX", "config.json"), 1)
	if err := os.MkdirAll(filepath.Join(lms, "mlx-community", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + filepath.Join(home, "bin")}
	st, err := Models(t.Context(), &run.Fake{}, env, env)
	if err != nil {
		t.Fatal(err)
	}
	o := st.Models.Ollama
	if o == nil || o.Dir != models || o.Port != "11434" || o.Err != "" {
		t.Fatalf("ollama: %+v", o)
	}
	got := map[string]string{}
	for _, m := range o.Models {
		got[m.Name] = m.Format + " " + m.Quant
	}
	want := map[string]string{"llama3.2:1b": " ", "llama3.2:3b": "GGUF Q4_K_M", "ann/tiny:latest": " "}
	if len(got) != len(want) {
		t.Fatalf("models %v", got)
	}
	for _, m := range o.Models {
		if got[m.Name] != want[m.Name] {
			t.Errorf("%s: %q, want %q", m.Name, got[m.Name], want[m.Name])
		}
		if m.Name == "llama3.2:3b" && (m.Size != 2015 || m.Frees != 2010 || !m.Downloaded.Equal(day)) {
			t.Errorf("3b: %+v", m)
		}
	}
	l := st.Models.LMStudio
	if l == nil || l.Dir != lms || len(l.Models) != 2 {
		t.Fatalf("lm studio: %+v", l)
	}
	if m := l.Models[0]; m.Name != "Qwen2.5-Coder-14B-GGUF" || m.Format != "GGUF" || m.Quant != "Q4_K_M" || m.Size != 320 || m.Frees != 320 {
		t.Errorf("gguf: %+v", m)
	}
	if m := l.Models[1]; m.Name != "Mistral-MLX" || m.Format != "MLX" || m.Quant != "" || m.Size != 101 {
		t.Errorf("mlx: %+v", m)
	}
}

// Neither app found: no state of either; OLLAMA_MODELS and OLLAMA_HOST
// move the models and the port.
func TestModelsElsewhere(t *testing.T) {
	// The service's folder of the machine running the test is no fixture.
	orig := ollamaServiceDirs
	ollamaServiceDirs = nil
	t.Cleanup(func() { ollamaServiceDirs = orig })
	home := root(t)
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home}
	st, _ := Models(t.Context(), &run.Fake{}, env, env)
	if st.Models.Ollama != nil || st.Models.LMStudio != nil {
		t.Errorf("found %+v", st.Models)
	}
	moved := filepath.Join(home, "big", "ollama")
	writeFile(t, filepath.Join(moved, "manifests", "registry.ollama.ai", "library", "qwen3", "8b"), `{"layers":[{"digest":"sha256:w","size":7}]}`)
	env = append(env, "OLLAMA_MODELS="+moved, "OLLAMA_HOST=0.0.0.0:11500")
	st, _ = Models(t.Context(), &run.Fake{}, env, env)
	o := st.Models.Ollama
	if o == nil || o.Dir != moved || o.DirVar != "OLLAMA_MODELS" || o.Port != "11500" || len(o.Models) != 1 || o.Models[0].Name != "qwen3:8b" {
		t.Errorf("ollama %+v", o)
	}
}

func TestOllamaLoaded(t *testing.T) {
	out := "NAME                 ID              SIZE     PROCESSOR    UNTIL\nqwen2.5-coder:32b    4bd6cbf2d094    21 GB    100% GPU     4 minutes from now\n"
	if got := ollamaLoaded(out); len(got) != 1 || got[0] != "qwen2.5-coder:32b" {
		t.Errorf("loaded %q", got)
	}
}

func TestLMStudioPort(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(home string)
		wantPort string
	}{
		{
			name: "modern http-server-config",
			setup: func(home string) {
				writeFile(t, filepath.Join(home, ".lmstudio", ".internal", "http-server-config.json"), `{"port":4321}`)
			},
			wantPort: "4321",
		},
		{
			name: "legacy http-server-config",
			setup: func(home string) {
				writeFile(t, filepath.Join(home, ".cache", "lm-studio", ".internal", "http-server-config.json"), `{"port":5555}`)
			},
			wantPort: "5555",
		},
		{
			name: "invalid port falls back to 1234",
			setup: func(home string) {
				writeFile(t, filepath.Join(home, ".lmstudio", ".internal", "http-server-config.json"), `{"port":99999}`)
			},
			wantPort: "1234",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := root(t)
			tt.setup(home)
			app := lmStudio(home)
			if app == nil || app.Port != tt.wantPort {
				t.Fatalf("lmStudio = %+v, want port %s", app, tt.wantPort)
			}
		})
	}
}
