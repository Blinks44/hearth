//go:build !windows

package ops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/Blinks44/hearth/locales"
)

const dockerDefaultSocket = "/var/run/docker.sock"

// DockerDefaultSocketPlan builds a confirmed plan that symlinks
// /var/run/docker.sock to the selected local Docker context's Unix socket so
// IDEs, Testcontainers, and tools that ignore Docker contexts use that engine.
func DockerDefaultSocketPlan(contextName, endpoint string, detect Detect) (Plan, bool) {
	if !contextRef.MatchString(contextName) {
		return Plan{}, false
	}
	socketPath, ok := strings.CutPrefix(endpoint, "unix://")
	if !ok || !filepath.IsAbs(socketPath) || strings.ContainsAny(socketPath, "\x00\r\n") {
		return Plan{}, false
	}
	socketPath = filepath.Clean(socketPath)
	if socketPath == dockerDefaultSocket {
		return Plan{}, false
	}
	p := Plan{
		Title: locales.DockerSocketLinkTitle(contextName),
		Why:   locales.DockerSocketLinkWhy(contextName),
		Steps: []Step{{
			Argv:    []string{"ln", "-sfn", socketPath, dockerDefaultSocket},
			Admin:   true,
			Timeout: 30 * time.Second,
		}},
		Changes: []string{dockerDefaultSocket + " → " + socketPath},
		Notes: []Note{
			AdminNote(),
			{Title: locales.DockerSocketLinkNoteTitle(), Text: locales.DockerSocketLinkNoteText()},
		},
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			if st.Docker == nil || st.Docker.DefaultSocketIssue != "" {
				return "", errors.New(locales.DockerSocketLinkFailed(contextName))
			}
			return locales.DockerSocketLinkVerified(contextName), nil
		}
	}
	return p, true
}
