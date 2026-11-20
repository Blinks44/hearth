package containerlog

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Blinks44/hearth/internal/run"
)

const MaxLines = 10000

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

// LinesFunc gives each line produced by a command, as run.Lines does.
type LinesFunc func(context.Context, run.Cmd, func(string)) error

// Batch publishes batches serially, including the final lines before returning.
func Batch(read func(func(LogLine)) error, got func([]LogLine)) error {
	var mu sync.Mutex
	var batch []LogLine
	send := func() {
		mu.Lock()
		lines := batch
		batch = nil
		mu.Unlock()
		if len(lines) > 0 {
			got(lines)
		}
	}
	done := make(chan struct{})
	var worker sync.WaitGroup
	worker.Go(func() {
		tick := time.NewTicker(100 * time.Millisecond)
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
	err := read(func(line LogLine) {
		mu.Lock()
		batch = append(batch, line)
		mu.Unlock()
	})
	close(done)
	worker.Wait()
	send()
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

// ParseLine reads a line a container CLI with --timestamps wrote: its time, its
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
