//go:build darwin

package ops

import (
	"time"

	"github.com/Blinks44/hearth/locales"
)

const (
	dockerSocketDaemonLabel = "dev.hearth.docker-socket"
	dockerSocketDaemonPath  = "/Library/LaunchDaemons/dev.hearth.docker-socket.plist"

	// macOS clears /private/var/run on every restart, so a plain symlink
	// disappears after reboot. Write a minimal root-owned one-shot LaunchDaemon
	// in the same elevated step (one password prompt) so launchd recreates the
	// symlink on boot without keeping a background process running.
	dockerSocketDarwinScript = `set -e
/bin/ln -sfn "$1" "$2"
/bin/mkdir -p /Library/LaunchDaemons
tmp="$3.tmp.$$"
trap '/bin/rm -f "$tmp"' EXIT
printf '%s' "$4" > "$tmp"
/usr/sbin/chown 0:0 "$tmp"
/bin/chmod 644 "$tmp"
/bin/mv -f "$tmp" "$3"
/bin/launchctl bootout "system/$5" >/dev/null 2>&1 || true
/bin/launchctl enable "system/$5" >/dev/null 2>&1 || true
/bin/launchctl bootstrap system "$3"`
)

func dockerSocketDaemonPlist(socketPath string) string {
	return launchAgent(dockerSocketDaemonLabel, []string{"/bin/ln", "-sfn", socketPath, dockerDefaultSocket})
}

func dockerDefaultSocketStep(socketPath string) (Step, []string, []Note) {
	plist := dockerSocketDaemonPlist(socketPath)
	step := Step{
		Argv: []string{
			"/bin/sh", "-c", dockerSocketDarwinScript, "sh",
			socketPath, dockerDefaultSocket, dockerSocketDaemonPath, plist, dockerSocketDaemonLabel,
		},
		Runs:    []string{"ln", "-sfn", socketPath, dockerDefaultSocket},
		Note:    locales.DockerSocketDaemonStep(),
		Admin:   true,
		Timeout: 30 * time.Second,
	}
	changes := []string{locales.DockerSocketDaemonChange(dockerSocketDaemonPath)}
	notes := []Note{{
		Title: locales.AppsPathLoginItem(),
		Text:  locales.DockerSocketDaemonNoteText(dockerSocketDaemonLabel),
	}}
	return step, changes, notes
}
