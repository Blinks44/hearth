//go:build !windows

package detect

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// The command-line tools by their versions, terraform kept off its update
// check and gcloud not run; the contexts from the settings files only,
// never the credentials next to them.
func TestCli(t *testing.T) {
	dir := root(t)
	bin := filepath.Join(dir, "bin")
	kubectl := fakeExe(t, filepath.Join(bin, "kubectl"))
	helm := fakeExe(t, filepath.Join(bin, "helm"))
	terraform := fakeExe(t, filepath.Join(bin, "terraform"))
	aws := fakeExe(t, filepath.Join(bin, "aws"))
	az := fakeExe(t, filepath.Join(bin, "az"))
	sdk := filepath.Join(dir, "google-cloud-sdk")
	fakeExe(t, filepath.Join(sdk, "bin", "gcloud"))
	writeFile(t, filepath.Join(sdk, "VERSION"), "503.0.0\n")
	if err := os.Symlink(filepath.Join(sdk, "bin", "gcloud"), filepath.Join(bin, "gcloud")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".kube", "config"), "apiVersion: v1\ncurrent-context: kind-dev\nusers:\n- name: dev\n  user:\n    token: secret\n")
	writeFile(t, filepath.Join(helmConfigDir(nil, dir), "repositories.yaml"), "repositories:\n- name: bitnami\n- name: jetstack\n")
	writeFile(t, filepath.Join(dir, ".aws", "config"), "[default]\nregion = us-east-1\n\n[profile work]\nregion = eu-central-1\n")
	writeFile(t, filepath.Join(dir, ".aws", "credentials"), "[work]\naws_secret_access_key = secret\n")
	gcloud := filepath.Join(dir, ".config", "gcloud")
	writeFile(t, filepath.Join(gcloud, "active_config"), "work\n")
	writeFile(t, filepath.Join(gcloud, "configurations", "config_work"), "[core]\naccount = ann@example.com\nproject = shop-prod\n")
	writeFile(t, filepath.Join(dir, ".azure", "azureProfile.json"), "\ufeff"+`{"subscriptions":[{"name":"Dev","isDefault":false},{"name":"Prod","isDefault":true}]}`)
	r := &run.Fake{}
	r.Stdout("Client Version: v1.31.4\nKustomize Version: v5.4.2\n", kubectl, "version", "--client")
	r.Stdout("v3.16.3+gcfd0749\n", helm, "version", "--short")
	r.Stdout("Terraform v1.9.8\non darwin_arm64\n", terraform, "version")
	r.Stdout("aws-cli/2.22.1 Python/3.12.6 Darwin/24.1.0 exe/arm64\n", aws, "--version")
	r.Stdout(`{"azure-cli": "2.67.0", "azure-cli-core": "2.67.0"}`, az, "version", "--output", "json")
	env := sysenv.Env{"HOME=" + dir, "PATH=" + bin, "AWS_PROFILE=work"}
	st, err := Cli(t.Context(), r, env, sysenv.Env{"PATH=/usr/bin"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, in := range copiesIn(st.Cli.Tools, dir) {
		got[in.Tool] = in.Version
	}
	want := map[string]string{"kubectl": "1.31.4", "helm": "3.16.3", "terraform": "1.9.8", "aws": "2.22.1", "gcloud": "503.0.0", "az": "2.67.0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("versions = %v, want %v", got, want)
	}
	for _, c := range r.Calls() {
		if c.Args[0] == terraform && sysenv.Env(c.Env).Get("CHECKPOINT_DISABLE") != "1" {
			t.Error("terraform may check for updates online")
		}
		if filepath.Base(c.Args[0]) == "gcloud" {
			t.Error("gcloud was run")
		}
	}
	if st.Cli.HelmRepos != 2 {
		t.Errorf("helm repos = %d, want 2", st.Cli.HelmRepos)
	}
	wantContexts := []inventory.CliContext{
		{Tool: "kubectl", Name: "kind-dev", File: filepath.Join(dir, ".kube", "config")},
		{
			Tool: "aws", Name: "work", Region: "eu-central-1", File: filepath.Join(dir, ".aws", "config"),
			NameSource: "AWS_PROFILE", RegionSource: "file",
			Choices: []inventory.CliChoice{
				{Name: "default", Region: "us-east-1"},
				{Name: "work", Region: "eu-central-1"},
			},
		},
		{
			Tool: "gcloud", Name: "work", Project: "shop-prod", File: filepath.Join(gcloud, "configurations", "config_work"),
			NameSource: "file", ProjectSource: "file",
			Choices: []inventory.CliChoice{{Name: "work", Project: "shop-prod"}},
		},
		{
			Tool: "az", Name: "Prod", File: filepath.Join(dir, ".azure", "azureProfile.json"),
			NameSource: "file",
			Choices: []inventory.CliChoice{
				{Name: "Dev"},
				{Name: "Prod"},
			},
		},
	}
	if !reflect.DeepEqual(st.Cli.Contexts, wantContexts) {
		t.Errorf("contexts =\n %+v\nwant\n %+v", st.Cli.Contexts, wantContexts)
	}
}

// With no settings files, no contexts are made up: AWS's default profile
// only when its config has it, gcloud only when its folder is there.
func TestCliNoContexts(t *testing.T) {
	dir := root(t)
	st, err := Cli(t.Context(), &run.Fake{}, sysenv.Env{"HOME=" + dir, "PATH=" + filepath.Join(dir, "bin")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Cli.Tools) != 0 || len(st.Cli.Contexts) != 0 || st.Cli.HelmRepos != 0 {
		t.Errorf("found %+v", st.Cli)
	}
}

// A snap's commands all link to snapd: each is its snap's copy, gcloud's
// version from the snap's folder.
func TestCliSnap(t *testing.T) {
	dir := root(t)
	bin := filepath.Join(dir, "snap", "bin")
	snapd := fakeExe(t, filepath.Join(dir, "usr", "bin", "snap"))
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{"kubectl": snapd, "google-cloud-cli.gcloud": snapd, "gcloud": "google-cloud-cli.gcloud"} {
		if err := os.Symlink(target, filepath.Join(bin, link)); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, "snap", "google-cloud-cli", "current", "VERSION"), "592.0.0\n")
	old := snapBin
	snapBin = bin
	t.Cleanup(func() { snapBin = old })
	r := &run.Fake{}
	r.Stdout("Client Version: v1.35.10\n", filepath.Join(bin, "kubectl"), "version", "--client")
	st, err := Cli(t.Context(), r, sysenv.Env{"HOME=" + dir, "PATH=" + bin}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []inventory.Installation{
		{Tool: "kubectl", Version: "1.35.10", Path: filepath.Join(bin, "kubectl"), RealPath: filepath.Join(bin, "kubectl"), OnPath: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "snap", Root: filepath.Join(dir, "snap", "kubectl", "current")}},
		{Tool: "gcloud", Version: "592.0.0", Path: filepath.Join(bin, "gcloud"), RealPath: filepath.Join(bin, "gcloud"), OnPath: 1,
			Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "snap", Root: filepath.Join(dir, "snap", "google-cloud-cli", "current")}},
	}
	if !reflect.DeepEqual(st.Cli.Tools, want) {
		t.Errorf("tools = %+v, want %+v", st.Cli.Tools, want)
	}
}
