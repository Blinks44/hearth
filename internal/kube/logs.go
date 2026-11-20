package kube

import (
	"bytes"
	"cmp"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// MaxLogLines is how many lines of a run the log view reads, its last
// ones, and keeps as more come.
const MaxLogLines = 10000

// flushEvery is how often Stream gives the lines that came.
const flushEvery = 100 * time.Millisecond

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
type Level int

const (
	LevelNone Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

// LogLine is a line of a container's log.
type LogLine struct {
	// At is when the container wrote it, by kubelet; zero when the line
	// came without.
	At    time.Time
	Pod   string
	Text  string
	Level Level
}

// LinesFunc runs a command that keeps writing and gives each line of its
// stdout, as run.Lines does.
type LinesFunc func(ctx context.Context, c run.Cmd, line func(string)) error

// Stream reads l's log, giving got the lines that came, now and then, on
// another goroutine than Stream's but one at a time. It returns when
// kubectl ends (a previous run's log is read, a followed one ended with
// its container) or ctx does; kubectl's refusal is an *Error.
func Stream(ctx context.Context, lines LinesFunc, env sysenv.Env, l Logs, got func([]LogLine)) error {
	var (
		mu    sync.Mutex
		batch []LogLine
		last  Level
	)
	send := func() {
		mu.Lock()
		b := batch
		batch = nil
		mu.Unlock()
		if len(b) > 0 {
			got(b)
		}
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		tick := time.NewTicker(flushEvery)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				send()
			case <-done:
				return
			}
		}
	})
	var stderr bytes.Buffer
	var failure *Error
	err := lines(ctx, run.Cmd{Args: l.argv(), Env: env, Output: &stderr, MaxOutput: 1 << 20}, func(s string) {
		ln := ParseLine(s, last)
		// The kubelet can send this diagnostic inside a successful log
		// response. A container line has a timestamp with --timestamps.
		if ln.At.IsZero() && strings.HasPrefix(s, "failed to create fsnotify watcher:") {
			failure = &Error{Access: detect.KubeFailure(s)}
			return
		}
		last = ln.Level
		mu.Lock()
		batch = append(batch, ln)
		mu.Unlock()
	})
	close(done)
	wg.Wait()
	send()
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

var (
	// levelWord finds a level among the first words of a line, as
	// "INFO msg", "[warn] msg" or "2026/11/19 12:00:00 ERROR msg".
	levelWord = regexp.MustCompile(`(?i)^((?:\S+\s+){0,2}?)[\[(<]?(trace|debug|info|notice|warn|warning|error|err|fatal|panic|critical|crit)[\])>]?(?::?\s+|:?$)`)
	// levelField finds a level of structured logs: level=error, or
	// "level":"warn" in JSON.
	levelField = regexp.MustCompile(`(?i)\b(?:level|lvl|severity)"?\s*[=:]\s*"?([a-z]+)`)
	// klog finds a line of Kubernetes' own components, as "E1119 12:00…".
	klog = regexp.MustCompile(`^([IWEF])\d{4} \d`)
)

// ParseLine reads a line kubectl logs --timestamps wrote: its time, its
// text and its level. A line without a level of its own that starts with
// space continues the line before, as a stack trace does, and takes prev.
// A level in capitals that is the line's first word, as INFO, leaves the
// text; "Error: …" keeps it.
func ParseLine(s string, prev Level) LogLine {
	var ln LogLine
	if at, text, ok := strings.Cut(s, " "); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil {
			ln.At, s = t, text
		}
	}
	ln.Text = s
	switch m := levelWord.FindStringSubmatchIndex(s); {
	case m != nil:
		word := s[m[4]:m[5]]
		ln.Level = levelOf(word)
		if m[3] == 0 && word == strings.ToUpper(word) && !strings.Contains(s[:m[1]], ":") {
			ln.Text = s[m[1]:]
		}
	case levelField.MatchString(s):
		ln.Level = levelOf(levelField.FindStringSubmatch(s)[1])
	case klog.MatchString(s):
		ln.Level = map[byte]Level{'I': LevelInfo, 'W': LevelWarn, 'E': LevelError, 'F': LevelError}[s[0]]
	case strings.HasPrefix(s, " ") || strings.HasPrefix(s, "\t"):
		ln.Level = prev
	}
	return ln
}

func levelOf(word string) Level {
	switch strings.ToLower(word) {
	case "trace", "debug":
		return LevelDebug
	case "info", "notice":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error", "err", "fatal", "panic", "critical", "crit":
		return LevelError
	}
	return LevelNone
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
