package docker

import (
	"context"
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

var colima = Target{Docker: "/opt/homebrew/bin/docker", Context: "colima", Endpoint: "unix:///Users/me/.colima/default/docker.sock"}

func infoArgs(t Target) []string {
	return []string{t.Docker, "--host", t.Endpoint, "info", "--format", "{{json .}}"}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRead(t *testing.T) {
	tests := []struct {
		name    string
		res     run.Result
		runErr  error
		want    Engine
		wantErr string
	}{
		{name: "answers", res: run.Result{Stdout: fixture(t, "info.json")},
			want: Engine{Version: "27.5.1", OS: "Ubuntu 24.04.1 LTS", Arch: "aarch64", CPUs: 4, Memory: 8326258688,
				Containers: 8, Running: 4, Stopped: 4, Images: 23}},
		{name: "engine down", res: run.Result{Stdout: fixture(t, "down.json"), ExitCode: 1},
			wantErr: "Cannot connect to the Docker daemon at unix:///Users/me/.colima/default/docker.sock. Is the docker daemon running?"},
		{name: "old CLI without JSON", res: run.Result{Stderr: []byte("template parsing error\n"), ExitCode: 1},
			wantErr: "template parsing error"},
		{name: "no CLI", runErr: errors.New("run docker: executable file not found"),
			wantErr: "docker info: run docker: executable file not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r run.Fake
			r.Set(tt.res, tt.runErr, infoArgs(colima)...)
			got, err := Read(t.Context(), &r, sysenv.Env{"DOCKER_CONTEXT=orbstack", "PATH=/usr/bin"}, colima)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Taken.IsZero() {
				t.Error("Taken not set")
			}
			got.Taken = time.Time{}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			// The terminal's context must not win over the engine read.
			env := r.Calls()[0].Env
			if env[len(env)-1] != "DOCKER_CONTEXT=" || !slices.Contains(env, "PATH=/usr/bin") {
				t.Errorf("env = %q", env)
			}
		})
	}
}

func TestReadEngineError(t *testing.T) {
	var r run.Fake
	r.Set(run.Result{Stdout: fixture(t, "down.json"), ExitCode: 1}, nil, infoArgs(colima)...)
	_, err := Read(t.Context(), &r, nil, colima)
	if _, ok := errors.AsType[*Error](err); !ok {
		t.Fatalf("err = %T %v, want *Error", err, err)
	}
}

func TestWatchStops(t *testing.T) {
	var r run.Fake
	r.Stdout(string(fixture(t, "info.json")), infoArgs(colima)...)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	reads := 0
	go func() {
		defer close(done)
		Watch(ctx, &r, nil, colima, false, func(e Engine, err error) {
			if err != nil || e.Running != 4 {
				t.Errorf("read %+v, %v", e, err)
			}
			reads++
			cancel()
		})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Watch did not stop with its context")
	}
	if reads != 1 {
		t.Fatalf("reads = %d, want 1", reads)
	}
}
