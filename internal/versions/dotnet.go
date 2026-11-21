package versions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// Ended reports that the line's support has ended at now.
func (l DotnetLine) Ended(now time.Time) bool {
	if strings.EqualFold(l.Phase, "eol") {
		return true
	}
	end := date(l.End)
	return !end.IsZero() && !now.Before(end)
}

// Line returns the release line of version (matching major.minor) and whether
// it is known.
func (d *Dotnet) Line(version string) (DotnetLine, bool) {
	if d == nil {
		return DotnetLine{}, false
	}
	want := dotnetLineKey(version)
	if want == "" {
		return DotnetLine{}, false
	}
	for _, l := range d.Lines {
		if dotnetLineKey(l.Version) == want {
			return l, true
		}
	}
	return DotnetLine{}, false
}

func dotnetLineKey(v string) string {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
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
