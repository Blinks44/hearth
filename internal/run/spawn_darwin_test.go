//go:build darwin

package run

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/ebitengine/purego"
)

func responsiblePIDFor(t *testing.T, pid int) int {
	t.Helper()
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatalf("dlopen libSystem: %v", err)
	}
	sym, err := purego.Dlsym(lib, "responsibility_get_pid_responsible_for_pid")
	if err != nil {
		t.Fatalf("dlsym responsibility_get_pid_responsible_for_pid: %v", err)
	}
	var getResp func(int32) int32
	purego.RegisterFunc(&getResp, sym)
	return int(getResp(int32(pid)))
}

// lineCatcher delivers the first newline-terminated line written to it.
type lineCatcher struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	once sync.Once
	ch   chan string
}

func (w *lineCatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if line, _, ok := strings.Cut(w.buf.String(), "\n"); ok {
		w.once.Do(func() { w.ch <- line })
	}
	return len(p), nil
}

func TestDisclaimResponsibility(t *testing.T) {
	if loadSpawnAPI() == nil {
		t.Fatal("loadSpawnAPI returned nil on darwin")
	}
	selfResp := responsiblePIDFor(t, os.Getpid())
	if selfResp <= 0 {
		t.Fatalf("unexpected self responsible pid: %d", selfResp)
	}

	spawnAndInspect := func(t *testing.T, disclaim bool) (childPID, grandPID, respChild, respGrand int) {
		t.Helper()
		pr, pw := io.Pipe()
		catcher := &lineCatcher{ch: make(chan string, 1)}
		// Keep both child and grandchild alive until stdin closes so we can
		// query their live responsible PIDs reliably without sleeps.
		script := `/bin/sleep 30 & grand=$!; echo "$$ $grand"; read _; kill "$grand" 2>/dev/null; wait "$grand" 2>/dev/null; exit 0`
		done := make(chan error, 1)
		go func() {
			_, err := (Exec{}).Run(t.Context(), Cmd{
				Args:     []string{"/bin/sh", "-c", script},
				Dir:      t.TempDir(),
				Stdin:    pr,
				Output:   catcher,
				Timeout:  10 * time.Second,
				Disclaim: disclaim,
			})
			done <- err
		}()

		var line string
		select {
		case line = <-catcher.ch:
		case err := <-done:
			t.Fatalf("child exited before reporting pids: %v", err)
		case <-time.After(5 * time.Second):
			_ = pw.Close()
			t.Fatal("timed out waiting for child pids")
		}

		fields := strings.Fields(line)
		if len(fields) != 2 {
			_ = pw.Close()
			t.Fatalf("unexpected pid line %q", line)
		}
		var err error
		if childPID, err = strconv.Atoi(fields[0]); err != nil {
			_ = pw.Close()
			t.Fatalf("parse child pid %q: %v", fields[0], err)
		}
		if grandPID, err = strconv.Atoi(fields[1]); err != nil {
			_ = pw.Close()
			t.Fatalf("parse grand pid %q: %v", fields[1], err)
		}

		respChild = responsiblePIDFor(t, childPID)
		respGrand = responsiblePIDFor(t, grandPID)

		_ = pw.Close()
		if err := <-done; err != nil {
			t.Fatalf("child run failed: %v", err)
		}
		return childPID, grandPID, respChild, respGrand
	}

	t.Run("disclaim=true makes child and grandchild responsible to child", func(t *testing.T) {
		childPID, _, respChild, respGrand := spawnAndInspect(t, true)
		if respChild != childPID {
			t.Errorf("child responsible pid = %d, want %d (self responsible = %d)", respChild, childPID, selfResp)
		}
		if respGrand != childPID {
			t.Errorf("grandchild responsible pid = %d, want %d (self responsible = %d)", respGrand, childPID, selfResp)
		}
	})

	t.Run("disclaim=false preserves inherited responsibility", func(t *testing.T) {
		childPID, _, respChild, _ := spawnAndInspect(t, false)
		if respChild != selfResp {
			t.Errorf("child responsible pid = %d, want inherited %d (child pid = %d)", respChild, selfResp, childPID)
		}
	})
}

func TestDisclaimExecBehavior(t *testing.T) {
	ctx := t.Context()

	t.Run("stdout stderr output and cap", func(t *testing.T) {
		var live syncBuffer
		res, err := (Exec{}).Run(ctx, Cmd{
			Args:      []string{"/bin/sh", "-c", "printf '0123456789abcdef'; printf 'err-msg' >&2"},
			Dir:       t.TempDir(),
			MaxOutput: 6,
			Output:    &live,
			Disclaim:  true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if res.ExitCode != 0 || string(res.Stdout) != "012345" || string(res.Stderr) != "err-ms" || !res.Truncated {
			t.Errorf("got code=%d stdout=%q stderr=%q truncated=%v", res.ExitCode, res.Stdout, res.Stderr, res.Truncated)
		}
		if !strings.Contains(live.String(), "0123456789abcdef") || !strings.Contains(live.String(), "err-msg") {
			t.Errorf("Output did not receive full streams: %q", live.String())
		}
	})

	t.Run("non-zero exit is a result", func(t *testing.T) {
		res, err := (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/sh", "-c", "printf partial; exit 7"},
			Dir:      t.TempDir(),
			Disclaim: true,
		})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if res.ExitCode != 7 || string(res.Stdout) != "partial" {
			t.Errorf("got code=%d stdout=%q", res.ExitCode, res.Stdout)
		}
	})

	t.Run("stdin reader and file", func(t *testing.T) {
		res, err := (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/cat"},
			Dir:      t.TempDir(),
			Stdin:    strings.NewReader("from-reader"),
			Disclaim: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if string(res.Stdout) != "from-reader" {
			t.Errorf("reader stdin: got %q, want %q", res.Stdout, "from-reader")
		}

		dir := t.TempDir()
		fPath := filepath.Join(dir, "in.txt")
		if err := os.WriteFile(fPath, []byte("from-file"), 0o600); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(fPath)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		res, err = (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/cat"},
			Dir:      dir,
			Stdin:    f,
			Disclaim: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if string(res.Stdout) != "from-file" {
			t.Errorf("file stdin: got %q, want %q", res.Stdout, "from-file")
		}
	})

	t.Run("env dedup and nil inheritance", func(t *testing.T) {
		res, err := (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/sh", "-c", `printf '%s|%s' "$FOO" "$KEEP"`},
			Env:      []string{"FOO=first", "KEEP=yes", "FOO=second"},
			Dir:      t.TempDir(),
			Disclaim: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if string(res.Stdout) != "second|yes" {
			t.Errorf("dedup env: got %q, want %q", res.Stdout, "second|yes")
		}

		t.Setenv("HEARTH_DISCLAIM_INHERIT", "inherited-ok")
		res, err = (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/sh", "-c", `printf '%s' "$HEARTH_DISCLAIM_INHERIT"`},
			Dir:      t.TempDir(),
			Disclaim: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if string(res.Stdout) != "inherited-ok" {
			t.Errorf("nil env inheritance: got %q, want %q", res.Stdout, "inherited-ok")
		}
	})

	t.Run("dir valid, relative executable, and invalid dir", func(t *testing.T) {
		dir := t.TempDir()
		script := filepath.Join(dir, "run.sh")
		if err := os.WriteFile(script, []byte("#!/bin/sh\npwd\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		res, err := (Exec{}).Run(ctx, Cmd{
			Args:     []string{"./run.sh"},
			Dir:      dir,
			Disclaim: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		want, _ := filepath.EvalSymlinks(dir)
		got, _ := filepath.EvalSymlinks(strings.TrimSpace(string(res.Stdout)))
		if got != want {
			t.Errorf("relative script in Dir: got %q, want %q", got, want)
		}

		_, err = (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/sh", "-c", "true"},
			Dir:      filepath.Join(dir, "does-not-exist"),
			Disclaim: true,
		})
		if err == nil {
			t.Error("invalid Dir succeeded, want error")
		}
	})

	t.Run("setsid own process group", func(t *testing.T) {
		res, err := (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/sh", "-c", `echo $$ $(ps -o pgid= -p $$)`},
			Dir:      t.TempDir(),
			Disclaim: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		f := strings.Fields(string(res.Stdout))
		if len(f) != 2 || f[0] != f[1] {
			t.Errorf("not in its own process group: %q", res.Stdout)
		}
	})

	t.Run("wait delay unblocks when grandchild holds pipe", func(t *testing.T) {
		start := time.Now()
		res, err := (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/sh", "-c", "/bin/sleep 5 & echo ready"},
			Dir:      t.TempDir(),
			Timeout:  5 * time.Second,
			Disclaim: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d >= 4*time.Second {
			t.Errorf("WaitDelay did not cut off inherited pipe: took %v", d)
		}
		if strings.TrimSpace(string(res.Stdout)) != "ready" {
			t.Errorf("stdout = %q, want ready", res.Stdout)
		}
	})

	t.Run("timeout and canceled", func(t *testing.T) {
		start := time.Now()
		res, err := (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/sleep", "10"},
			Dir:      t.TempDir(),
			Timeout:  150 * time.Millisecond,
			Disclaim: true,
		})
		if !errors.Is(err, context.DeadlineExceeded) || res.ExitCode != -1 || time.Since(start) > 3*time.Second {
			t.Errorf("timeout: code=%d err=%v elapsed=%v", res.ExitCode, err, time.Since(start))
		}

		cctx, cancel := context.WithCancel(ctx)
		cancel()
		res, err = (Exec{}).Run(cctx, Cmd{
			Args:     []string{"/bin/sh", "-c", "true"},
			Dir:      t.TempDir(),
			Disclaim: true,
		})
		if !errors.Is(err, context.Canceled) || res.ExitCode != -1 {
			t.Errorf("canceled: code=%d err=%v", res.ExitCode, err)
		}
	})

	t.Run("missing program and nul byte errors", func(t *testing.T) {
		_, err := (Exec{}).Run(ctx, Cmd{
			Args:     []string{"hearth-no-such-program"},
			Dir:      t.TempDir(),
			Disclaim: true,
		})
		if !errors.Is(err, exec.ErrNotFound) {
			t.Errorf("missing program: err=%v, want ErrNotFound", err)
		}

		_, err = (Exec{}).Run(ctx, Cmd{
			Args:     []string{"/bin/sh", "-c", "bad\x00arg"},
			Dir:      t.TempDir(),
			Disclaim: true,
		})
		if !errors.Is(err, syscall.EINVAL) {
			t.Errorf("nul arg: err=%v, want EINVAL", err)
		}
	})
}

func TestDedupEnv(t *testing.T) {
	got := dedupEnv([]string{"A=1", "B=2", "A=3", "NOEQ", "B=4"})
	want := []string{"A=3", "B=4", "NOEQ"}
	if !slices.Equal(got, want) {
		t.Errorf("dedupEnv = %q, want %q", got, want)
	}
}
