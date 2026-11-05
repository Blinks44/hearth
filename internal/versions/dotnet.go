package versions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Dotnet lists SDK releases from Microsoft's release index.
type Dotnet struct {
	Lines   []DotnetLine
	Fetched time.Time
}

// DotnetLine is the newest SDK and support status of one release line.
type DotnetLine struct {
	Version string
	Support string
	Phase   string
	End     string
}

// FetchDotnet uses the shared cache for Microsoft's published release index.
func FetchDotnet(ctx context.Context, c *Cache) (Dotnet, error) {
	data, fetched, err := c.Get(ctx, "dotnet-index.json", "https://builds.dotnet.microsoft.com/dotnet/release-metadata/releases-index.json")
	if err != nil {
		return Dotnet{}, err
	}
	lines, err := parseDotnet(data)
	return Dotnet{Lines: lines, Fetched: fetched}, err
}

func parseDotnet(data []byte) ([]DotnetLine, error) {
	var doc struct {
		Releases []struct {
			SDK     string `json:"latest-sdk"`
			Support string `json:"release-type"`
			Phase   string `json:"support-phase"`
			End     string `json:"eol-date"`
		} `json:"releases-index"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("read .NET releases: %w", err)
	}
	if len(doc.Releases) == 0 {
		return nil, fmt.Errorf("empty .NET release index")
	}
	var lines []DotnetLine
	for _, r := range doc.Releases {
		if r.SDK != "" {
			lines = append(lines, DotnetLine{r.SDK, r.Support, r.Phase, r.End})
		}
	}
	return lines, nil
}
