package versions

import "testing"

func TestParsePHPCycles(t *testing.T) {
	lines, err := ParsePHPCycles([]byte(`[{"cycle":"8.4","latest":"8.4.3","releaseDate":"2024-11-21","eol":"2028-12-31"},{"cycle":"7.4","latest":"7.4.33","eol":"2022-11-28"}]`))
	if err != nil {
		t.Fatal(err)
	}
	p := PHP{Lines: lines}
	l, ok := p.Line("8.4.3")
	if !ok || l.Latest != "8.4.3" || l.End.Year() != 2028 {
		t.Fatalf("line %+v, %v", l, ok)
	}
	if _, ok := p.Line("8.40.0"); ok {
		t.Fatal("matched wrong line")
	}
	if _, err := ParsePHPCycles([]byte("{")); err == nil {
		t.Fatal("accepted broken response")
	}
}
