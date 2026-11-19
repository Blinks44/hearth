package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
)

func TestRunForward(t *testing.T) {
	for _, tt := range []struct {
		name   string
		output []string
		stderr string
		ready  int
		want   error
	}{
		{"binds once", []string{"Forwarding from 127.0.0.1:18080 -> 8080", "Forwarding from 127.0.0.1:18080 -> 8080"}, "", 1, nil},
		{"other address is not ready", []string{"Forwarding from 127.0.0.1:1234 -> 80"}, "", 0, nil},
		{"bind race", nil, "Unable to listen: bind: address already in use\nerror: unable to listen on any of the requested ports", 0, ErrForwardPortBusy},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := Forward{Kubectl: "/bin/kubectl", Context: "kind-dev", Namespace: "shop", Kind: "service", Name: "web", Local: 18080, Remote: 80}
			wantArgv := []string{"/bin/kubectl", "--context=kind-dev", "port-forward", "--namespace=shop", "--address=127.0.0.1",
				"pod/web-123", "18080:8080"}
			lines := func(_ context.Context, cmd run.Cmd, line func(string)) error {
				if !slices.Equal(cmd.Args, wantArgv) || !slices.Equal(cmd.Env, []string{"KUBECONFIG=/tmp/test"}) {
					t.Fatalf("command = %v, env = %v", cmd.Args, cmd.Env)
				}
				for _, s := range tt.output {
					line(s)
				}
				_, _ = io.WriteString(cmd.Output, tt.stderr)
				return nil
			}
			var ready int
			err := runForward(t.Context(), lines, []string{"KUBECONFIG=/tmp/test"}, f, kube.ForwardPod{Name: "web-123", Port: 8080}, func() { ready++ })
			if !errors.Is(err, tt.want) || ready != tt.ready {
				t.Errorf("ready = %d, err = %v", ready, err)
			}
		})
	}
}

func TestRunForwardStop(t *testing.T) {
	f := Forward{Kubectl: "kubectl", Context: "kind-dev", Namespace: "shop", Kind: "pod", Name: "web", Local: 18080, Remote: 8080}
	ctx, stop := context.WithCancel(t.Context())
	defer stop()
	procErr := fmt.Errorf("kubectl stopped: %w", context.Canceled)
	err := runForward(ctx, func(ctx context.Context, cmd run.Cmd, line func(string)) error {
		line("Forwarding from 127.0.0.1:18080 -> 8080")
		if ctx.Err() == nil {
			t.Error("process context is still active")
		}
		_, _ = io.WriteString(cmd.Output, strings.Repeat("x", 20000))
		return procErr
	}, nil, f, kube.ForwardPod{Name: "web", Port: 8080}, stop)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunForward = %v, want cancellation", err)
	}
}

func TestRunForwardStartupTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := Forward{Kubectl: "kubectl", Context: "kind-dev", Namespace: "shop", Kind: "pod", Name: "web", Local: 18080, Remote: 8080}
		err := runForward(t.Context(), func(ctx context.Context, _ run.Cmd, _ func(string)) error {
			<-ctx.Done()
			return ctx.Err()
		}, nil, f, kube.ForwardPod{Name: "web", Port: 8080}, func() { t.Error("reported ready without a listener") })
		if !errors.Is(err, ErrForwardTimeout) {
			t.Fatalf("RunForward = %v, want startup timeout", err)
		}
	})
}

func TestRunForwardEndsWithPod(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := Forward{Kubectl: "kubectl", Context: "kind-dev", Namespace: "shop", Kind: "pod", Name: "web", UID: "web-1", Local: 18080, Remote: 8080}
		var runner run.Fake
		get := []string{"kubectl", "--context=kind-dev", "get", "pod/web", "--namespace=shop", "--output=json", "--request-timeout=15s"}
		runner.Stdout(`{"metadata":{"name":"web","uid":"web-1"},"status":{"phase":"Running"}}`, get...)
		err := RunForward(t.Context(), &runner, func(ctx context.Context, _ run.Cmd, line func(string)) error {
			line("Forwarding from 127.0.0.1:18080 -> 8080")
			<-ctx.Done()
			return ctx.Err()
		}, nil, f, func(pod kube.ForwardPod) {
			if pod.UID != "web-1" {
				t.Errorf("ready pod = %+v", pod)
			}
			runner.Set(run.Result{ExitCode: 1, Stderr: []byte(`Error from server (NotFound): pods "web" not found`)}, nil, get...)
		})
		if !errors.Is(err, ErrForwardPodEnded) {
			t.Fatalf("RunForward = %v, want pod ended", err)
		}
	})
}
