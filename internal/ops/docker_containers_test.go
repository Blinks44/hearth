package ops

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/run"
)

func TestDockerContainerActions(t *testing.T) {
	target := docker.Target{Docker: "/usr/bin/docker", Context: "default", Endpoint: "unix:///var/run/docker.sock"}
	first := docker.Container{ID: strings.Repeat("a", 64), Name: "missing"}
	second := docker.Container{ID: strings.Repeat("b", 64), Name: "web"}
	first.State.Status, second.State.Status = "running", "running"
	p, err := PlanDockerContainers(target, nil, []docker.Container{first, second}, DockerRemove)
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	var removed bool
	r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
		id, verb := cmd.Args[len(cmd.Args)-1], cmd.Args[4]
		if cmd.Args[2] != target.Endpoint || cmd.Env[len(cmd.Env)-1] != "DOCKER_CONTEXT=" {
			t.Fatalf("command escaped its target: %+v", cmd)
		}
		commands = append(commands, verb+":"+id[:1])
		switch {
		case verb == "inspect" && (id == first.ID || removed):
			return run.Result{ExitCode: 1, Stderr: []byte("Error: No such container: " + id)}, nil
		case verb == "inspect":
			return run.Result{Stdout: fmt.Appendf(nil, "{\"ID\":%q,\"Name\":\"/web\",\"State\":{\"Status\":\"running\"}}\n", id)}, nil
		case verb == "rm":
			removed = true
		}
		return run.Result{}, nil
	})
	result := Execute(t.Context(), r, p, io.Discard, func(State) {})
	if !slices.Equal(commands, []string{"inspect:a", "inspect:b", "stop:b", "rm:b", "inspect:b"}) {
		t.Fatalf("commands = %v", commands)
	}
	if result.Phase != NotVerified || !slices.Equal(result.Steps, []StepStatus{StepFailed, Done}) || result.StepDetails[0] == "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDockerComposeRemovesEnvironmentButKeepsVolumes(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(file, []byte("name: shop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := docker.Target{Docker: "/usr/bin/docker", Context: "default", Endpoint: "unix:///var/run/docker.sock"}
	var r run.Fake
	r.Stdout(`{"name":"shop"}`, target.Command(nil, "compose", "--project-directory", dir, "-f", file, "config", "--format", "json").Args...)
	r.Stdout("", target.Command(nil, "ps", "--all", "--no-trunc", "--format", "{{json .}}").Args...)
	want := target.Command(nil, "compose", "--project-directory", dir, "-f", file, "-p", "shop", "down").Args
	r.Stdout("", want...)
	p, err := PlanDockerCompose(t.Context(), &r, nil, target, docker.ComposeProject{Dir: dir, Files: file}, DockerDown)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Calls()) != 2 || !slices.Equal(p.Steps[0].Argv, want) || p.Steps[0].Dir != dir {
		t.Fatalf("plan %+v, commands %+v", p.Steps, r.Calls())
	}
	if result := Execute(t.Context(), &r, p, io.Discard, func(State) {}); result.Phase != Succeeded {
		t.Fatalf("result %+v", result)
	}
	if last := r.Calls()[4]; !slices.Equal(last.Args, want) || last.Dir != dir {
		t.Fatalf("executed %+v", last)
	}
	r.Stdout(`{"name":"shop","services":{"web":{"image":"example.invalid/web"}}}`, target.Command(nil, "compose", "--project-directory", dir, "-f", file, "config", "--format", "json").Args...)
	if result := Execute(t.Context(), &r, p, io.Discard, func(State) {}); result.Phase == Succeeded || len(r.Calls()) != 6 {
		t.Fatalf("changed config was executed: %+v, commands %+v", result, r.Calls())
	}
}

func TestDockerContainerPauseUnpause(t *testing.T) {
	target := docker.Target{Docker: "/usr/bin/docker", Context: "default", Endpoint: "unix:///var/run/docker.sock"}
	running := docker.Container{ID: strings.Repeat("a", 64), Name: "web"}
	running.State.Status = "running"
	paused := docker.Container{ID: strings.Repeat("b", 64), Name: "worker"}
	paused.State.Status = "paused"
	stopped := docker.Container{ID: strings.Repeat("c", 64), Name: "done"}
	stopped.State.Status = "exited"

	if !DockerPause.Applies(running) || DockerPause.Applies(paused) || DockerPause.Applies(stopped) {
		t.Fatal("unexpected DockerPause.Applies")
	}
	if !DockerUnpause.Applies(paused) || DockerUnpause.Applies(running) || DockerUnpause.Applies(stopped) {
		t.Fatal("unexpected DockerUnpause.Applies")
	}
	if _, err := PlanDockerContainers(target, nil, []docker.Container{stopped}, DockerPause); err == nil {
		t.Fatal("accepted pausing a stopped container")
	}

	pPause, err := PlanDockerContainers(target, nil, []docker.Container{running}, DockerPause)
	if err != nil {
		t.Fatal(err)
	}
	status := "running"
	r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
		verb := cmd.Args[4]
		switch verb {
		case "inspect":
			return run.Result{Stdout: fmt.Appendf(nil, "{\"ID\":%q,\"Name\":\"/web\",\"State\":{\"Status\":%q}}\n", running.ID, status)}, nil
		case "pause":
			status = "paused"
		case "unpause":
			status = "running"
		}
		return run.Result{}, nil
	})
	if res := Execute(t.Context(), r, pPause, io.Discard, func(State) {}); res.Phase != Succeeded {
		t.Fatalf("pause result = %+v", res)
	}
	if status != "paused" {
		t.Fatalf("status after pause = %q", status)
	}

	running.State.Status = "paused"
	pUnpause, err := PlanDockerContainers(target, nil, []docker.Container{running}, DockerUnpause)
	if err != nil {
		t.Fatal(err)
	}
	if res := Execute(t.Context(), r, pUnpause, io.Discard, func(State) {}); res.Phase != Succeeded {
		t.Fatalf("unpause result = %+v", res)
	}
	if status != "running" {
		t.Fatalf("status after unpause = %q", status)
	}
}
