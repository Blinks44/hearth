//go:build !windows

package ops

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestColimaInstallPlanKeepsExistingBrewPrefixLink(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	prefix := filepath.Join(home, "brew")
	bin := filepath.Join(prefix, "bin")
	orb := filepath.Join(home, "Applications", "OrbStack.app", "bin")
	brew := filepath.Join(bin, "brew")
	service := filepath.Join(home, ".homebrew", "services", "colima.env")
	for _, dir := range []string{bin, orb, filepath.Dir(service)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bundled := filepath.Join(orb, "docker")
	if err := os.WriteFile(bundled, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(bin, "docker")
	if err := os.Symlink(bundled, link); err != nil {
		t.Fatal(err)
	}
	serviceBefore := "COLIMA_TEST=value\nPATH = /my/bin:/usr/bin\n"
	if err := os.WriteFile(service, []byte(serviceBefore), 0o600); err != nil {
		t.Fatal(err)
	}

	env := sysenv.Env{"HOME=" + home, "SHELL=/bin/zsh", "PATH=" + bin}
	p, err := ColimaInstallPlan(inventory.PackageManager{Path: brew, Prefix: prefix}, true, env, &run.Fake{})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range p.Steps {
		if slices.Contains(step.Argv, "--overwrite") || step.Remove != "" {
			t.Fatalf("destructive step: %+v", step)
		}
	}
	wantInstall := []string{brew, "install", "--formula", "--skip-link", "docker"}
	if !slices.ContainsFunc(p.Steps, func(step Step) bool { return slices.Equal(step.Argv, wantInstall) }) {
		t.Fatalf("missing %v: %+v", wantInstall, p.Steps)
	}
	opt := filepath.Join(prefix, "opt", "docker", "bin")
	if step := p.Steps[len(p.Steps)-1]; !strings.HasPrefix(sysenv.Env(step.Env).Get("PATH"), opt+":") {
		t.Fatalf("start PATH: %v", step.Env)
	}
	for _, file := range []string{filepath.Join(home, ".zshrc"), filepath.Join(home, ".zprofile"), service} {
		if !slices.ContainsFunc(p.Steps, func(step Step) bool {
			return step.Edit != nil && step.Edit.Path == file && strings.Contains(step.Edit.After, opt+":")
		}) {
			t.Errorf("missing PATH edit for %s", file)
		}
	}
	for _, step := range p.Steps {
		if step.Edit != nil && step.Edit.Path == service {
			if !strings.HasPrefix(step.Edit.After, serviceBefore) || !strings.Contains(step.Edit.After, opt+":/my/bin:/usr/bin") {
				t.Errorf("service PATH edit changed existing values: %q", step.Edit.After)
			}
		}
	}
	if got, err := os.Readlink(link); err != nil || got != bundled {
		t.Fatalf("planning changed OrbStack's link: %q, %v", got, err)
	}
}
