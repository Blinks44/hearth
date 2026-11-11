package versions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Cycle is a release line on endoflife.date, as Python's 3.13.
type Cycle struct {
	Cycle  string // as "3.13"
	Latest string // its newest release, as "3.13.1"
	// Release is when the line came out; End is when its security fixes
	// end.
	Release, End time.Time
	// LTS reports a line with long-term support, as Java's 25.
	LTS bool
}

// Cycles is what is known of a tool's release lines.
type Cycles struct {
	// Lines come newest first.
	Lines []Cycle
	// Fetched is when the list was last got from the server.
	Fetched time.Time
}

// fetchCycles gets the release lines of tool at url through c, cached as
// file.
func fetchCycles(ctx context.Context, c *Cache, tool, file, url string) (Cycles, error) {
	data, fetched, err := c.Get(ctx, file, url)
	if err != nil {
		return Cycles{}, err
	}
	lines, err := parseCycles(tool, data)
	if err != nil {
		return Cycles{}, err
	}
	return Cycles{Lines: lines, Fetched: fetched}, nil
}

// parseCycles reads endoflife.date's list of tool's lines.
func parseCycles(tool string, data []byte) ([]Cycle, error) {
	var cycles []struct {
		Cycle, Latest, ReleaseDate string
		EOL                        any // a date, or false
		LTS                        any // a bool, or the date support became long-term
	}
	if err := json.Unmarshal(data, &cycles); err != nil {
		return nil, fmt.Errorf("read %s release lines: %w", tool, err)
	}
	lines := make([]Cycle, 0, len(cycles))
	for _, c := range cycles {
		eol, _ := c.EOL.(string)
		lts, _ := c.LTS.(bool)
		if since, ok := c.LTS.(string); ok {
			lts = since != ""
		}
		lines = append(lines, Cycle{Cycle: c.Cycle, Latest: c.Latest, Release: date(c.ReleaseDate), End: date(eol), LTS: lts})
	}
	return lines, nil
}

// Line returns the line of version, as 3.13 of 3.13.1, and whether it is
// known.
func (p *Cycles) Line(version string) (Cycle, bool) {
	for _, l := range p.Lines {
		if version == l.Cycle || len(version) > len(l.Cycle) && version[:len(l.Cycle)+1] == l.Cycle+"." {
			return l, true
		}
	}
	return Cycle{}, false
}

// Newest is the newest release of the newest line out at now and still
// supported; "" when none is known.
func (p *Cycles) Newest(now time.Time) string {
	return p.newest(now, false)
}

// NewestLTS is Newest among the lines with long-term support.
func (p *Cycles) NewestLTS(now time.Time) string {
	return p.newest(now, true)
}

func (p *Cycles) newest(now time.Time, lts bool) string {
	if p == nil {
		return ""
	}
	for _, l := range p.Lines {
		if l.Latest != "" && (!lts || l.LTS) && (l.End.IsZero() || now.Before(l.End)) && (l.Release.IsZero() || !now.Before(l.Release)) {
			return l.Latest
		}
	}
	return ""
}
