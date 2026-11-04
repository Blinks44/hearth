package store

import (
	"os"
	"path/filepath"
)

// writeFile replaces dir/name with data whole: a crash midway leaves the
// old file.
func writeFile(dir, name string, data []byte) error {
	f, err := os.CreateTemp(dir, name+".*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(dir, name))
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}
