package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
)

const sizesFile = "sizes.json"

// LoadSizes reads the sizes SaveSizes kept in dir. Without them the error
// is os.ErrNotExist.
func LoadSizes(dir string) (inventory.Sizes, error) {
	var s inventory.Sizes
	data, err := os.ReadFile(filepath.Join(dir, sizesFile))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("read cached sizes: %w", err)
	}
	return s, nil
}

// SaveSizes keeps s in dir, replacing the ones there whole.
func SaveSizes(dir string, s inventory.Sizes) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode sizes: %w", err)
	}
	if err := writeFile(dir, sizesFile, data); err != nil {
		return fmt.Errorf("save sizes: %w", err)
	}
	return nil
}
