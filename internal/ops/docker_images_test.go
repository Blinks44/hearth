package ops

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/run"
)

func TestDockerImageRemoval(t *testing.T) {
	target := docker.Target{Docker: "/usr/bin/docker", Context: "local", Endpoint: "unix:///run/docker.sock"}
	id, container := "sha256:"+strings.Repeat("a", 64), strings.Repeat("b", 64)
	want := target.Command(nil, "image", "rm", "--no-prune", "--", id).Args
	for _, test := range []struct {
		name        string
		inUse, race bool
		phase       Phase
	}{
		{name: "unused", phase: Succeeded},
		{name: "used by stopped container", inUse: true},
		{name: "used after confirmation", race: true, phase: NotVerified},
	} {
		t.Run(test.name, func(t *testing.T) {
			used, writes := test.inUse, 0
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				if cmd.Args[2] != target.Endpoint || cmd.Env[len(cmd.Env)-1] != "DOCKER_CONTEXT=" {
					t.Fatalf("unpinned command: %+v", cmd)
				}
				switch {
				case slices.Equal(cmd.Args, want):
					writes++
				case slices.Equal(cmd.Args[3:5], []string{"image", "inspect"}):
					return run.Result{Stdout: fmt.Appendf(nil, `{"ID":%q,"RepoTags":["tool:1"]}`, id)}, nil
				case cmd.Args[3] == "ps":
					if used {
						return run.Result{Stdout: fmt.Appendf(nil, `{"ID":%q}`, container)}, nil
					}
				case slices.Equal(cmd.Args[3:5], []string{"container", "inspect"}):
					return run.Result{Stdout: fmt.Appendf(nil, `{"ID":%q,"Name":"/stopped","ImageID":%q,"State":{"Status":"exited"}}`, container, id)}, nil
				default:
					t.Fatalf("unexpected command: %+v", cmd)
				}
				return run.Result{}, nil
			})
			p, err := PlanDockerImageRemoval(t.Context(), r, target, []string{"DOCKER_CONTEXT=other"},
				[]docker.Image{{ID: id, RepoTags: []string{"tool:1"}}})
			if err != nil || writes != 0 {
				t.Fatalf("planning = %d writes, %v", writes, err)
			}
			if test.inUse {
				if len(p.Steps) != 0 || len(p.Notes) != 1 || !strings.Contains(p.Notes[0].Text, "stopped") {
					t.Fatalf("blocked plan = %+v", p)
				}
				return
			}
			used = test.race
			state := Execute(t.Context(), r, p, io.Discard, func(State) {})
			if state.Phase != test.phase || test.race && (writes != 0 || !strings.Contains(state.Err, "stopped")) ||
				!test.race && writes != 1 {
				t.Fatalf("execution = %+v, writes = %d", state, writes)
			}
		})
	}
}

func TestDockerPullPlan(t *testing.T) {
	target := docker.Target{Docker: "/usr/bin/docker", Context: "local", Endpoint: "unix:///run/docker.sock"}
	for _, ref := range []string{"alpine:3.23", "localhost:5000/team/app:3", "[::1]:5000/app", "docker.io/library/alpine@sha256:" + strings.Repeat("a", 64)} {
		p, err := PlanDockerPull(target, []string{"DOCKER_CONTEXT=other"}, ref)
		if err != nil || len(p.Steps) != 1 || !slices.Equal(p.Steps[0].Argv, target.Command(nil, "image", "pull", "--", ref).Args) ||
			p.Env[len(p.Env)-1] != "DOCKER_CONTEXT=" || p.DockerAction != DockerImagePull {
			t.Fatalf("pull %q: %+v, %v", ref, p, err)
		}
	}
	for _, ref := range []string{"", "--all-tags", "alpine latest", "https://registry.example.com/image", "user:password@registry.example.com/image"} {
		if _, err := PlanDockerPull(target, nil, ref); err == nil {
			t.Errorf("accepted %q", ref)
		}
	}
}
