package ops

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Forward is a port-forward approved in the cluster view.
type Forward struct {
	Kubectl, Context, Namespace, Kind, Name, UID string
	Local, Remote                                int
}

func (f Forward) Address() string { return forwardAddress(f.Local) }

func forwardAddress(port int) string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func validPort(port int) bool { return port > 0 && port <= 65535 }

var (
	ErrForwardPortBusy = errors.New("local port is in use")
	ErrForwardTimeout  = errors.New("kubectl did not start forwarding in time")
	ErrForwardPodEnded = errors.New("the target pod ended")
)

// CheckForwardPort probes the same address kubectl will bind. On a
// conflict it also asks the OS for a free port to suggest to the user.
func CheckForwardPort(port int) (suggestion int, err error) {
	if !validPort(port) {
		return 0, fmt.Errorf("invalid local port %d", port)
	}
	l, err := net.Listen("tcp4", forwardAddress(port))
	if err == nil {
		return 0, l.Close()
	}
	if !errors.Is(err, forwardAddrInUse) {
		return 0, fmt.Errorf("check %s: %w", forwardAddress(port), err)
	}
	if free, err := net.Listen("tcp4", forwardAddress(0)); err == nil {
		suggestion = free.Addr().(*net.TCPAddr).Port
		_ = free.Close()
	}
	return suggestion, ErrForwardPortBusy
}

func (f Forward) argv(pod kube.ForwardPod) []string {
	return []string{f.Kubectl, "--context=" + f.Context, "port-forward", "--namespace=" + f.Namespace,
		"--address=127.0.0.1", "pod/" + pod.Name, strconv.Itoa(f.Local) + ":" + strconv.Itoa(pod.Port)}
}

// RunForward owns kubectl and the read of its exact target until the
// connection ends or ctx is cancelled.
func RunForward(ctx context.Context, r run.Runner, lines kube.LinesFunc, env sysenv.Env, f Forward, ready func(kube.ForwardPod)) error {
	if !validPort(f.Local) || !validPort(f.Remote) || f.Kubectl == "" || f.Context == "" ||
		f.Namespace == "" || f.Name == "" || f.UID == "" || (f.Kind != "pod" && f.Kind != "service") {
		return errors.New("incomplete port-forward target")
	}
	t := kube.Target{Kubectl: f.Kubectl, Context: f.Context}
	pod, err := kube.ResolveForwardPod(ctx, r, env, t, kube.Ref{Namespace: f.Namespace, UID: f.UID}, f.Kind+"/"+f.Name, f.Remote)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancelCause(ctx)
	var watch sync.WaitGroup
	watch.Go(func() {
		tick := time.NewTicker(kube.Every)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if kube.ForwardPodEnded(ctx, r, env, t, f.Namespace, pod) {
					cancel(ErrForwardPodEnded)
					return
				}
			}
		}
	})
	err = runForward(ctx, lines, env, f, pod, func() { ready(pod) })
	cancel(nil)
	watch.Wait()
	return err
}

func runForward(ctx context.Context, lines kube.LinesFunc, env sysenv.Env, f Forward, pod kube.ForwardPod, ready func()) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	startup := time.AfterFunc(30*time.Second, func() { cancel(ErrForwardTimeout) })
	defer startup.Stop()

	var stderr forwardOutput
	prefix := "Forwarding from " + f.Address() + " -> "
	err := lines(ctx, run.Cmd{Args: f.argv(pod), Env: env, Output: &stderr}, func(line string) {
		if strings.HasPrefix(line, prefix) && startup.Stop() {
			ready()
		}
	})
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		if strings.Contains(msg, "unable to listen on any of the requested ports") && strings.Contains(msg, "address already in use") {
			return fmt.Errorf("%w: %s", ErrForwardPortBusy, msg)
		}
		return fmt.Errorf("kubectl: %s", msg)
	}
	return err
}

// forwardOutput caps kubectl's diagnostics while the session is open.
type forwardOutput struct{ strings.Builder }

func (w *forwardOutput) Write(p []byte) (int, error) {
	const limit = 16 << 10
	_, _ = w.Builder.Write(p[:min(len(p), limit-w.Len())])
	return len(p), nil
}
