//go:build unix

package ops

import (
	"time"

	"github.com/Blinks44/hearth/locales"
)

// stopProcess asks the process to quit with SIGTERM, which a server
// takes as Ctrl+C: it closes what it has open first.
func stopProcess(pid string) Step {
	return Step{Argv: []string{"kill", "-TERM", pid}, Timeout: 30 * time.Second}
}

func processWhy() string { return locales.ProcessWhyUnix() }
