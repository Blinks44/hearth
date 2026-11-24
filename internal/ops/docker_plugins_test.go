package ops

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDockerRemoveBrokenPluginPlan(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires developer mode or elevation on Windows")
	}
	configDir := filepath.Join(t.TempDir(), ".docker")
	pluginDir := filepath.Join(configDir, "cli-plugins")
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"DOCKER_CONFIG=" + configDir}

	broken := filepath.Join(pluginDir, "docker-compose")
	missingTarget := filepath.Join(t.TempDir(), "missing-compose")
	if err := os.Symlink(missingTarget, broken); err != nil {
		t.Fatal(err)
	}

	realFile := filepath.Join(pluginDir, "docker-buildx")
	if err := os.WriteFile(realFile, []byte("bin"), 0o700); err != nil {
		t.Fatal(err)
	}

	validLink := filepath.Join(pluginDir, "docker-valid")
	if err := os.Symlink(realFile, validLink); err != nil {
		t.Fatal(err)
	}

	otherLink := filepath.Join(pluginDir, "compose-link")
	if err := os.Symlink(missingTarget, otherLink); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name      string
		dir, file string
	}{
		{"regular file", pluginDir, "docker-buildx"},
		{"working symlink", pluginDir, "docker-valid"},
		{"non-plugin prefix", pluginDir, "compose-link"},
		{"missing entry", pluginDir, "docker-none"},
		{"path traversal", pluginDir, "../docker-compose"},
		{"wrong directory", configDir, "docker-compose"},
	} {
		if _, err := DockerRemoveBrokenPluginPlan(env, tc.dir, tc.file, nil); err == nil {
			t.Errorf("%s: unsafe broken plugin removal was accepted", tc.name)
		}
	}

	remaining := []string{"docker-compose"}
	detect := func(context.Context) (inventory.ToolState, error) {
		return inventory.ToolState{Docker: &inventory.DockerState{BrokenPlugins: remaining}}, nil
	}
	p, err := DockerRemoveBrokenPluginPlan(env, pluginDir, "docker-compose", detect)
	if err != nil {
		t.Fatalf("DockerRemoveBrokenPluginPlan: %v", err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Remove != broken {
		t.Fatalf("steps = %+v, want Remove %q", p.Steps, broken)
	}
	if err := p.Before(context.Background()); err != nil {
		t.Fatalf("Before failed on dangling symlink: %v", err)
	}
	if _, err := p.Verify(context.Background()); err == nil {
		t.Fatal("Verify passed before symlink was removed")
	}

	// If the target appears before execution, Before must refuse the stale plan.
	if err := os.WriteFile(missingTarget, []byte("restored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.Before(context.Background()); err == nil {
		t.Fatal("Before accepted symlink after its target reappeared")
	}
	if err := os.Remove(missingTarget); err != nil {
		t.Fatal(err)
	}
	if err := p.Before(context.Background()); err != nil {
		t.Fatalf("Before failed after target disappeared again: %v", err)
	}

	if err := removeAll(p.Steps[0].Remove); err != nil {
		t.Fatalf("removeAll: %v", err)
	}
	// Detect still reporting it fails Verify.
	if _, err := p.Verify(context.Background()); err == nil {
		t.Fatal("Verify passed while detect still reported the plugin broken")
	}
	remaining = nil
	if _, err := p.Verify(context.Background()); err != nil {
		t.Fatalf("Verify after removal: %v", err)
	}
	// Unrelated files remain untouched.
	if _, err := os.Stat(realFile); err != nil {
		t.Errorf("real plugin file was affected: %v", err)
	}
}
