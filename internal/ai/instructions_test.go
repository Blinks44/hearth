package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestCodexInstructions(t *testing.T) {
	for _, tc := range []struct{ name, shared, native, override, state, active string }{
		{name: "missing", state: "missing"},
		{name: "shared only", shared: "Common", state: "shared"},
		{name: "native only", native: "Native", state: "native", active: "AGENTS.md"},
		{name: "independent same", shared: "Common", native: "Common", state: "same", active: "AGENTS.md"},
		{name: "different", shared: "Common", native: "Native", state: "different", active: "AGENTS.md"},
		{name: "override wins", shared: "Common", native: "Native", override: "Override", state: "override", active: "AGENTS.override.md"},
		{name: "empty override falls back", native: "Native", override: " \r\n", state: "native", active: "AGENTS.md"},
		{name: "empty native protected", shared: "Common", native: " \n", state: "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			custom := filepath.Join(home, "custom-codex")
			env := sysenv.Env{"HOME=" + home, "CODEX_HOME=" + custom}
			files := map[string]string{filepath.Join(home, ".agents/AGENTS.md"): tc.shared, filepath.Join(custom, "AGENTS.md"): tc.native, filepath.Join(custom, "AGENTS.override.md"): tc.override}
			for p, txt := range files {
				if txt != "" {
					if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, []byte(txt), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			got := CodexInstructions(env)
			if got.State != tc.state {
				t.Fatalf("state %s, want %s: %+v", got.State, tc.state, got)
			}
			want := ""
			if tc.active != "" {
				want = filepath.Join(custom, tc.active)
			}
			if got.Active != want {
				t.Fatalf("active %q, want %q", got.Active, want)
			}
			// No contents are persisted, even for instruction files that contain secrets.
			ag := byID(Detect(t.Context(), &run.Fake{}, env, nil))["codex"]
			if ag.Instructions == nil || *ag.Instructions != got {
				t.Fatal("scan lost global instructions")
			}
		})
	}
}

func TestCodexInstructionLinksAndReadErrors(t *testing.T) {
	home := t.TempDir()
	env := sysenv.Env{"USERPROFILE=" + home}
	shared := filepath.Join(home, ".agents/AGENTS.md")
	native := filepath.Join(home, ".codex/AGENTS.md")
	for _, p := range []string{shared, native} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	const secret = "private-instruction-token-12345"
	if err := os.WriteFile(shared, []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, native); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	got := CodexInstructions(env)
	if got.State != "linked" || got.Active != native {
		t.Fatalf("link: %+v", got)
	}
	data, _ := json.Marshal(got)
	if strings.Contains(string(data), secret) {
		t.Fatal("instruction text leaked into inventory")
	}
	if err := os.Remove(shared); err != nil {
		t.Fatal(err)
	}
	got = CodexInstructions(env)
	if got.State != "unreadable" || !got.Native.Exists || !got.Native.Link {
		t.Fatalf("broken link treated as absent: %+v", got)
	}
}

func TestInstructionText(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"large", []byte(strings.Repeat("x", 256*1024+1))}, {"invalid UTF8", []byte{255, 254}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "AGENTS.md")
			if err := os.WriteFile(p, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := InstructionText(p); err == nil {
				t.Fatal("accepted unreadable instructions")
			}
		})
	}
	if _, err := InstructionText(t.TempDir()); err == nil {
		t.Fatal("read directory")
	}
}
