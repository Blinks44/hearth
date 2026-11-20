package kube

import (
	"bytes"
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/containerlog"
	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// MaxLogLines is how many lines of a run the log view reads, its last
// ones, and keeps as more come.
const MaxLogLines = containerlog.MaxLines

// Logs is the log the view reads: a container's current run, followed as
// it writes, or its previous run, which ended.
type Logs struct {
	Kubectl, Context, Namespace, Pod, Container string
	Previous                                    bool
	// Terminated reads an ended current run without opening a follow stream.
	Terminated bool
	// Since reads the lines of the last Since alone; zero reads the run.
	Since time.Duration
}

// LogOwner identifies the workload whose pods may supply logs.
type LogOwner struct {
	Kind, Namespace, Name, UID string
}

func readLogPods(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]Pod, error) {
	owner := t.LogOwner
	scope := "--namespace=" + owner.Namespace
	controllers := map[string]string{owner.UID: owner.Kind}
	if owner.Kind == "Deployment" {
		var sets struct{ Items []meta }
		if err := get(ctx, r, env, t, []string{"replicasets.apps", scope}, &sets); err != nil {
			return nil, err
		}
		clear(controllers)
		for _, set := range sets.Items {
			if set.controller().Kind == owner.Kind && set.controllerUID() == owner.UID {
				controllers[set.Metadata.UID] = "ReplicaSet"
			}
		}
	}
	var list struct{ Items []pod }
	if err := get(ctx, r, env, t, []string{"pods", scope}, &list); err != nil {
		return nil, err
	}
	var pods []Pod
	for _, p := range list.Items {
		controller := p.controller()
		if kind, ok := controllers[p.controllerUID()]; ok && kind == controller.Kind {
			pods = append(pods, p.pod(Owner{Kind: owner.Kind, Name: owner.Name}))
		}
	}
	slices.SortFunc(pods, func(a, b Pod) int { return cmp.Compare(a.Name, b.Name) })
	return pods, nil
}

// argv is the kubectl logs reading l, every line with its time.
func (l Logs) argv() []string {
	argv := []string{l.Kubectl, "--context=" + l.Context, "logs", "--namespace=" + l.Namespace, l.Pod,
		"--container=" + l.Container, "--timestamps", "--tail=" + strconv.Itoa(MaxLogLines)}
	if l.Previous {
		argv = append(argv, "--previous")
	} else if !l.Terminated {
		argv = append(argv, "--follow")
	}
	if l.Since > 0 {
		argv = append(argv, "--since="+l.Since.String())
	}
	return argv
}

// Level is how severe a log line says it is.
type Level = containerlog.Level

const (
	LevelNone  = containerlog.LevelNone
	LevelDebug = containerlog.LevelDebug
	LevelInfo  = containerlog.LevelInfo
	LevelWarn  = containerlog.LevelWarn
	LevelError = containerlog.LevelError
)

type LogLine = containerlog.LogLine
type LinesFunc = containerlog.LinesFunc

func ParseLine(s string, prev Level) LogLine {
	return containerlog.ParseLine(s, prev)
}

// Stream reads l's log, giving got the lines that came, now and then, on
// another goroutine than Stream's but one at a time. It returns when
// kubectl ends (a previous run's log is read, a followed one ended with
// its container) or ctx does; kubectl's refusal is an *Error.
func Stream(ctx context.Context, lines LinesFunc, env sysenv.Env, l Logs, got func([]LogLine)) error {
	var stderr bytes.Buffer
	var failure *Error
	var last Level
	err := containerlog.Batch(func(emit func(LogLine)) error {
		return lines(ctx, run.Cmd{Args: l.argv(), Env: env, Output: &stderr, MaxOutput: 1 << 20}, func(s string) {
			ln := ParseLine(s, last)
			// The kubelet can send this diagnostic inside a successful log
			// response. A container line has a timestamp with --timestamps.
			if ln.At.IsZero() && strings.HasPrefix(s, "failed to create fsnotify watcher:") {
				failure = &Error{Access: detect.KubeFailure(s)}
				return
			}
			last = ln.Level
			emit(ln)
		})
	}, got)
	if ctx.Err() != nil {
		return err
	}
	if failure != nil {
		return failure
	}
	if err == nil {
		return err
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return &Error{Access: detect.KubeFailure(msg)}
	}
	return err
}

// lastLine is the last line of the run of the target's Tail container,
// without its time; empty when kubectl has none to give.
func lastLine(ctx context.Context, r run.Runner, env sysenv.Env, t Target) string {
	argv := []string{t.Kubectl, "--context=" + t.Context, "logs", "--namespace=" + t.Tail.Namespace, t.Tail.Pod,
		"--container=" + t.Tail.Container}
	if t.Tail.Previous {
		argv = append(argv, "--previous")
	}
	argv = append(argv, "--tail=1", "--request-timeout=15s")
	out, err := r.Run(ctx, run.Cmd{Args: argv, Env: env, Timeout: timeout})
	if err != nil || out.ExitCode != 0 {
		return ""
	}
	return strings.TrimSpace(string(out.Stdout))
}
