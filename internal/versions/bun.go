package versions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// BunLatestURL is the npm registry's newest stable release of Bun, which
// Bun publishes with every release.
const BunLatestURL = "https://registry.npmjs.org/bun/latest"

// Latest is a tool's newest stable version.
type Latest struct {
	// Version is without the v, as "1.2.8".
	Version string
	// Fetched is when it was last got from the server.
	Fetched time.Time
}

// FetchBun gets Bun's newest stable version through c.
func FetchBun(ctx context.Context, c *Cache) (Latest, error) {
	data, fetched, err := c.Get(ctx, "bun-latest.json", BunLatestURL)
	if err != nil {
		return Latest{}, err
	}
	v, err := ParseNpmLatest(data)
	if err != nil {
		return Latest{}, err
	}
	return Latest{Version: v, Fetched: fetched}, nil
}

// ParseNpmLatest reads the version of a package's latest release from the
// npm registry's answer.
func ParseNpmLatest(data []byte) (string, error) {
	var p struct{ Version string }
	if err := json.Unmarshal(data, &p); err != nil {
		return "", fmt.Errorf("read npm release: %w", err)
	}
	if p.Version == "" {
		return "", fmt.Errorf("read npm release: no version")
	}
	return p.Version, nil
}
