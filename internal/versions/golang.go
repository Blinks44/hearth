package versions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GoReleasesURL lists the newest release of each supported Go line.
const GoReleasesURL = "https://go.dev/dl/?mode=json"

// Go is what go.dev says of Go's versions.
type Go struct {
	// Releases are the newest of each supported line, newest first,
	// without the go: "1.27.3", "1.26.10".
	Releases []string
	// Fetched is when the list was last got from the server.
	Fetched time.Time
}

// FetchGo gets Go's supported releases through c.
func FetchGo(ctx context.Context, c *Cache) (Go, error) {
	data, fetched, err := c.Get(ctx, "go-releases.json", GoReleasesURL)
	if err != nil {
		return Go{}, err
	}
	releases, err := ParseGoReleases(data)
	if err != nil {
		return Go{}, err
	}
	return Go{Releases: releases, Fetched: fetched}, nil
}

// ParseGoReleases reads go.dev's list of releases, the stable ones.
func ParseGoReleases(data []byte) ([]string, error) {
	var list []struct {
		Version string
		Stable  bool
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("read Go releases: %w", err)
	}
	var releases []string
	for _, r := range list {
		if r.Stable && strings.HasPrefix(r.Version, "go") {
			releases = append(releases, strings.TrimPrefix(r.Version, "go"))
		}
	}
	return releases, nil
}
