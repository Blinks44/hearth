package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// SetName is the name a set's file is offered to be saved as.
const SetName = "hearth-set.json"

// setVersion is the version of a set's file Hearth writes and reads.
const setVersion = 1

// setFile is a set as saved: a file a person can read, which says it is
// Hearth's.
type setFile struct {
	Hearth  string   `json:"hearth"`
	Version int      `json:"version"`
	Tools   []string `json:"tools"`
}

// MarshalSet returns the file of a set: the catalog's tools a computer
// has, which another installs.
func MarshalSet(tools []string) ([]byte, error) {
	data, err := json.MarshalIndent(setFile{Hearth: "set", Version: setVersion, Tools: tools}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("write set: %w", err)
	}
	return append(data, '\n'), nil
}

// ErrNotSet tells that a file is not a set Hearth saved, and ErrNewerSet
// that a newer Hearth saved it.
var (
	ErrNotSet   = errors.New("not a Hearth set")
	ErrNewerSet = errors.New("set from a newer Hearth")
)

// ParseSet returns the tools of a set's file in the catalog's order,
// without those the catalog does not know, as a newer Hearth's.
func ParseSet(data []byte) ([]string, error) {
	var f setFile
	if err := json.Unmarshal(data, &f); err != nil || f.Hearth != "set" {
		return nil, ErrNotSet
	}
	if f.Version > setVersion {
		return nil, fmt.Errorf("set version %d: %w", f.Version, ErrNewerSet)
	}
	var tools []string
	for _, e := range Tools {
		if slices.Contains(f.Tools, e.ID) {
			tools = append(tools, e.ID)
		}
	}
	return tools, nil
}
