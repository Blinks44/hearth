package ops

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestAWSProfilePlan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix rc file test")
	}
	home := t.TempDir()
	rc := filepath.Join(home, ".zshrc")
	initial := "# user config\nexport FOO=bar\n# >>> hearth >>>\nexport AWS_PROFILE='old'\n# <<< hearth <<<\n"
	if err := os.WriteFile(rc, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh", "AWS_ACCESS_KEY_ID=AKIAEXAMPLE"}
	r := &run.Fake{}
	r.Set(run.Result{Stdout: []byte("eu-central-1\n")}, nil, "/usr/local/bin/aws", "configure", "get", "region", "--profile", "staging")

	p, err := AWSProfilePlan("/usr/local/bin/aws", "old", "staging", "eu-central-1", env, r)
	if err != nil {
		t.Fatalf("AWSProfilePlan: %v", err)
	}
	if !p.RestartTerminal {
		t.Error("expected RestartTerminal=true")
	}
	if len(p.Steps) != 2 || p.Steps[0].Edit == nil {
		t.Fatalf("Steps = %+v, want edit + configure set region", p.Steps)
	}
	if !strings.Contains(p.Steps[0].Edit.After, "export AWS_PROFILE='staging'") {
		t.Errorf("Edit.After = %q", p.Steps[0].Edit.After)
	}
	if strings.Contains(p.Steps[0].Edit.After, "old") {
		t.Errorf("old profile should be replaced: %q", p.Steps[0].Edit.After)
	}
	if len(p.Notes) != 1 || p.Notes[0].Tag != "AWS_ACCESS_KEY_ID" {
		t.Errorf("Notes = %+v, want AWS_ACCESS_KEY_ID note", p.Notes)
	}
	if got, err := p.Verify(t.Context()); err != nil || !strings.Contains(got, "eu-central-1") {
		t.Errorf("Verify = %q, %v", got, err)
	}

	// Switching to default removes AWS_PROFILE from Hearth's block and warns
	// if the user set AWS_PROFILE outside the block.
	if err := os.WriteFile(rc, []byte("export AWS_PROFILE=manual\n# >>> hearth >>>\nexport AWS_PROFILE='staging'\n# <<< hearth <<<\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p2, err := AWSProfilePlan("/usr/local/bin/aws", "staging", "default", "", sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh"}, r)
	if err != nil {
		t.Fatalf("AWSProfilePlan default: %v", err)
	}
	if strings.Contains(p2.Steps[0].Edit.After, "staging") || !strings.Contains(p2.Steps[0].Edit.After, "export AWS_PROFILE=manual") {
		t.Errorf("Edit.After = %q, should preserve user line and clear Hearth block", p2.Steps[0].Edit.After)
	}
	if len(p2.Notes) != 1 || p2.Notes[0].Tag != "AWS_PROFILE" {
		t.Errorf("Notes = %+v, want outside-block warning", p2.Notes)
	}

	if _, err := AWSProfilePlan("/usr/local/bin/aws", "default", "--evil", "", env, r); err == nil {
		t.Error("expected leading-dash profile to be rejected")
	}
}

func TestGcloudSwitchPlan(t *testing.T) {
	env := sysenv.Env{"CLOUDSDK_CORE_PROJECT=override-proj"}
	r := &run.Fake{}
	r.Set(run.Result{Stdout: []byte("work\n")}, nil, "/bin/gcloud", "config", "configurations", "list", "--filter=is_active:true", "--format=value(name)")
	r.Set(run.Result{Stdout: []byte("acme-prod\n")}, nil, "/bin/gcloud", "config", "get-value", "core/project")

	p, err := GcloudSwitchPlan("/bin/gcloud", "default", "work", "", "acme-prod", env, r)
	if err != nil {
		t.Fatalf("GcloudSwitchPlan: %v", err)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("Steps = %+v, want 2", p.Steps)
	}
	if len(p.Notes) != 1 || p.Notes[0].Tag != "CLOUDSDK_CORE_PROJECT" {
		t.Errorf("Notes = %+v, want CLOUDSDK_CORE_PROJECT override note", p.Notes)
	}
	if got, err := p.Verify(t.Context()); err != nil || !strings.Contains(got, "acme-prod") {
		t.Errorf("Verify = %q, %v", got, err)
	}
	if _, err := GcloudSwitchPlan("/bin/gcloud", "work", "work", "acme-prod", "acme-prod", env, r); err == nil {
		t.Error("expected no-op switch to return error")
	}
}

func TestAzSubscriptionPlan(t *testing.T) {
	r := &run.Fake{}
	r.Set(run.Result{Stdout: []byte("sub-123\n")}, nil, "/bin/az", "account", "show", "--query", "id", "--output", "tsv")

	p, err := AzSubscriptionPlan("/bin/az", "Old Sub", "sub-123", "Work Sub", nil, r)
	if err != nil {
		t.Fatalf("AzSubscriptionPlan: %v", err)
	}
	if len(p.Steps) != 1 || p.Steps[0].Argv[3] != "--subscription" || p.Steps[0].Argv[4] != "sub-123" {
		t.Fatalf("Steps = %+v", p.Steps)
	}
	if got, err := p.Verify(t.Context()); err != nil || !strings.Contains(got, "Work Sub") {
		t.Errorf("Verify = %q, %v", got, err)
	}
}

func TestCloudLoginPlan(t *testing.T) {
	r := &run.Fake{}
	r.Set(run.Result{Stdout: []byte(`{"Account":"123456789012","Arn":"arn:aws:sts::123456789012:assumed-role/Dev/user"}`)}, nil,
		"/bin/aws", "sts", "get-caller-identity", "--output", "json", "--no-cli-pager")

	if _, err := CloudLoginPlan("aws", "/bin/aws", "staging", false, nil, r); err == nil {
		t.Error("expected non-SSO AWS login to return guidance error")
	}

	p, err := CloudLoginPlan("aws", "/bin/aws", "staging", true, nil, r)
	if err != nil {
		t.Fatalf("CloudLoginPlan aws sso: %v", err)
	}
	if strings.Join(p.Steps[0].Argv, " ") != "/bin/aws sso login --profile staging" {
		t.Errorf("Argv = %v", p.Steps[0].Argv)
	}
	if got, err := p.Verify(t.Context()); err != nil || !strings.Contains(got, "Dev/user") {
		t.Errorf("Verify = %q, %v", got, err)
	}

	gp, err := CloudLoginPlan("gcloud", "/bin/gcloud", "", false, nil, nil)
	if err != nil || strings.Join(gp.Steps[0].Argv, " ") != "/bin/gcloud auth login" {
		t.Errorf("gcloud login = %+v, %v", gp, err)
	}
	ap, err := CloudLoginPlan("az", "/bin/az", "", false, nil, nil)
	if err != nil || strings.Join(ap.Steps[0].Argv, " ") != "/bin/az login --output none" {
		t.Errorf("az login = %+v, %v", ap, err)
	}
}

func TestInfraVersionPlanAndMiseSpec(t *testing.T) {
	specs := map[string]string{
		"":            "latest",
		"*":           "latest",
		"~> 1.10":     "1.10",
		"~> 1.10.2":   "1.10",
		">= 1.8":      "latest",
		">= 1.8, < 2": "1.8",
		"= 1.9.8":     "1.9.8",
		"1.10.5":      "1.10.5",
	}
	for want, exp := range specs {
		if got := MiseSpecForConstraint(want); got != exp {
			t.Errorf("MiseSpecForConstraint(%q) = %q, want %q", want, got, exp)
		}
	}

	setup := Setup{
		GOOS:  "darwin",
		Have:  map[string]bool{"mise": true},
		Title: func(id string) string { return id },
	}
	p, err := InfraVersionPlan("tofu", "OpenTofu", "~> 1.8", setup, nil, nil)
	if err != nil {
		t.Fatalf("InfraVersionPlan: %v", err)
	}
	if len(p.Steps) != 1 || strings.Join(p.Steps[0].Argv, " ") != "mise use --global opentofu@1.8" {
		t.Errorf("Steps = %+v", p.Steps)
	}

	setupNoMise := Setup{
		GOOS:     "darwin",
		Have:     map[string]bool{"homebrew": true, "terraform": true},
		Managers: []inventory.PackageManager{{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}},
		Title:    func(id string) string { return id },
	}
	p2, err := InfraVersionPlan("terraform", "Terraform", "~> 1.10", setupNoMise, sysenv.Env{"HOME=" + t.TempDir(), "SHELL=/bin/zsh"}, nil)
	if err != nil {
		t.Fatalf("InfraVersionPlan without mise: %v", err)
	}
	foundArg := false
	for _, s := range p2.Steps {
		if strings.Contains(strings.Join(s.Argv, " "), "terraform@1.10") {
			foundArg = true
		}
	}
	if !foundArg {
		t.Errorf("expected terraform@1.10 step in %+v", p2.Steps)
	}
}
