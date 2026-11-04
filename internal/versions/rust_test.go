package versions

import "testing"

func TestRustTrain(t *testing.T) {
	lines, err := parseCycles("Rust", []byte(`[{"cycle":"1.99","releaseDate":"2026-10-01","eol":false,"latest":"1.99.1"},{"cycle":"1.98","latest":"1.98.1","eol":"2026-10-01"}]`))
	if err != nil {
		t.Fatal(err)
	}
	r := Rust{Lines: lines}
	got, err := r.Train()
	if err != nil {
		t.Fatal(err)
	}
	if got.Stable != "1.99.1" || got.Beta != "1.100.0" || got.Nightly != "1.101.0" || got.NextStable.Format("2006-01-02") != "2026-11-12" {
		t.Fatalf("train %+v", got)
	}
	if _, err := (&Rust{}).Train(); err == nil {
		t.Fatal("train of no lines")
	}
}
