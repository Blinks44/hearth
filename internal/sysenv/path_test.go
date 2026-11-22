package sysenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing")
	env := Env{"PATH=" + root + string(os.PathListSeparator) + missing + string(os.PathListSeparator) + root + string(os.PathListSeparator) + file}
	got := Paths(env)
	if len(got) != 4 || got[0].Missing || !got[1].Missing || got[2].Dir != root || !got[3].Missing {
		t.Fatalf("entries: %+v", got)
	}
}

func TestVariables(t *testing.T) {
	got := Variables(Env{
		"JAVA_HOME=/old",
		"JAVA_HOME=/jdk",
		"API_TOKEN=secret",
		"CUSTOM_SECRET=secret",
		"OLLAMA_MODELS=/models",
		"CLAUDE_CONFIG_DIR=/home/dev/.claude-work",
		"CODEX_HOME=/home/dev/.codex",
		"ANDROID_HOME=/sdk/android",
		"FLUTTER_ROOT=/sdk/flutter",
		"HTTP_PROXY=http://alice:supersecret@proxy.internal:8080",
		"HTTPS_PROXY=https://proxy.internal:8443",
	}, Env{
		"GOPATH=/go",
		"HTTP_PROXY=user:pass@proxy.local:3128",
	})
	want := map[string][2]string{
		"JAVA_HOME":         {"/jdk", ""},
		"GOPATH":            {"", "/go"},
		"OLLAMA_MODELS":     {"/models", ""},
		"CLAUDE_CONFIG_DIR": {"/home/dev/.claude-work", ""},
		"CODEX_HOME":        {"/home/dev/.codex", ""},
		"ANDROID_HOME":      {"/sdk/android", ""},
		"FLUTTER_ROOT":      {"/sdk/flutter", ""},
		"HTTP_PROXY":        {"http://•••@proxy.internal:8080", "•••@proxy.local:3128"},
		"HTTPS_PROXY":       {"https://proxy.internal:8443", ""},
	}
	if len(got) != len(want) {
		t.Fatalf("variables (%d): %+v", len(got), got)
	}
	for _, v := range got {
		pair, ok := want[v.Name]
		if !ok {
			t.Fatalf("unexpected variable %q = (%q, %q)", v.Name, v.Terminal, v.GUI)
		}
		if v.Terminal != pair[0] || v.GUI != pair[1] {
			t.Errorf("%s = (%q, %q), want (%q, %q)", v.Name, v.Terminal, v.GUI, pair[0], pair[1])
		}
	}
}

func TestSanitizeProxyCredentialsWithAtSign(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"http://alice:p@ss@proxy.internal:8080/path", "http://•••@proxy.internal:8080/path"},
		{"user:p@ss@proxy.local:3128", "•••@proxy.local:3128"},
		{"https://proxy.internal:8443/user@domain", "https://proxy.internal:8443/user@domain"},
	}
	for _, tt := range tests {
		if got := sanitizeProxy(tt.in); got != tt.want {
			t.Errorf("sanitizeProxy(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
