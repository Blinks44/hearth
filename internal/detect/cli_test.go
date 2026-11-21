package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestKubeContextsMerge(t *testing.T) {
	home := t.TempDir()
	first, second := filepath.Join(home, "first"), filepath.Join(home, "second")
	writeFile(t, first, `current-context: staging
contexts:
- name: production
  context:
    cluster: prod-first
    namespace: digital
- name: staging
  context:
    cluster: stage
users:
- name: private
  user:
    token: DO-NOT-RETAIN
- name: eks
  user:
    exec:
      command: aws
      args: [eks, get-token, --token, DO-NOT-RETAIN-ARG]
      env: [{name: SECRET, value: DO-NOT-RETAIN-ENV}]
`)
	writeFile(t, second, `current-context: production
contexts:
- name: production
  context:
    cluster: prod-second
- name: local
  context:
    cluster: local-cluster
    user: eks
users:
- name: eks
  user:
    exec:
      command: gke-gcloud-auth-plugin
`)
	broken, missing := filepath.Join(home, "broken"), filepath.Join(home, "missing")
	writeFile(t, broken, "contexts: [\n")
	sep := string(os.PathListSeparator)
	env := sysenv.Env{"KUBECONFIG=" + first + sep + second + sep + sep + first + sep + broken + sep + missing}
	kc := readKubeConfig(env, home)
	if kc.current == nil || kc.current.Name != "staging" || kc.current.File != first {
		t.Fatalf("current: %+v", kc.current)
	}
	// By file in merge order; a later context of a taken name is shadowed.
	want := []inventory.KubeContext{
		{Name: "production", Cluster: "prod-first", Namespace: "digital", File: first},
		{Name: "staging", Cluster: "stage", File: first},
		// The user's first definition, in the other file.
		{Name: "local", Cluster: "local-cluster", File: second, Auth: "aws"},
		{Name: "production", Cluster: "prod-second", File: second, Shadowed: true},
	}
	if !slices.Equal(kc.contexts, want) {
		t.Fatalf("contexts:\n got %+v\nwant %+v", kc.contexts, want)
	}
	files := []inventory.KubeFile{{Path: first}, {Path: second}, {Path: broken, Invalid: true}, {Path: missing, Missing: true}}
	if !slices.Equal(kc.files, files) {
		t.Fatalf("files: %+v", kc.files)
	}
	data, err := json.Marshal(inventory.CliState{KubeContexts: kc.contexts, KubeFiles: kc.files})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "DO-NOT-RETAIN") || strings.Contains(string(data), "get-token") {
		t.Fatal("credentials retained")
	}
	// Re-read after a terminal changes selection; no cached context survives.
	writeFile(t, first, "current-context: local\n")
	if current := kubeContext(env, home); current == nil || current.Name != "local" {
		t.Fatalf("stale current: %+v", current)
	}
	// A blank KUBECONFIG entry must not replace the default config.
	env = sysenv.Env{"KUBECONFIG=" + string(os.PathListSeparator)}
	if got := KubeConfigFiles(env, home); len(got) != 1 || got[0] != filepath.Join(home, ".kube", "config") {
		t.Fatalf("default files: %v", got)
	}
}

func TestCloudContextsPrecedenceAndStates(t *testing.T) {
	t.Run("aws env overrides, SSO, config path, and ambiguity", func(t *testing.T) {
		home := t.TempDir()
		customCfg := filepath.Join(home, "custom-aws-config")
		writeFile(t, customCfg, `
[default]
region = us-east-1

[profile staging]
region = eu-central-1
sso_session = corp
s3 =
    max_concurrent_requests = 20

[sso-session corp]
sso_start_url = https://corp.awsapps.com/start
`)
		writeFile(t, filepath.Join(home, ".aws", "credentials"), "[staging]\naws_secret_access_key = SUPER_SECRET_KEY\n")

		// Env overrides file and config path.
		env := sysenv.Env{
			"AWS_CONFIG_FILE=" + customCfg,
			"AWS_PROFILE=staging",
			"AWS_REGION=ap-southeast-1",
		}
		got := awsProfile(env, home)
		if got == nil {
			t.Fatal("expected aws context")
		}
		if got.Name != "staging" || got.NameSource != "AWS_PROFILE" || got.Region != "ap-southeast-1" || got.RegionSource != "AWS_REGION" {
			t.Fatalf("unexpected selection: %+v", got)
		}
		if got.ConfigEnv != "AWS_CONFIG_FILE" || got.File != customCfg || !got.HasSSO || got.Status != "" {
			t.Fatalf("unexpected metadata: %+v", got)
		}
		if len(got.Choices) != 2 || got.Choices[0].Name != "default" || got.Choices[1].Name != "staging" || !got.Choices[1].HasSSO {
			t.Fatalf("unexpected choices: %+v", got.Choices)
		}

		// AWS_ACCESS_KEY_ID marks ambiguous without retaining secret value.
		envCreds := append(slices.Clone(env), "AWS_ACCESS_KEY_ID=AKIAEXAMPLEKEY123456", "AWS_SECRET_ACCESS_KEY=DO_NOT_RETAIN_SECRET")
		amb := awsProfile(envCreds, home)
		if amb.Status != "ambiguous" || amb.Issue != "env_creds" {
			t.Fatalf("expected env_creds ambiguity: %+v", amb)
		}
		raw, _ := json.Marshal(amb)
		if strings.Contains(string(raw), "AKIAEXAMPLEKEY123456") || strings.Contains(string(raw), "DO_NOT_RETAIN_SECRET") || strings.Contains(string(raw), "SUPER_SECRET_KEY") {
			t.Fatalf("secret leaked in CliContext JSON: %s", raw)
		}

		// Conflicting AWS_PROFILE and AWS_DEFAULT_PROFILE.
		conflict := awsProfile(sysenv.Env{"AWS_CONFIG_FILE=" + customCfg, "AWS_PROFILE=staging", "AWS_DEFAULT_PROFILE=default"}, home)
		if conflict.Status != "ambiguous" || conflict.Issue != "profile_conflict" || conflict.Name != "staging" {
			t.Fatalf("expected profile_conflict: %+v", conflict)
		}

		// Missing named profile.
		missingProf := awsProfile(sysenv.Env{"AWS_CONFIG_FILE=" + customCfg, "AWS_PROFILE=ghost"}, home)
		if missingProf.Status != "missing" || missingProf.Issue != "profile_missing" {
			t.Fatalf("expected profile_missing: %+v", missingProf)
		}

		// Missing custom config file.
		missingFile := awsProfile(sysenv.Env{"AWS_CONFIG_FILE=" + filepath.Join(home, "nope")}, home)
		if missingFile.Status != "missing" || missingFile.Issue != "config_missing" {
			t.Fatalf("expected config_missing: %+v", missingFile)
		}

		// Invalid INI config file.
		broken := filepath.Join(home, "broken-aws")
		writeFile(t, broken, "[profile broken\nregion = us-east-1\n")
		inv := awsProfile(sysenv.Env{"AWS_CONFIG_FILE=" + broken}, home)
		if inv.Status != "invalid" || inv.Issue != "config_invalid" {
			t.Fatalf("expected config_invalid: %+v", inv)
		}
	})

	t.Run("gcloud config dir, active config, project override, and invalid states", func(t *testing.T) {
		home := t.TempDir()
		cfgDir := filepath.Join(home, "custom-gcloud")
		writeFile(t, filepath.Join(cfgDir, "active_config"), "default\n")
		writeFile(t, filepath.Join(cfgDir, "configurations", "config_default"), "[core]\nproject = file-proj\n")
		writeFile(t, filepath.Join(cfgDir, "configurations", "config_prod"), "[core]\nproject = prod-proj\n[compute]\nregion = europe-west1\n")

		// Env overrides both active config and project.
		env := sysenv.Env{
			"CLOUDSDK_CONFIG=" + cfgDir,
			"CLOUDSDK_ACTIVE_CONFIG_NAME=prod",
			"CLOUDSDK_CORE_PROJECT=override-proj",
		}
		got := gcloudProject(env, home)
		if got == nil {
			t.Fatal("expected gcloud context")
		}
		if got.Name != "prod" || got.NameSource != "CLOUDSDK_ACTIVE_CONFIG_NAME" {
			t.Fatalf("unexpected config name/source: %+v", got)
		}
		if got.Project != "override-proj" || got.ProjectSource != "CLOUDSDK_CORE_PROJECT" {
			t.Fatalf("unexpected project/source: %+v", got)
		}
		if got.Status != "ambiguous" || got.Issue != "project_override" || got.ConfigEnv != "CLOUDSDK_CONFIG" {
			t.Fatalf("expected project_override: %+v", got)
		}
		if len(got.Choices) != 2 || got.Choices[0].Name != "default" || got.Choices[1].Name != "prod" {
			t.Fatalf("unexpected choices: %+v", got.Choices)
		}

		// Missing active config file.
		missingCfg := gcloudProject(sysenv.Env{"CLOUDSDK_CONFIG=" + cfgDir, "CLOUDSDK_ACTIVE_CONFIG_NAME=absent"}, home)
		if missingCfg.Status != "missing" || missingCfg.Issue != "active_config_missing" {
			t.Fatalf("expected active_config_missing: %+v", missingCfg)
		}

		// Missing CLOUDSDK_CONFIG directory.
		missingDir := gcloudProject(sysenv.Env{"CLOUDSDK_CONFIG=" + filepath.Join(home, "no-such-gcloud")}, home)
		if missingDir.Status != "missing" || missingDir.Issue != "config_missing" {
			t.Fatalf("expected config_missing: %+v", missingDir)
		}

		// Invalid config INI.
		writeFile(t, filepath.Join(cfgDir, "configurations", "config_bad"), "[core\nproject = x\n")
		badCfg := gcloudProject(sysenv.Env{"CLOUDSDK_CONFIG=" + cfgDir, "CLOUDSDK_ACTIVE_CONFIG_NAME=bad"}, home)
		if badCfg.Status != "invalid" || badCfg.Issue != "config_invalid" {
			t.Fatalf("expected config_invalid: %+v", badCfg)
		}
	})

	t.Run("azure config dir, subscriptions, multiple defaults, and invalid JSON", func(t *testing.T) {
		home := t.TempDir()
		azDir := filepath.Join(home, "custom-azure")
		writeFile(t, filepath.Join(azDir, "azureProfile.json"), "\ufeff"+`{
			"subscriptions": [
				{"id": "sub-1", "name": "Dev", "state": "Enabled", "isDefault": true},
				{"id": "sub-2", "name": "Prod", "state": "Enabled", "isDefault": true}
			]
		}`)
		writeFile(t, filepath.Join(azDir, "accessTokens.json"), `[{"accessToken": "DO_NOT_READ_TOKEN"}]`)

		got := azSubscription(sysenv.Env{"AZURE_CONFIG_DIR=" + azDir}, home)
		if got == nil {
			t.Fatal("expected az context")
		}
		if got.Status != "ambiguous" || got.Issue != "multiple_defaults" || got.ConfigEnv != "AZURE_CONFIG_DIR" {
			t.Fatalf("expected multiple_defaults: %+v", got)
		}
		if len(got.Choices) != 2 || got.Name != "Dev" || got.SubscriptionID != "sub-1" {
			t.Fatalf("unexpected choices: %+v", got)
		}
		raw, _ := json.Marshal(got)
		if strings.Contains(string(raw), "DO_NOT_READ_TOKEN") {
			t.Fatal("azure token leaked")
		}

		// Invalid JSON.
		badDir := filepath.Join(home, "bad-azure")
		writeFile(t, filepath.Join(badDir, "azureProfile.json"), `{invalid`)
		bad := azSubscription(sysenv.Env{"AZURE_CONFIG_DIR=" + badDir}, home)
		if bad.Status != "invalid" || bad.Issue != "config_invalid" {
			t.Fatalf("expected config_invalid: %+v", bad)
		}

		// Missing custom AZURE_CONFIG_DIR.
		miss := azSubscription(sysenv.Env{"AZURE_CONFIG_DIR=" + filepath.Join(home, "missing-azure")}, home)
		if miss.Status != "missing" || miss.Issue != "config_missing" {
			t.Fatalf("expected config_missing: %+v", miss)
		}
	})
}
