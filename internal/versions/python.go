package versions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// PythonCyclesURL lists Python's release lines with their newest release
// and end of life. python.org's own release-cycle.json has the dates but
// not the newest releases.
const PythonCyclesURL = "https://endoflife.date/api/python.json"

// PythonLine is a Python release line, as 3.13.
type PythonLine struct {
	Cycle  string // as "3.13"
	Latest string // its newest release, as "3.13.1"
	// Release is when the line came out; End is when its security fixes
	// end.
	Release, End time.Time
}

// Python is what is known of Python's release lines.
type Python struct {
	// Lines come newest first.
	Lines []PythonLine
	// Fetched is when the list was last got from the server.
	Fetched time.Time
}

// FetchPython gets Python's release lines through c.
func FetchPython(ctx context.Context, c *Cache) (Python, error) {
	data, fetched, err := c.Get(ctx, "python-cycles.json", PythonCyclesURL)
	if err != nil {
		return Python{}, err
	}
	lines, err := ParsePythonCycles(data)
	if err != nil {
		return Python{}, err
	}
	return Python{Lines: lines, Fetched: fetched}, nil
}

// ParsePythonCycles reads endoflife.date's list of Python's lines.
func ParsePythonCycles(data []byte) ([]PythonLine, error) {
	var cycles []struct {
		Cycle, Latest, ReleaseDate string
		EOL                        any // a date, or false
	}
	if err := json.Unmarshal(data, &cycles); err != nil {
		return nil, fmt.Errorf("read Python release lines: %w", err)
	}
	lines := make([]PythonLine, 0, len(cycles))
	for _, c := range cycles {
		eol, _ := c.EOL.(string)
		lines = append(lines, PythonLine{Cycle: c.Cycle, Latest: c.Latest, Release: date(c.ReleaseDate), End: date(eol)})
	}
	return lines, nil
}

// Line returns the line of version, as 3.13 of 3.13.1, and whether it is
// known.
func (p *Python) Line(version string) (PythonLine, bool) {
	for _, l := range p.Lines {
		if version == l.Cycle || len(version) > len(l.Cycle) && version[:len(l.Cycle)+1] == l.Cycle+"." {
			return l, true
		}
	}
	return PythonLine{}, false
}
