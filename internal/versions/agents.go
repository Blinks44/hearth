package versions

import (
	"context"
	"slices"
	"strings"
	"time"
)

// Agents holds the latest npm registry release for npm-packaged AI agents,
// keyed by agent ID (as "claude", "codex", "gemini").
type Agents struct {
	Latest  map[string]Latest
	Fetched time.Time
}

// agentRegistryURL is the npm registry base URL; tests override it.
var agentRegistryURL = "https://registry.npmjs.org"

// FetchAgents gets the latest stable npm release for each agent in packages
// (agent ID -> npm package name) through c.
func FetchAgents(ctx context.Context, c *Cache, packages map[string]string) (Agents, error) {
	out := Agents{Latest: map[string]Latest{}}
	ids := make([]string, 0, len(packages))
	for id, pkg := range packages {
		if pkg != "" {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	var firstErr error
	for _, id := range ids {
		pkg := packages[id]
		cacheFile := "agent-" + strings.NewReplacer("/", "-", "@", "").Replace(id) + "-latest.json"
		data, at, err := c.Get(ctx, cacheFile, agentRegistryURL+"/"+pkg+"/latest")
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		v, err := ParseNpmLatest(data)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out.Latest[id] = Latest{Version: v, Fetched: at}
		if !at.IsZero() && (out.Fetched.IsZero() || at.Before(out.Fetched)) {
			out.Fetched = at
		}
	}
	if len(out.Latest) == 0 && firstErr != nil {
		return Agents{}, firstErr
	}
	return out, firstErr
}
