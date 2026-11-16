//go:build e2e

// Package e2e builds Hearth with the automation plugin, starts it on a
// home of its own with a fake node and a project, and walks its pages
// through the plugin's socket.
package e2e

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeVersion is the version the fake node says, which no real copy has.
const fakeVersion = "99.0.0"

func TestSmoke(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "Hearth")
	build := exec.Command("go", "build", "-tags", "e2e", "-o", app, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	home := filepath.Join(dir, "home")
	bin := filepath.Join(dir, "bin")
	project := filepath.Join(dir, "e2e-project")
	write(t, filepath.Join(bin, "node"), "#!/bin/sh\necho v"+fakeVersion+"\n", 0o755)
	write(t, filepath.Join(bin, "brew"), `#!/bin/sh
case "$1" in
  --version) echo 'Homebrew 0.0.0-e2e' ;;
  --prefix) echo /e2e/brew ;;
  info) echo '{"formulae":[{"name":"e2e-package","installed":[{"version":"1.0"}]}],"casks":[]}' ;;
  outdated) echo '{"formulae":[],"casks":[]}' ;;
  tap) echo e2e/tools ;;
esac
`, 0o755)
	write(t, filepath.Join(project, ".nvmrc"), "99\n", 0o644)
	// The settings in the home's user data list the project, as adding it
	// would; the folder dialog cannot be driven.
	t.Setenv("HOME", home)
	config, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal(map[string]any{"format": 1, "theme": "light", "versionLists": false,
		"projects": []map[string]string{{"path": project}}})
	write(t, filepath.Join(config, "Hearth", "settings.json"), string(settings), 0o644)

	// A short path: a Unix socket's path has a limit near 100 bytes.
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("hearth-e2e-%d.sock", os.Getpid()))
	cmd := exec.Command(app)
	cmd.Env = []string{
		"HOME=" + home, "SHELL=/bin/sh", "MYGO_LOCALE=en",
		// Exercise the app independently of native GPU frame scheduling.
		"MYGO_GPU=0",
		"PATH=" + bin + ":/usr/bin:/bin", "MYGO_AUTOMATION=" + sock,
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = os.Remove(sock)
	})

	c := dial(t, sock)
	c.waitFor(t, "Your whole developer environment on this computer")
	// The scan finds the fake node, and the project shows with it.
	c.waitFor(t, "e2e-project")
	// Standard manager locations can add probes with 15-second timeouts.
	c.waitForWithin(t, "Node.js", 90*time.Second)

	for _, step := range []struct{ key, text string }{
		{"/projects", "Code folders Hearth checks"},
		{"/tools/node", "v" + fakeVersion},
		{"/tools/docker", "CLI, contexts and container engines"},
		{"/tools/databases", "Database servers and containers"},
		{"/system/packages", "Package manager status"},
		{"/system/path", "Where terminals and apps look for commands"},
		{"/settings", "Local app · no account or Hearth telemetry"},
		{"/overview", "Needs attention"},
	} {
		c.call(t, "act", map[string]any{"key": step.key, "action": "press"})
		c.waitFor(t, step.text)
	}
	c.call(t, "act", map[string]any{"key": "open-search", "action": "press"})
	c.waitFor(t, "ACTIONS")
	c.call(t, "input", map[string]any{"events": []map[string]any{{"type": "text", "text": "node"}}})
	c.waitFor(t, "VERSIONS")
	c.call(t, "input", map[string]any{"events": []map[string]any{{"type": "key", "key": "Enter"}}})
	c.waitFor(t, "Available to install")
	c.call(t, "act", map[string]any{"key": "open-search", "action": "press"})
	c.waitFor(t, "RECENT")
	c.call(t, "input", map[string]any{"events": []map[string]any{{"type": "key", "key": "Escape"}}})
}

func write(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

// client speaks the automation plugin's protocol over its socket.
type client struct {
	conn net.Conn
	r    *bufio.Reader
	id   int
}

// dial connects to the app's socket once the app opened it.
func dial(t *testing.T, sock string) *client {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			return &client{conn: conn, r: bufio.NewReader(conn)}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the app opened no socket: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

type callError struct{ Code, Message string }

func (e *callError) Error() string { return e.Code + ": " + e.Message }

// try sends a request and returns its result or its error.
func (c *client) try(method string, params any) (json.RawMessage, error) {
	c.id++
	line, err := json.Marshal(map[string]any{"id": c.id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	answer, err := c.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var res struct {
		Result json.RawMessage
		Error  *callError
	}
	if err := json.Unmarshal(answer, &res); err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Result, nil
}

func (c *client) call(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	res, err := c.try(method, params)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	return res
}

// labels are the texts the window's elements show; none before the
// window opens.
func (c *client) labels(t *testing.T) []string {
	t.Helper()
	res, err := c.try("tree", map[string]any{})
	var ce *callError
	if errors.As(err, &ce) && ce.Code == "no_window" {
		return nil
	}
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	var tree struct {
		Nodes []struct{ Label, Value string }
	}
	if err := json.Unmarshal(res, &tree); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, n := range tree.Nodes {
		out = append(out, n.Label, n.Value)
	}
	return out
}

// waitFor waits until an element of the window shows text, as the scan
// and the pages' fades take their time.
func (c *client) waitFor(t *testing.T, text string) {
	t.Helper()
	c.waitForWithin(t, text, 30*time.Second)
}

func (c *client) waitForWithin(t *testing.T, text string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		// A window that keeps moving never settles; the tree is read anyway.
		_, _ = c.try("idle", map[string]any{"timeout": 2000})
		labels := c.labels(t)
		if slices.ContainsFunc(labels, func(l string) bool { return strings.Contains(l, text) }) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("no %q in %q", text, labels)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
