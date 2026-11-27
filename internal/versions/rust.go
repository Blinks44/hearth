package versions

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RustCyclesURL lists Rust's release lines with their newest release.
const RustCyclesURL = "https://endoflife.date/api/rust.json"

// Rust is what is known of Rust's release lines.
type Rust = Cycles

// FetchRust gets Rust's release lines through c.
func FetchRust(ctx context.Context, c *Cache) (Rust, error) {
	return fetchCycles(ctx, c, "Rust", "rust-cycles.json", RustCyclesURL)
}

// RustTrain is what each of Rust's channels ships now. A stable line
// comes every six weeks, from the beta before it, and beta from nightly,
// so beta and nightly are the next two lines.
type RustTrain struct {
	Stable, Beta, Nightly string
	// NextStable is when beta becomes stable.
	NextStable time.Time
}

// Train tells the channels from the newest stable line.
func (r *Rust) Train() (RustTrain, error) {
	if len(r.Lines) == 0 {
		return RustTrain{}, fmt.Errorf("no Rust release lines")
	}
	l := r.Lines[0]
	major, minor, ok := strings.Cut(l.Cycle, ".")
	n, err := strconv.Atoi(minor)
	if !ok || err != nil {
		return RustTrain{}, fmt.Errorf("read Rust line %q", l.Cycle)
	}
	t := RustTrain{
		Stable:  l.Latest,
		Beta:    fmt.Sprintf("%s.%d.0", major, n+1),
		Nightly: fmt.Sprintf("%s.%d.0", major, n+2),
	}
	if !l.Release.IsZero() {
		t.NextStable = l.Release.AddDate(0, 0, 42)
	}
	return t, nil
}

// RustEnded reports whether a stable Rust version should be flagged as
// end-of-life. Rust ships a new minor every six weeks and endoflife.date
// marks the previous minor EOL immediately, so the single minor right behind
// the current stable release is treated as a normal update rather than EOL.
func RustEnded(r *Rust, version string, now time.Time) bool {
	if r == nil || len(r.Lines) == 0 {
		return false
	}
	c, ok := r.Line(version)
	if !ok || !c.Ended(now) {
		return false
	}
	cur := r.Lines[0]
	if !cur.Ended(now) {
		curMaj, curMin, ok1 := parseMajorMinor(cur.Cycle)
		verMaj, verMin, ok2 := parseMajorMinor(c.Cycle)
		if ok1 && ok2 && verMaj == curMaj && verMin >= curMin-1 {
			return false
		}
	}
	return true
}

func parseMajorMinor(cycle string) (int, int, bool) {
	majorStr, minorStr, ok := strings.Cut(cycle, ".")
	if !ok {
		return 0, 0, false
	}
	major, err1 := strconv.Atoi(majorStr)
	minor, err2 := strconv.Atoi(minorStr)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}
