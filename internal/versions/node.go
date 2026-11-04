// Package versions knows the versions tools can be installed at, from the
// official online indexes. Cache keeps them, so that they show offline.
package versions

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Where Node's releases and the schedule of its release lines are.
const (
	NodeIndexURL    = "https://nodejs.org/dist/index.json"
	NodeScheduleURL = "https://raw.githubusercontent.com/nodejs/Release/main/schedule.json"
)

// Release is a version of a tool one can install.
type Release struct {
	// Version is without the v, as "22.12.0".
	Version string
	Date    string // as "2024-12-02"
	// LTS is the codename of the LTS line the release is on, as "Jod";
	// empty for a Current release.
	LTS string
	// Security reports a release that fixes vulnerabilities.
	Security bool
}

// Line is a release line, as Node 22: when it starts, becomes LTS, goes to
// maintenance and ends. A date it has none of is zero.
type Line struct {
	Start, LTS, Maintenance, End time.Time
	Codename                     string
}

// Node is what nodejs.org says of Node's versions.
type Node struct {
	// Releases come newest first, without prereleases.
	Releases []Release
	// Lines are by major version; the 0.x ones are left out.
	Lines map[int]Line
	// Fetched is when the lists were last got from the server.
	Fetched time.Time
}

// FetchNode gets Node's releases and schedule through c.
func FetchNode(ctx context.Context, c *Cache) (Node, error) {
	index, fetched, err := c.Get(ctx, "node-index.json", NodeIndexURL)
	if err != nil {
		return Node{}, err
	}
	schedule, scheduled, err := c.Get(ctx, "node-schedule.json", NodeScheduleURL)
	if err != nil {
		return Node{}, err
	}
	n := Node{Fetched: fetched}
	if scheduled.Before(fetched) {
		n.Fetched = scheduled
	}
	if n.Releases, err = ParseNodeIndex(index); err != nil {
		return Node{}, err
	}
	if n.Lines, err = ParseNodeSchedule(schedule); err != nil {
		return Node{}, err
	}
	return n, nil
}

// ParseNodeIndex reads nodejs.org/dist/index.json.
func ParseNodeIndex(data []byte) ([]Release, error) {
	var index []struct {
		Version  string
		Date     string
		LTS      any // false, or the codename
		Security bool
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("read Node release index: %w", err)
	}
	var releases []Release
	for _, r := range index {
		v := strings.TrimPrefix(r.Version, "v")
		if v == "" || strings.Contains(v, "-") {
			continue
		}
		codename, _ := r.LTS.(string)
		releases = append(releases, Release{Version: v, Date: r.Date, LTS: codename, Security: r.Security})
	}
	return releases, nil
}

// ParseNodeSchedule reads the schedule of Node's release lines,
// nodejs/Release's schedule.json.
func ParseNodeSchedule(data []byte) (map[int]Line, error) {
	var schedule map[string]struct {
		Start, LTS, Maintenance, End, Codename string
	}
	if err := json.Unmarshal(data, &schedule); err != nil {
		return nil, fmt.Errorf("read Node release schedule: %w", err)
	}
	lines := map[int]Line{}
	for key, s := range schedule {
		major, err := strconv.Atoi(strings.TrimPrefix(key, "v"))
		if err != nil {
			continue // a 0.x line
		}
		lines[major] = Line{
			Start:       date(s.Start),
			LTS:         date(s.LTS),
			Maintenance: date(s.Maintenance),
			End:         date(s.End),
			Codename:    s.Codename,
		}
	}
	return lines, nil
}

func date(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s) // none or malformed: zero
	return t
}

// Major is the major version of a version, as 22 of "22.12.0"; -1 when it
// has none.
func Major(version string) int {
	s, _, _ := strings.Cut(strings.TrimPrefix(version, "v"), ".")
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

// Latest returns the newest release of each line still supported at now,
// newest line first. A line the schedule does not know, as the 0.x ones,
// is left out.
func (n Node) Latest(now time.Time) []Release {
	var latest []Release
	seen := map[int]bool{}
	for _, r := range n.Releases {
		m := Major(r.Version)
		if seen[m] {
			continue
		}
		seen[m] = true
		if l, ok := n.Lines[m]; !ok || !l.End.IsZero() && !now.Before(l.End) {
			continue
		}
		latest = append(latest, r)
	}
	return latest
}

// Recommended returns the newest LTS release, and false when there is
// none.
func (n Node) Recommended() (Release, bool) {
	for _, r := range n.Releases {
		if r.LTS != "" {
			return r, true
		}
	}
	return Release{}, false
}
