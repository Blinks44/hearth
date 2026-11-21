//go:build integration

package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// TestGcloudSwitchIntegration exercises real gcloud configuration activation
// and project update inside an isolated CLOUDSDK_CONFIG directory without
// contacting Google Cloud.
func TestGcloudSwitchIntegration(t *testing.T) {
	baseEnv := sysenv.GUI()
	paths := sysenv.LookPathAll("gcloud", baseEnv)
	if len(paths) == 0 {
		t.Skip("gcloud not installed")
	}
	dir := t.TempDir()
	cfgs := filepath.Join(dir, "configurations")
	if err := os.MkdirAll(cfgs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "active_config"), []byte("default\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgs, "config_default"), []byte("[core]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgs, "config_staging"), []byte("[core]\nproject = old-proj\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var env sysenv.Env
	for _, kv := range baseEnv {
		if strings.HasPrefix(kv, "CLOUDSDK_CONFIG=") ||
			strings.HasPrefix(kv, "CLOUDSDK_ACTIVE_CONFIG_NAME=") ||
			strings.HasPrefix(kv, "CLOUDSDK_CORE_PROJECT=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "CLOUDSDK_CONFIG="+dir)
	p, err := GcloudSwitchPlan(paths[0], "default", "staging", "old-proj", "acme-stage", env, run.Exec{})
	if err != nil {
		t.Fatal(err)
	}
	var journal strings.Builder
	got := Execute(t.Context(), run.Exec{}, p, &journal, func(State) {})
	if got.Phase != Succeeded {
		t.Fatalf("result: %+v, journal:\n%s", got, journal.String())
	}
	active, err := os.ReadFile(filepath.Join(dir, "active_config"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(active)) != "staging" {
		t.Fatalf("active_config = %q, want staging", active)
	}
	cfgText, err := os.ReadFile(filepath.Join(cfgs, "config_staging"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfgText), "project = acme-stage") {
		t.Fatalf("config_staging missing updated project:\n%s", cfgText)
	}
}
