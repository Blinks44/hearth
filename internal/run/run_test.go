package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// The test binary doubles as the command under test: with HEARTH_HELPER
// set, TestMain acts as the helper instead of running the tests.
func TestMain(m *testing.M) {
	switch os.Getenv("HEARTH_HELPER") {
	case "":
		os.Exit(m.Run())
	case "echo":
		fmt.Print(os.Getenv("HEARTH_SAY"))
		fmt.Fprint(os.Stderr, "to stderr")
		os.Exit(0)
	case "exit3":
		fmt.Print("partial")
		os.Exit(3)
	case "sleep":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "lines":
		fmt.Print("one\ntwo\n")
		os.Exit(0)
	case "flood":
		fmt.Print(strings.Repeat("x", 10_000))
		os.Exit(0)
	case "pwd":
		wd, _ := os.Getwd()
		fmt.Print(wd)
		os.Exit(0)
	}
	os.Exit(2)
}

func helper(mode string, env ...string) Cmd {
	// Under -race the helper is a race build, which sleeps a second at a
	// zero exit unless told not to.
	return Cmd{
		Args: []string{os.Args[0]},
		Env:  append([]string{"HEARTH_HELPER=" + mode, "GORACE=atexit_sleep_ms=0"}, env...),
	}
}

func TestExecRun(t *testing.T) {
	ctx := context.Background()

	t.Run("output and env", func(t *testing.T) {
		res, err := Exec{}.Run(ctx, helper("echo", "HEARTH_SAY=hello"))
		if err != nil {
			t.Fatal(err)
		}
		if string(res.Stdout) != "hello" || string(res.Stderr) != "to stderr" || res.ExitCode != 0 {
			t.Errorf("got stdout %q stderr %q code %d", res.Stdout, res.Stderr, res.ExitCode)
		}
	})

	t.Run("non-zero exit is a result", func(t *testing.T) {
		res, err := Exec{}.Run(ctx, helper("exit3"))
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if res.ExitCode != 3 || string(res.Stdout) != "partial" {
			t.Errorf("got code %d stdout %q", res.ExitCode, res.Stdout)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		c := helper("sleep")
		c.Timeout = 200 * time.Millisecond
		start := time.Now()
		_, err := Exec{}.Run(ctx, c)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want DeadlineExceeded", err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("Run took %v after a 200ms timeout", d)
		}
	})

	t.Run("canceled", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := Exec{}.Run(cctx, helper("echo"))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want Canceled", err)
		}
	})

	t.Run("output is capped", func(t *testing.T) {
		c := helper("flood")
		c.MaxOutput = 100
		res, err := Exec{}.Run(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Stdout) != 100 || !res.Truncated {
			t.Errorf("kept %d bytes, truncated %v; want 100, true", len(res.Stdout), res.Truncated)
		}
	})

	t.Run("output goes to Output too", func(t *testing.T) {
		c := helper("flood")
		c.MaxOutput = 100
		var out syncBuffer
		c.Output = &out
		if _, err := (Exec{}).Run(ctx, c); err != nil {
			t.Fatal(err)
		}
		if n := len(out.String()); n != 10_000 {
			t.Errorf("Output got %d bytes, want all 10000", n)
		}
	})

	t.Run("working directory", func(t *testing.T) {
		dir := t.TempDir()
		c := helper("pwd")
		c.Dir = dir
		res, err := Exec{}.Run(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := filepath.EvalSymlinks(dir)
		got, _ := filepath.EvalSymlinks(string(res.Stdout))
		if got != want {
			t.Errorf("ran in %q, want %q", got, want)
		}
	})

	t.Run("missing program", func(t *testing.T) {
		_, err := Exec{}.Run(ctx, Cmd{Args: []string{"hearth-no-such-program"}, Env: []string{"PATH=" + t.TempDir()}})
		if !errors.Is(err, exec.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("empty argv", func(t *testing.T) {
		if _, err := (Exec{}).Run(ctx, Cmd{}); err == nil {
			t.Error("empty argv ran")
		}
	})
}

func TestFake(t *testing.T) {
	var f Fake
	f.Stdout("v22.12.0\n", "node", "--version")
	res, err := f.Run(context.Background(), Cmd{Args: []string{"node", "--version"}})
	if err != nil || string(res.Stdout) != "v22.12.0\n" {
		t.Errorf("got %q, %v", res.Stdout, err)
	}
	if _, err := f.Run(context.Background(), Cmd{Args: []string{"bun", "--version"}}); !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("unknown argv: err = %v, want ErrNotFound", err)
	}
	if n := len(f.Calls()); n != 2 {
		t.Errorf("recorded %d calls, want 2", n)
	}
}

func TestLines(t *testing.T) {
	var got []string
	if err := Lines(t.Context(), helper("lines"), func(l string) { got = append(got, l) }); err != nil || strings.Join(got, ",") != "one,two" {
		t.Fatalf("%q %v", got, err)
	}
	if err := Lines(t.Context(), helper("exit3"), func(string) {}); err == nil {
		t.Error("exit 3: no error")
	}
	var stderr syncBuffer
	c := helper("echo", "HEARTH_SAY=hello")
	c.Output = &stderr
	got = nil
	if err := Lines(t.Context(), c, func(l string) { got = append(got, l) }); err != nil || strings.Join(got, ",") != "hello" || stderr.String() != "to stderr" {
		t.Errorf("stdout %q, stderr %q, %v", got, stderr.String(), err)
	}
	c = helper("flood")
	c.MaxOutput = 100
	if err := Lines(t.Context(), c, func(string) {}); err == nil {
		t.Error("long line: no error")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := Lines(ctx, helper("sleep"), func(string) {}); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 5*time.Second {
		t.Errorf("canceled: %v after %v", err, time.Since(start))
	}
}

// syncBuffer is a buffer two goroutines may write at once.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
