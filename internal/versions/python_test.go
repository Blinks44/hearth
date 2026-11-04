package versions

import (
	"testing"
	"time"
)

func TestParsePythonCycles(t *testing.T) {
	data := `[{"cycle":"3.14","releaseDate":"2025-10-07","eol":"2030-10-31","latest":"3.14.0"},
		{"cycle":"3.13","releaseDate":"2024-10-07","eol":"2029-10-31","latest":"3.13.1"},
		{"cycle":"2.7","releaseDate":"2010-07-03","eol":false,"latest":"2.7.18"}]`
	lines, err := ParsePythonCycles([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	p := Python{Lines: lines}
	l, ok := p.Line("3.13.1")
	if !ok || l.Latest != "3.13.1" || !l.End.Equal(time.Date(2029, 10, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("line of 3.13.1: %+v, %v", l, ok)
	}
	// 3.1 is not the line of 3.13.1.
	if l, ok := p.Line("3.1.5"); ok {
		t.Errorf("line of 3.1.5: %+v", l)
	}
	if l, _ := p.Line("2.7.18"); !l.End.IsZero() {
		t.Errorf("2.7 end %v", l.End)
	}
	if _, err := ParsePythonCycles([]byte("{")); err == nil {
		t.Error("broken list parsed")
	}
}
