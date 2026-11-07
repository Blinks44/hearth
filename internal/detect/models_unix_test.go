//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ollama on PATH: its version from the warning it gives without the
// server, and the models ollama ps says are in memory.
func TestOllamaCommand(t *testing.T) {
	home := root(t)
	bin := filepath.Join(home, "bin")
	exe := fakeExe(t, filepath.Join(bin, "ollama"))
	writeFile(t, filepath.Join(home, ".ollama", "models", "manifests", "registry.ollama.ai", "library", "llama3.2", "3b"), `{"layers":[]}`)
	r := &run.Fake{}
	r.Set(run.Result{Stderr: []byte("Warning: could not connect to a running Ollama instance\nWarning: client version is 0.5.4\n")}, nil, exe, "--version")
	r.Stdout("NAME          ID    SIZE   PROCESSOR  UNTIL\nllama3.2:3b   a80c  3 GB   100% GPU   4 minutes from now\n", exe, "ps")
	env := sysenv.Env{"HOME=" + home, "PATH=" + bin}
	st, err := Models(t.Context(), r, env, env)
	if err != nil {
		t.Fatal(err)
	}
	o := st.Models.Ollama
	if o == nil || o.Path != exe || o.Version != "0.5.4" || len(o.Models) != 1 || !o.Models[0].Loaded {
		t.Fatalf("ollama %+v", o)
	}
}

// A models folder the user may not read, as the Linux service's: the
// models come from ollama list, sized as it says.
func TestOllamaUnreadable(t *testing.T) {
	home := root(t)
	bin := filepath.Join(home, "bin")
	exe := fakeExe(t, filepath.Join(bin, "ollama"))
	locked := filepath.Join(home, "service")
	moved := filepath.Join(locked, "models")
	writeFile(t, filepath.Join(moved, "manifests", "registry.ollama.ai", "library", "x", "1"), `{}`)
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("running as root: nothing is unreadable")
	}
	r := &run.Fake{}
	r.Stdout("NAME                 ID              SIZE      MODIFIED\nall-minilm:latest    1b226e2802db    45 MB     2 minutes ago\nllama3.2:3b          a80c4f17acd5    2.0 GB    5 months ago\n", exe, "list")
	env := sysenv.Env{"HOME=" + home, "PATH=" + bin, "OLLAMA_MODELS=" + moved}
	st, _ := Models(t.Context(), r, env, env)
	o := st.Models.Ollama
	if o == nil || o.Err != "" || len(o.Models) != 2 || o.Models[0].Size != 45_000_000 || o.Models[1].Size != 2_000_000_000 || !o.Models[1].Downloaded.IsZero() {
		t.Fatalf("ollama %+v", o)
	}
}
