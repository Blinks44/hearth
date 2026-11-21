package ops

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/run"
)

func TestDockerResourceRemoval(t *testing.T) {
	target := docker.Target{Docker: "/usr/bin/docker", Context: "local", Endpoint: "unix:///run/docker.sock"}
	networkID, containerID := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tc := range []struct {
		name                       string
		kind                       docker.ResourceKind
		resource, project          string
		active, attaches, replaced bool
		want                       Phase
	}{
		{name: "free volume", kind: docker.VolumeResource, resource: "data", want: Succeeded},
		{name: "compose volume", kind: docker.VolumeResource, resource: "shop_data", project: "shop", want: Succeeded},
		{name: "stopped container uses volume", kind: docker.VolumeResource, resource: "data", active: true},
		{name: "volume attached after review", kind: docker.VolumeResource, resource: "data", attaches: true, want: NotVerified},
		{name: "volume recreated after review", kind: docker.VolumeResource, resource: "data", replaced: true, want: NotVerified},
		{name: "unused network", kind: docker.NetworkResource, resource: "shop_default", want: Succeeded},
		{name: "stopped container uses network", kind: docker.NetworkResource, resource: "shop_default", active: true},
		{name: "network attached after review", kind: docker.NetworkResource, resource: "shop_default", attaches: true, want: NotVerified},
		{name: "system bridge", kind: docker.NetworkResource, resource: "bridge"},
		{name: "system host", kind: docker.NetworkResource, resource: "host"},
		{name: "system none", kind: docker.NetworkResource, resource: "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			id := tc.resource
			if tc.kind == docker.NetworkResource {
				id = networkID
			}
			created := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
			used, writes := tc.active, 0
			cmdWant := target.Command(nil, string(tc.kind), "rm", "--", id).Args
			r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
				if cmd.Args[2] != target.Endpoint || cmd.Env[len(cmd.Env)-1] != "DOCKER_CONTEXT=" {
					t.Fatalf("unpinned command: %+v", cmd)
				}
				switch {
				case slices.Equal(cmd.Args, cmdWant):
					writes++
				case slices.Equal(cmd.Args[3:5], []string{string(tc.kind), "inspect"}):
					return run.Result{Stdout: fmt.Appendf(nil, `{"ID":%q,"Name":%q,"Project":%q,"Created":%q}`, id, tc.resource, tc.project, created.Format(time.RFC3339))}, nil
				case cmd.Args[3] == "ps":
					if used {
						return run.Result{Stdout: fmt.Appendf(nil, `{"ID":%q}`, containerID)}, nil
					}
				case slices.Equal(cmd.Args[3:5], []string{"container", "inspect"}):
					return run.Result{Stdout: fmt.Appendf(nil, `{"ID":%q,"Name":"/stopped","State":{"Status":"exited"},"Mounts":[{"Type":"volume","Name":%q}],"Networks":{"shop_default":{"NetworkID":%q}}}`, containerID, id, id)}, nil
				default:
					t.Fatalf("unexpected command: %+v", cmd)
				}
				return run.Result{}, nil
			})
			p, err := PlanDockerResourceRemoval(t.Context(), r, target, nil, tc.kind,
				docker.Resource{ID: id, Name: tc.resource, Project: tc.project, Created: created})
			if err != nil || writes != 0 {
				t.Fatalf("planning = %d writes, %v", writes, err)
			}
			if tc.want == Running {
				if len(p.Steps) != 0 || (tc.active && !strings.Contains(p.Why, "stopped")) {
					t.Fatalf("blocked plan = %+v", p)
				}
				return
			}
			wantName := ""
			if tc.kind == docker.VolumeResource && tc.project != "" {
				wantName = tc.resource
			}
			if p.ConfirmName != wantName || len(p.Steps) != 1 {
				t.Fatalf("confirmation = %+v", p)
			}
			if tc.replaced {
				created = created.Add(time.Minute)
			}
			used = tc.attaches
			state := Execute(t.Context(), r, p, io.Discard, func(State) {})
			if state.Phase != tc.want || (writes == 1) != (tc.want == Succeeded) {
				t.Fatalf("execution = %+v, writes = %d", state, writes)
			}
		})
	}
}

func TestPlanDockerResourcesRemoval(t *testing.T) {
	target := docker.Target{Docker: "/usr/bin/docker", Context: "local", Endpoint: "unix:///run/docker.sock"}
	created := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	r := resourceRunner(func(_ context.Context, cmd run.Cmd) (run.Result, error) {
		switch {
		case slices.Equal(cmd.Args[3:5], []string{"volume", "inspect"}):
			var out []byte
			if sep := slices.Index(cmd.Args, "--"); sep >= 0 {
				for _, name := range cmd.Args[sep+1:] {
					out = fmt.Appendf(out, `{"ID":%q,"Name":%q,"Project":"shop","Created":%q}`+"\n", name, name, created.Format(time.RFC3339))
				}
			}
			return run.Result{Stdout: out}, nil
		case slices.Equal(cmd.Args[3:5], []string{"network", "inspect"}):
			var out []byte
			if sep := slices.Index(cmd.Args, "--"); sep >= 0 {
				for _, id := range cmd.Args[sep+1:] {
					name := "shop_default"
					if id == strings.Repeat("b", 64) {
						name = "bridge"
					}
					out = fmt.Appendf(out, `{"ID":%q,"Name":%q,"Driver":"bridge","Created":%q}`+"\n", id, name, created.Format(time.RFC3339))
				}
			}
			return run.Result{Stdout: out}, nil
		case cmd.Args[3] == "ps":
			return run.Result{}, nil
		default:
			t.Fatalf("unexpected command: %+v", cmd)
			return run.Result{}, nil
		}
	})
	p, err := PlanDockerResourcesRemoval(t.Context(), r, target, nil, docker.VolumeResource, []docker.Resource{
		{ID: "vol-a", Name: "vol-a", Project: "shop", Created: created},
		{ID: "vol-b", Name: "vol-b", Project: "shop", Created: created},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 2 || p.ConfirmName != "" || p.DockerAction != DockerResourceRemove {
		t.Fatalf("bulk volume plan = %+v", p)
	}
	// Including a system network blocks the entire bulk plan.
	blocked, err := PlanDockerResourcesRemoval(t.Context(), r, target, nil, docker.NetworkResource, []docker.Resource{
		{ID: strings.Repeat("a", 64), Name: "shop_default", Driver: "bridge", Created: created},
		{ID: strings.Repeat("b", 64), Name: "bridge", Driver: "bridge", Created: created},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked.Steps) != 0 {
		t.Fatalf("system network in bulk selection was not blocked: %+v", blocked)
	}
}
