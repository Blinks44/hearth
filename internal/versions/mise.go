package versions

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Mise is what mise outdated says of the tools mise installed.
type Mise struct {
	// Tools are by mise's name of a tool, as "jq" or "npm:prettier".
	Tools map[string]MiseTool
	// Fetched is when mise was last asked.
	Fetched time.Time
}

// MiseTool is the newer versions of one tool of mise's.
type MiseTool struct {
	// Current is the version installed, as "1.7.1".
	Current string
	// InRange is the newest release the version its config asks for
	// allows, as the newest 1.7.x for "1.7"; "" for a tool no config asks
	// for, or one up to date.
	InRange string `json:",omitempty"`
	// Latest is the newest release overall; "" when Current is.
	Latest string `json:",omitempty"`
}

// FetchMise asks mise, at path mise with the terminal's env, which of its
// tools have newer versions, through c: once a day, and only from what is
// kept while c is offline. installed are its tools' versions, as
// "jq@1.7.1": mise is asked again once they change.
func FetchMise(ctx context.Context, c *Cache, r run.Runner, env sysenv.Env, mise string, installed []string) (Mise, error) {
	installed = slices.Sorted(slices.Values(installed))
	key, _ := json.Marshal([]any{1, mise, installed})
	data, fetched, err := c.Query(ctx, fmt.Sprintf("mise-outdated-%x.json", sha256.Sum256(key)), func(ctx context.Context) ([]byte, error) {
		inRange, err := miseOutdated(ctx, r, env, mise)
		if err != nil {
			return nil, err
		}
		latest, err := miseOutdated(ctx, r, env, mise, "--bump")
		if err != nil {
			return nil, err
		}
		return json.Marshal(mergeMise(inRange, latest))
	})
	if len(data) == 0 {
		return Mise{}, err
	}
	m := Mise{Fetched: fetched}
	if e := json.Unmarshal(data, &m.Tools); e != nil {
		return Mise{}, fmt.Errorf("read kept mise outdated: %w", e)
	}
	return m, err
}

// miseOutdated runs mise outdated over every tool mise installed, from
// the home folder so that no project's config counts.
func miseOutdated(ctx context.Context, r run.Runner, env sysenv.Env, mise string, flags ...string) (map[string]miseEntry, error) {
	args := append([]string{mise, "outdated", "--json", "--inactive"}, flags...)
	res, err := r.Run(ctx, run.Cmd{Args: args, Env: env, Dir: env.Get("HOME"), Timeout: 2 * time.Minute, MaxOutput: 4 << 20})
	if err != nil {
		return nil, fmt.Errorf("mise outdated: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("mise outdated exited %d: %s", res.ExitCode, strings.TrimSpace(string(res.Stderr)))
	}
	return parseMiseOutdated(res.Stdout)
}

type miseEntry struct {
	Current, Latest string
	Source          struct{ Type string }
}

// parseMiseOutdated reads mise outdated --json.
func parseMiseOutdated(data []byte) (map[string]miseEntry, error) {
	var tools map[string]miseEntry
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("read mise outdated: %w", err)
	}
	return tools, nil
}

// mergeMise puts together mise outdated without --bump, whose latest is
// the newest release a config's version allows, and with it, whose
// latest is the newest overall. A tool no config asks for gets the newest
// overall either way, so it has no InRange.
func mergeMise(inRange, latest map[string]miseEntry) map[string]MiseTool {
	out := map[string]MiseTool{}
	for name, e := range latest {
		out[name] = MiseTool{Current: e.Current, Latest: e.Latest}
	}
	for name, e := range inRange {
		if e.Source.Type == "unknown" {
			continue
		}
		t := out[name]
		t.Current, t.InRange = e.Current, e.Latest
		out[name] = t
	}
	return out
}
