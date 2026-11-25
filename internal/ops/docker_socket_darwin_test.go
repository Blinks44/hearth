//go:build darwin

package ops

import (
	"encoding/xml"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestDockerDefaultSocketPlanDarwinLaunchDaemon(t *testing.T) {
	socket := "/Users/dev & qa/.colima/default/docker.sock"
	p, ok := DockerDefaultSocketPlan("colima", "unix://"+socket, nil)
	if !ok {
		t.Fatal("expected valid macOS Docker default socket plan")
	}
	if len(p.Steps) != 1 || !p.Steps[0].Admin {
		t.Fatalf("expected single admin step, got %+v", p.Steps)
	}
	step := p.Steps[0]
	wantShown := []string{"ln", "-sfn", socket, dockerDefaultSocket}
	if !slices.Equal(step.Shown(), wantShown) {
		t.Fatalf("Shown() = %q, want %q", step.Shown(), wantShown)
	}
	if step.Note == "" {
		t.Fatal("expected step note mentioning LaunchDaemon")
	}
	if len(step.Argv) != 9 || step.Argv[0] != "/bin/sh" || step.Argv[1] != "-c" ||
		step.Argv[4] != socket || step.Argv[5] != dockerDefaultSocket ||
		step.Argv[6] != dockerSocketDaemonPath || step.Argv[8] != dockerSocketDaemonLabel {
		t.Fatalf("unexpected darwin argv: %q", step.Argv)
	}
	plistXML := step.Argv[7]
	var plist struct {
		Dict struct {
			Keys    []string `xml:"key"`
			Strings []string `xml:"string"`
			Args    []string `xml:"array>string"`
		} `xml:"dict"`
	}
	if err := xml.NewDecoder(strings.NewReader(plistXML)).Decode(&plist); err != nil {
		t.Fatalf("parse generated plist: %v\n%s", err, plistXML)
	}
	wantArgs := []string{"/bin/ln", "-sfn", socket, dockerDefaultSocket}
	if !slices.Equal(plist.Dict.Keys, []string{"Label", "ProgramArguments", "RunAtLoad"}) ||
		!slices.Equal(plist.Dict.Strings, []string{dockerSocketDaemonLabel}) ||
		!slices.Equal(plist.Dict.Args, wantArgs) {
		t.Fatalf("unexpected plist contents: %+v\n%s", plist.Dict, plistXML)
	}
	if !slices.ContainsFunc(p.Changes, func(s string) bool { return strings.Contains(s, dockerSocketDaemonPath) }) {
		t.Fatalf("expected LaunchDaemon path in plan changes: %q", p.Changes)
	}
	if !slices.ContainsFunc(p.Notes, func(n Note) bool { return strings.Contains(n.Text, dockerSocketDaemonLabel) }) {
		t.Fatalf("expected LaunchDaemon label in plan notes: %+v", p.Notes)
	}
	if plutil, err := exec.LookPath("plutil"); err == nil {
		cmd := exec.CommandContext(t.Context(), plutil, "-lint", "-")
		cmd.Stdin = strings.NewReader(plistXML)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("plutil -lint failed: %v\n%s", err, out)
		}
	}
}
