package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/history"
)

const historyFile = "history.json"

// LoadHistory reads the changes of the last checks SaveHistory kept in
// dir, the newest first. Without them the error is os.ErrNotExist.
func LoadHistory(dir string) ([]history.Entry, error) {
	var entries []history.Entry
	data, err := os.ReadFile(filepath.Join(dir, historyFile))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("read history: %w", err)
	}
	return entries, nil
}

// SaveHistory keeps entries in dir, replacing the ones there whole.
func SaveHistory(dir string, entries []history.Entry) error {
	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encode history: %w", err)
	}
	if err := writeFile(dir, historyFile, data); err != nil {
		return fmt.Errorf("save history: %w", err)
	}
	return nil
}
