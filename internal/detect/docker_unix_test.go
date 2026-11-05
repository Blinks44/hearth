//go:build !windows

package detect

import (
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestDockerKeepsCommandName(t *testing.T) {
	home := root(t)
	target := fakeExe(t, filepath.Join(home, "app", "docker-tools"))
	exe := filepath.Join(home, "bin", "docker")
	symlink(t, target, exe)
	env := sysenv.Env{"HOME=" + home, "PATH=" + filepath.Dir(exe)}
	r := &run.Fake{}
	r.Stdout("Docker version 27.4.1, build abc", exe, "--version")
	r.Stdout(`{"Name":"default","DockerEndpoint":"unix:///tmp/docker.sock"}`, exe, "context", "ls", "--format", "{{json .}}")
	r.Stdout(`{"Server":{"Version":"27.4.1"}}`, exe, "--host", "unix:///tmp/docker.sock", "version", "--format", "{{json .}}")
	st, err := detectDocker(t.Context(), r, env, env, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Installations) != 1 || st.Installations[0].Version != "27.4.1" || st.Installations[0].RealPath != target || st.Docker.Contexts[0].Status != "available" {
		t.Fatalf("%+v %+v", st, st.Docker)
	}
	for _, call := range r.Calls() {
		if call.Args[0] != exe {
			t.Fatalf("lost argv0: %v", call.Args)
		}
	}
}
