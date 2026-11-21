//go:build integration

package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// TestCloudLiveDetectionAndAuth runs against real aws, gcloud, and tofu CLIs
// when present, strictly isolated to temporary config directories so real user
// configs and cloud accounts are never touched.
func TestCloudLiveDetectionAndAuth(t *testing.T) {
	baseEnv := sysenv.GUI()

	stripEnv := func(env sysenv.Env, prefixes ...string) sysenv.Env {
		var out sysenv.Env
		for _, kv := range env {
			skip := false
			for _, p := range prefixes {
				if len(kv) > len(p) && kv[:len(p)] == p && kv[len(p)] == '=' {
					skip = true
					break
				}
			}
			if !skip {
				out = append(out, kv)
			}
		}
		return out
	}

	t.Run("aws", func(t *testing.T) {
		paths := sysenv.LookPathAll("aws", baseEnv)
		if len(paths) == 0 {
			t.Skip("aws not installed")
		}
		dir := t.TempDir()
		cfg := filepath.Join(dir, "config")
		creds := filepath.Join(dir, "credentials")
		content := "[default]\nregion = us-east-1\n\n[profile staging]\nregion = eu-central-1\nsso_start_url = https://example.awsapps.com/start\n"
		if err := os.WriteFile(cfg, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		env := append(stripEnv(baseEnv,
			"AWS_CONFIG_FILE", "AWS_SHARED_CREDENTIALS_FILE", "AWS_PROFILE", "AWS_DEFAULT_PROFILE",
			"AWS_REGION", "AWS_DEFAULT_REGION", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		),
			"AWS_CONFIG_FILE="+cfg,
			"AWS_SHARED_CREDENTIALS_FILE="+creds,
			"AWS_PROFILE=staging",
			"AWS_EC2_METADATA_DISABLED=true",
		)
		cc := awsProfile(env, dir)
		if cc == nil || cc.Name != "staging" || cc.NameSource != "AWS_PROFILE" || cc.Region != "eu-central-1" || !cc.HasSSO {
			t.Fatalf("unexpected aws context: %+v", cc)
		}
		auth := CheckCloudAuth(t.Context(), run.Exec{}, "aws", paths[0], env)
		t.Logf("live aws auth: state=%d detail=%q took=%v", auth.State, auth.Detail, auth.Took)
		if auth.State != CloudAuthNotLoggedIn && auth.State != CloudAuthExpired {
			t.Errorf("expected unauthenticated/expired state for isolated aws config, got %+v", auth)
		}
	})

	t.Run("gcloud", func(t *testing.T) {
		paths := sysenv.LookPathAll("gcloud", baseEnv)
		if len(paths) == 0 {
			t.Skip("gcloud not installed")
		}
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "configurations"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "active_config"), []byte("work\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "configurations", "config_work"), []byte("[core]\nproject = acme-stage\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		env := append(stripEnv(baseEnv, "CLOUDSDK_CONFIG", "CLOUDSDK_ACTIVE_CONFIG_NAME", "CLOUDSDK_CORE_PROJECT"),
			"CLOUDSDK_CONFIG="+dir,
		)
		cc := gcloudProject(env, dir)
		if cc == nil || cc.Name != "work" || cc.Project != "acme-stage" || cc.ConfigEnv != "CLOUDSDK_CONFIG" {
			t.Fatalf("unexpected gcloud context: %+v", cc)
		}
		auth := CheckCloudAuth(t.Context(), run.Exec{}, "gcloud", paths[0], env)
		t.Logf("live gcloud auth: state=%d detail=%q took=%v", auth.State, auth.Detail, auth.Took)
		if auth.State != CloudAuthNotLoggedIn {
			t.Errorf("expected CloudAuthNotLoggedIn for empty CLOUDSDK_CONFIG, got %+v", auth)
		}
	})

	t.Run("tofu", func(t *testing.T) {
		if len(sysenv.LookPathAll("tofu", baseEnv)) == 0 {
			t.Skip("tofu not installed")
		}
		ts, err := Cli(t.Context(), run.Exec{}, baseEnv, baseEnv)
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		if ts.Cli != nil {
			for _, in := range ts.Cli.Tools {
				if in.Tool == "tofu" && in.Version != "" {
					t.Logf("live tofu detected: version=%q path=%q source=%+v", in.Version, in.Path, in.Source)
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("Cli did not detect installed tofu: %+v", ts.Cli)
		}
	})
}
