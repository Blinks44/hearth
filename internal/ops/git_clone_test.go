package ops

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestParseGitCloneURL(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantURL  string
		wantName string
		wantErr  bool
	}{
		{
			name:     "github https",
			raw:      "https://github.com/org/moduix",
			wantURL:  "https://github.com/org/moduix",
			wantName: "moduix",
		},
		{
			name:     "github https .git with trailing slash",
			raw:      "  https://github.com/org/moduix.git/  ",
			wantURL:  "https://github.com/org/moduix.git/",
			wantName: "moduix",
		},
		{
			name:     "github ssh scp",
			raw:      "git@github.com:org/moduix.git",
			wantURL:  "git@github.com:org/moduix.git",
			wantName: "moduix",
		},
		{
			name:     "gitlab nested group scp",
			raw:      "git@gitlab.company.internal:platform/backend/payments-api.git",
			wantURL:  "git@gitlab.company.internal:platform/backend/payments-api.git",
			wantName: "payments-api",
		},
		{
			name:     "ssh scheme with port",
			raw:      "ssh://git@git.example.com:2222/team/service.git",
			wantURL:  "ssh://git@git.example.com:2222/team/service.git",
			wantName: "service",
		},
		{
			name:     "bare host shorthand",
			raw:      "github.com/org/hearth",
			wantURL:  "https://github.com/org/hearth",
			wantName: "hearth",
		},
		{
			name:     "bare gitlab nested shorthand",
			raw:      "gitlab.com/group/subgroup/web-app.git",
			wantURL:  "https://gitlab.com/group/subgroup/web-app.git",
			wantName: "web-app",
		},
		{name: "empty", raw: "", wantErr: true},
		{name: "leading dash injection", raw: "--upload-pack=sh", wantErr: true},
		{name: "whitespace injection", raw: "https://github.com/org/repo --config core.sshCommand=id", wantErr: true},
		{name: "file scheme rejected", raw: "file:///tmp/repo", wantErr: true},
		{name: "ext helper rejected", raw: "ext::sh -c id", wantErr: true},
		{name: "local unix path rejected", raw: "/Users/dev/repo", wantErr: true},
		{name: "relative path rejected", raw: "./repo", wantErr: true},
		{name: "windows path rejected", raw: `C:\Users\dev\repo`, wantErr: true},
		{name: "host only without repo", raw: "https://github.com/", wantErr: true},
		{name: "scp empty path", raw: "git@github.com:", wantErr: true},
		{name: "scp absolute path", raw: "git@github.com:/org/repo", wantErr: true},
		{name: "dotdot repo name", raw: "https://github.com/org/..", wantErr: true},
		{name: "only .git suffix", raw: "https://github.com/org/.git", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotName, err := ParseGitCloneURL(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseGitCloneURL(%q) = (%q, %q, nil), want error", tc.raw, gotURL, gotName)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseGitCloneURL(%q) unexpected error: %v", tc.raw, err)
			}
			if gotURL != tc.wantURL || gotName != tc.wantName {
				t.Fatalf("ParseGitCloneURL(%q) = (%q, %q), want (%q, %q)", tc.raw, gotURL, gotName, tc.wantURL, tc.wantName)
			}
		})
	}
}

func TestValidateCloneFolderName(t *testing.T) {
	valid := []string{"hearth", "my-app_2", "v1.2"}
	for _, name := range valid {
		if err := ValidateCloneFolderName(name); err != nil {
			t.Errorf("ValidateCloneFolderName(%q) unexpected error: %v", name, err)
		}
	}
	invalid := []string{"", " ", " app", ".", "..", "a/b", `a\b`, "a:b", "app\x00"}
	for _, name := range invalid {
		if err := ValidateCloneFolderName(name); err == nil {
			t.Errorf("ValidateCloneFolderName(%q) = nil, want error", name)
		}
	}
}

func TestGitClonePlan(t *testing.T) {
	parent := t.TempDir()
	env := sysenv.Env{"HOME=" + parent, "PATH=/usr/bin"}

	plan, err := GitClonePlan("/opt/homebrew/bin/git", "git@github.com:org/moduix.git", parent, "", env)
	if err != nil {
		t.Fatalf("GitClonePlan error: %v", err)
	}
	wantTarget := filepath.Join(parent, "moduix")
	if len(plan.Steps) != 1 {
		t.Fatalf("got %d steps, want 1", len(plan.Steps))
	}
	wantArgv := []string{"/opt/homebrew/bin/git", "clone", "--", "git@github.com:org/moduix.git", wantTarget}
	if !slices.Equal(plan.Steps[0].Argv, wantArgv) {
		t.Fatalf("step argv = %v, want %v", plan.Steps[0].Argv, wantArgv)
	}
	if !slices.Contains(plan.Steps[0].Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("step env = %v, missing GIT_TERMINAL_PROMPT=0", plan.Steps[0].Env)
	}

	// Verify fails before .git exists, succeeds once .git is created.
	if _, err := plan.Verify(context.Background()); err == nil {
		t.Fatal("Verify succeeded before .git existed")
	}
	if err := os.MkdirAll(filepath.Join(wantTarget, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := plan.Verify(context.Background())
	if err != nil || !strings.Contains(got, "moduix") {
		t.Fatalf("Verify = (%q, %v), want message mentioning moduix", got, err)
	}

	// Now that destination exists, Before and a fresh plan both refuse it.
	if err := plan.Before(context.Background()); err == nil {
		t.Fatal("Before succeeded when destination already exists")
	}
	if _, err := GitClonePlan("/opt/homebrew/bin/git", "git@github.com:org/moduix.git", parent, "moduix", env); err == nil {
		t.Fatal("GitClonePlan succeeded when destination already exists")
	}
}

func TestGitCloneWithInstallPlan(t *testing.T) {
	parent := t.TempDir()
	env := sysenv.Env{"HOME=" + parent, "PATH=/custom/bin"}
	installPlan := Plan{
		Title: "Install Git",
		Env:   env,
		Steps: []Step{{Argv: []string{"brew", "install", "git"}}},
		Notes: []Note{{Title: "Note", Text: "Installs via Homebrew"}},
	}

	plan, err := GitCloneWithInstallPlan(installPlan, "https://github.com/org/demo.git", parent, "demo", env)
	if err != nil {
		t.Fatalf("GitCloneWithInstallPlan error: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("got %d steps, want 2", len(plan.Steps))
	}
	if !slices.Equal(plan.Parts, []int{0, 1}) {
		t.Fatalf("Parts = %v, want [0 1]", plan.Parts)
	}
	if !plan.RestartTerminal {
		t.Fatal("RestartTerminal = false, want true")
	}
	cloneStep := plan.Steps[1]
	wantTarget := filepath.Join(parent, "demo")
	wantArgv := []string{"git", "clone", "--", "https://github.com/org/demo.git", wantTarget}
	if !slices.Equal(cloneStep.Argv, wantArgv) {
		t.Fatalf("clone argv = %v, want %v", cloneStep.Argv, wantArgv)
	}
	if !slices.Contains(cloneStep.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("clone env = %v, missing GIT_TERMINAL_PROMPT=0", cloneStep.Env)
	}
	pathVal := sysenv.Env(cloneStep.Env).Get("PATH")
	if !strings.Contains(pathVal, "/custom/bin") {
		t.Fatalf("clone step PATH = %q, want existing /custom/bin preserved", pathVal)
	}

	// Execute with a runner stub: install succeeds, clone creates .git, verify passes.
	var seen [][]string
	r := gitCloneRunner(func(c run.Cmd) (run.Result, error) {
		seen = append(seen, c.Args)
		if len(c.Args) >= 2 && c.Args[1] == "clone" {
			if err := os.MkdirAll(filepath.Join(wantTarget, ".git"), 0o755); err != nil {
				return run.Result{}, err
			}
		}
		return run.Result{ExitCode: 0}, nil
	})
	st := Execute(context.Background(), r, plan, io.Discard, func(State) {})
	if st.Phase != Succeeded {
		t.Fatalf("Execute phase = %v (err=%q), want Succeeded", st.Phase, st.Err)
	}
	if len(seen) != 2 {
		t.Fatalf("runner saw %d commands, want 2", len(seen))
	}
}

type gitCloneRunner func(run.Cmd) (run.Result, error)

func (f gitCloneRunner) Run(_ context.Context, c run.Cmd) (run.Result, error) { return f(c) }
