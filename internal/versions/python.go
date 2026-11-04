package versions

import "context"

// PythonCyclesURL lists Python's release lines with their newest release
// and end of life. python.org's own release-cycle.json has the dates but
// not the newest releases.
const PythonCyclesURL = "https://endoflife.date/api/python.json"

// PythonLine is a Python release line, as 3.13.
type PythonLine = Cycle

// Python is what is known of Python's release lines.
type Python = Cycles

// FetchPython gets Python's release lines through c.
func FetchPython(ctx context.Context, c *Cache) (Python, error) {
	return fetchCycles(ctx, c, "Python", "python-cycles.json", PythonCyclesURL)
}

// ParsePythonCycles reads endoflife.date's list of Python's lines.
func ParsePythonCycles(data []byte) ([]PythonLine, error) { return parseCycles("Python", data) }
