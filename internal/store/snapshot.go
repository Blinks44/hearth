// Package store keeps Hearth's files: the user's settings and projects,
// and the last snapshot, so that the window shows it at once on the next
// start.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
)

const snapshotFile = "snapshot.json"

// LoadSnapshot reads the snapshot SaveSnapshot kept in dir. Without one
// the error is os.ErrNotExist.
func LoadSnapshot(dir string) (inventory.Snapshot, error) {
	var s inventory.Snapshot
	data, err := os.ReadFile(filepath.Join(dir, snapshotFile))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("read cached snapshot: %w", err)
	}
	return s, nil
}

// SaveSnapshot keeps s in dir, replacing the one there whole: a crash
// midway leaves the old one.
func SaveSnapshot(dir string, s inventory.Snapshot) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	if err := WriteFile(dir, snapshotFile, data); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	return nil
}
