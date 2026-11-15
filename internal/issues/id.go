package issues

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// ID identifies the issue's target and observed versions without exposing secrets.
// A changed or removed issue must be requested again from the current snapshot.
func ID(is Issue) string {
	parts := []string{string(is.Kind), is.Tool, is.Project, is.Check.File, is.Check.Want}
	parts = append(parts, is.Dirs...)
	for _, in := range is.Copies {
		parts = append(parts, in.Path, in.Version)
	}
	for _, s := range is.MCP {
		parts = append(parts, s.Agent, s.Name, s.File)
	}
	for _, s := range is.Skills {
		parts = append(parts, s.Name, s.Dir)
	}
	if is.Agent != nil {
		parts = append(parts, is.Agent.ID)
	}
	if is.Rule != nil {
		parts = append(parts, is.Rule.Agent)
	}
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
