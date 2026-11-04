package store

import (
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadSnapshot(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty cache: %v, want os.ErrNotExist", err)
	}
	want := inventory.Snapshot{
		Taken: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
		Tools: []inventory.ToolState{
			{Tool: "node", Installations: []inventory.Installation{{
				Tool: "node", Version: "22.12.0", Path: "/opt/homebrew/bin/node", OnPath: 1,
				Source: inventory.Source{Kind: inventory.KindPackageManager, Manager: "brew"},
			}}},
			{Tool: "bun", Err: "timed out"},
		},
	}
	for range 2 { // the second save replaces the first
		if err := SaveSnapshot(dir, want); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Taken.Equal(want.Taken) || !reflect.DeepEqual(got.Tools, want.Tools) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("cache holds %d files, want the snapshot alone", len(entries))
	}
}
