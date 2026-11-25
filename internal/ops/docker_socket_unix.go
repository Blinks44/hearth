//go:build !windows

package ops

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

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
	step, extraChanges, extraNotes := dockerDefaultSocketStep(socketPath)
	changes := append([]string{dockerDefaultSocket + " → " + socketPath}, extraChanges...)
	notes := append([]Note{
		AdminNote(),
		{Title: locales.DockerSocketLinkNoteTitle(), Text: locales.DockerSocketLinkNoteText()},
	}, extraNotes...)
	p := Plan{
		Title:   locales.DockerSocketLinkTitle(contextName),
		Why:     locales.DockerSocketLinkWhy(contextName),
		Steps:   []Step{step},
		Changes: changes,
		Notes:   notes,
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
