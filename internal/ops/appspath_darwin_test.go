package ops

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestAppsPathPlan(t *testing.T) {
	home := t.TempDir()
	path := "/Users/me/.local/bin:/opt/homebrew/bin:/usr/bin:/bin:/a&b"
	agent := filepath.Join(home, "Library", "LaunchAgents", "dev.hearth.path.plist")
	for _, before := range []string{"", "old"} {
		if before != "" {
			if err := os.MkdirAll(filepath.Dir(agent), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(agent, []byte(before), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		p, err := AppsPathPlan(sysenv.Env{"HOME=" + home, "PATH=" + path})
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Steps) != 2 || p.Steps[0].Edit == nil || p.Verify == nil {
			t.Fatalf("steps %+v", p.Steps)
		}
		e := p.Steps[0].Edit
		if e.Path != agent || e.Before != before || e.New != (before == "") {
			t.Errorf("edit %s, before %q, new %v", e.Path, e.Before, e.New)
		}
		want := []string{"/bin/launchctl", "setenv", "PATH", path}
		if !slices.Equal(p.Steps[1].Argv, want) {
			t.Errorf("argv %q", p.Steps[1].Argv)
		}
		// The agent runs the same command as the user signs in.
		var plist struct {
			Dict struct {
				Keys    []string `xml:"key"`
				Strings []string `xml:"string"`
				Args    []string `xml:"array>string"`
			} `xml:"dict"`
		}
		if err := xml.NewDecoder(strings.NewReader(e.After)).Decode(&plist); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(plist.Dict.Args, want) || !slices.Equal(plist.Dict.Keys, []string{"Label", "ProgramArguments", "RunAtLoad"}) ||
			!slices.Equal(plist.Dict.Strings, []string{"dev.hearth.path"}) {
			t.Errorf("plist %+v\n%s", plist.Dict, e.After)
		}
	}
	stop := AppsPathStopPlan(home)
	if len(stop.Steps) != 2 || !slices.Equal(stop.Steps[0].Argv, []string{"/bin/rm", "-f", agent}) ||
		!slices.Equal(stop.Steps[1].Argv, []string{"/bin/launchctl", "unsetenv", "PATH"}) {
		t.Errorf("stop steps %+v", stop.Steps)
	}
}
