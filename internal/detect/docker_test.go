package detect

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDocker(t *testing.T) {
	for _, tc := range []struct {
		name, override, selected, status string
		server                           string
		code                             int
		probe, host                      bool
	}{
		{name: "available", selected: "colima", status: "available", server: `{"Server":{"Version":"27.4.1"}}`, probe: true},
		{name: "stopped", selected: "colima", status: "unavailable", server: `{"Client":{"Version":"27.4.1"},"Server":null}`, code: 1, probe: true},
		{name: "invalid response", selected: "colima", status: "unavailable", server: `oops`, probe: true},
		{name: "remote context", override: "DOCKER_CONTEXT=remote", selected: "remote"},
		{name: "remote host", override: "DOCKER_HOST=tcp://user:secret@server:2376/path?token=hidden", selected: "default"},
		{name: "local host", override: "DOCKER_HOST=" + dockerDefaultEndpoint, selected: "default", status: "available", server: `{"Server":{"Version":"27.4.1"}}`, probe: true},
		{name: "host overrides context", override: "DOCKER_CONTEXT=colima", host: true, selected: "default"},
		{name: "unknown context", override: "DOCKER_CONTEXT=missing", selected: "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := root(t)
			bin := filepath.Join(home, "bin")
			exe := filepath.Join(bin, "docker"+dockerSuffix)
			writeFile(t, exe, "")
			if err := os.Chmod(exe, 0o755); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(home, "config")
			writeFile(t, filepath.Join(config, "config.json"), `{"currentContext":"colima","auths":{"registry":{"auth":"PRIVATE"}}}`)
			writeFile(t, filepath.Join(config, "cli-plugins", "docker-compose"+dockerSuffix), "")
			env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + bin, "DOCKER_CONFIG=" + config}
			if tc.host {
				env = append(env, "DOCKER_HOST=ssh://user:secret@server")
			}
			if tc.override != "" {
				env = append(env, tc.override)
			}
			r := &run.Fake{}
			r.Stdout("Docker version 27.4.1, build abc\n", exe, "--version")
			r.Stdout(`{"Name":"default","DockerEndpoint":"`+dockerDefaultEndpoint+`"}
{"Name":"colima","DockerEndpoint":"unix:///tmp/colima/docker.sock"}
{"Name":"remote","DockerEndpoint":"ssh://user:secret@server/path?token=hidden"}`, exe, "context", "ls", "--format", "{{json .}}")
			r.Set(run.Result{Stdout: []byte(tc.server), Stderr: []byte("PRIVATE"), ExitCode: tc.code}, nil, exe, "--host", func() string {
				if tc.selected == "colima" {
					return "unix:///tmp/colima/docker.sock"
				}
				return dockerDefaultEndpoint
			}(), "version", "--format", "{{json .}}")
			st, err := detectDocker(t.Context(), r, env, env, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(st.Installations) != 1 || st.Installations[0].Version != "27.4.1" || st.Installations[0].OnPath != 1 || st.Installations[0].InGUI != 1 {
				t.Fatalf("%+v", st)
			}
			if st.Docker.Selected != tc.selected || len(st.Docker.Plugins) != 1 {
				t.Fatalf("%+v", st.Docker)
			}
			probes := 0
			for _, call := range r.Calls() {
				if call.Args[1] == "--host" {
					probes++
					if call.Timeout == 0 {
						t.Error("no explicit probe timeout")
					}
				}
			}
			if (probes == 1) != tc.probe {
				t.Fatalf("probes=%d, want probe=%v", probes, tc.probe)
			}
			for _, c := range st.Docker.Contexts {
				want := ""
				if c.Name == tc.selected {
					want = tc.status
				}
				if c.Status != want {
					t.Errorf("%+v, want status %q", c, want)
				}
			}
			data, _ := json.Marshal(st)
			for _, secret := range []string{"secret", "hidden", "PRIVATE", "user:"} {
				if strings.Contains(string(data), secret) {
					t.Errorf("snapshot contains %q", secret)
				}
			}
		})
	}
}

func TestDockerContextFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.json"), `{"currentContext":"orbstack"}`)
	writeFile(t, filepath.Join(dir, "contexts", "meta", "one", "meta.json"), `{"Name":"orbstack","Endpoints":{"docker":{"Host":"unix:///tmp/orbstack/docker.sock"}}}`)
	writeFile(t, filepath.Join(dir, "contexts", "meta", "bad", "meta.json"), `{`)
	contexts, selected, problem := dockerContextFiles(dir)
	if len(contexts) != 2 || selected != "orbstack" || problem == "" || contexts[1].Engine != "OrbStack" {
		t.Fatalf("%+v %s %s", contexts, selected, problem)
	}
	writeFile(t, filepath.Join(dir, "config.json"), `{`)
	_, selected, problem = dockerContextFiles(dir)
	if selected != "default" || problem == "" {
		t.Fatalf("%s %s", selected, problem)
	}
}

func TestDockerWithoutCLI(t *testing.T) {
	home := root(t)
	config := filepath.Join(home, ".docker")
	writeFile(t, filepath.Join(config, "config.json"), `{"currentContext":"desktop-linux"}`)
	writeFile(t, filepath.Join(config, "contexts", "meta", "one", "meta.json"), `{"Name":"desktop-linux","Endpoints":{"docker":{"Host":"unix:///tmp/desktop/docker.sock"}}}`)
	r := &run.Fake{}
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH=", "DOCKER_CONFIG=" + config}
	st, err := detectDocker(t.Context(), r, env, env, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if st.Docker.Selected != "desktop-linux" || len(r.Calls()) != 0 {
		t.Fatalf("%+v, calls %v", st, r.Calls())
	}
	for _, c := range st.Docker.Contexts {
		if c.Status != "" {
			t.Fatalf("unprobed context: %+v", c)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := detectDocker(ctx, r, env, env, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
}
