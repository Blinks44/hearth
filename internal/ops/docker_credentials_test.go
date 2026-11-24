package ops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDockerCredentialPlan(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, config, want string
		issue              inventory.DockerCredentialIssue
	}{
		{"default store", `{"auths":{"registry.example":{"auth":"test-secret"}},"credsStore": "desktop"}`,
			`{"auths":{"registry.example":{"auth":"test-secret"}},"credsStore": "NEW"}`, inventory.DockerCredentialIssue{Name: "desktop"}},
		{"registry helper", "{\r\n  " + `"credHelpers": {"ghcr.io": "desktop", "other.example": "desktop"},"auths":{"ghcr.io":{"identitytoken":"test-secret"}}` + "\r\n}\r\n",
			"{\r\n  " + `"credHelpers": {"ghcr.io": "NEW", "other.example": "desktop"},"auths":{"ghcr.io":{"identitytoken":"test-secret"}}` + "\r\n}\r\n",
			inventory.DockerCredentialIssue{Name: "desktop", Registry: "ghcr.io", Key: "ghcr.io"}},
		{"stale observation", `{"credsStore":"other"}`, "", inventory.DockerCredentialIssue{Name: "desktop"}},
		{"ambiguous config", `{"credsStore":"desktop","CredsStore":"desktop"}`, "", inventory.DockerCredentialIssue{Name: "desktop"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, helper := range []string{"osxkeychain", "wincred", "pass", "secretservice"} {
				for _, suffix := range []string{"", ".exe"} {
					err := os.WriteFile(filepath.Join(dir, "docker-credential-"+helper+suffix), nil, 0o700)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, []byte(tc.config), 0o600); err != nil {
				t.Fatal(err)
			}
			env := sysenv.Env{"PATH=" + dir, "DOCKER_CONFIG=" + dir, "PATHEXT=.EXE"}
			issue := tc.issue
			issue.Replacement = docker.CredentialReplacement(env)
			if issue.Replacement == "" {
				t.Fatal("test native helper was not found")
			}
			p, err := DockerCredentialPlan(env, dir, issue)
			if tc.want == "" {
				if err == nil {
					t.Fatal("unsafe repair was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			edit := p.Steps[0].Edit
			want := strings.ReplaceAll(tc.want, "NEW", issue.Replacement)
			diff := edit.PreviewDiff()
			if edit.After != want || len(diff) != 2 || strings.Contains(diff[0].Text+diff[1].Text, "test-secret") {
				t.Fatal("repair changed unrelated bytes or exposed auth in the diff")
			}
			now := time.Now()
			backup, err := applyEdit(*edit, now)
			if err != nil {
				t.Fatal(err)
			}
			old, err := os.ReadFile(backup)
			if err != nil || string(old) != tc.config {
				t.Fatal("backup does not preserve the original config")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != want {
				t.Fatal("repair did not write the expected bytes")
			}
			if _, err := applyEdit(*edit, now); !errors.Is(err, ErrChanged) {
				t.Errorf("repeat using stale plan = %v, want ErrChanged", err)
			}
		})
	}
}

func TestDockerCredentialHelperInstallPlan(t *testing.T) {
	t.Parallel()
	brew := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
	apt := inventory.PackageManager{ID: "apt", Name: "APT", Path: "/usr/bin/apt"}
	p, ok := DockerCredentialHelperInstallPlan("osxkeychain", brew, nil)
	if !ok {
		t.Fatal("expected Homebrew install plan for osxkeychain")
	}
	want := []string{"/opt/homebrew/bin/brew", "install", "--formula", "docker-credential-helper"}
	if len(p.Steps) != 1 || strings.Join(p.Steps[0].Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("steps = %+v, want %q", p.Steps, want)
	}
	if !strings.Contains(p.Why, "docker-credential-osxkeychain") {
		t.Errorf("Why = %q, want mention of docker-credential-osxkeychain", p.Why)
	}
	for _, tc := range []struct {
		helper string
		m      inventory.PackageManager
	}{
		{"desktop", brew},
		{"pass", brew},
		{"", brew},
		{"osxkeychain", apt},
	} {
		if _, ok := DockerCredentialHelperInstallPlan(tc.helper, tc.m, nil); ok {
			t.Errorf("unexpected plan for helper %q on %q", tc.helper, tc.m.ID)
		}
	}
}
