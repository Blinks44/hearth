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
