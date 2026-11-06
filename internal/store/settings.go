package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	settingsFile = "settings.json"
	// settingsFormat is the version of settings.json this build writes.
	// A change that older builds would misread takes the next number.
	settingsFormat = 1
)

// Settings is what the user chose in Hearth, kept in PathUserData. The
// language is not here: mygo keeps the one App.SetLocale chose.
type Settings struct {
	Format int `json:"format"`
	// Theme is "light", "dark" or "system".
	Theme string `json:"theme"`
	// VersionLists allows release lists and package-manager availability
	// queries, which may refresh manager metadata.
	VersionLists bool      `json:"versionLists"`
	Projects     []Project `json:"projects"`
	// Suggested is set once Hearth offered the projects in the usual code
	// folders, so that it offers them only on the first start.
	Suggested bool `json:"suggested,omitempty"`
}

// Project is a folder the user added to Hearth.
type Project struct {
	Path string `json:"path"`
}

// DefaultSettings is what a new user starts with. A field that a file
// lacks keeps its value from here.
func DefaultSettings() Settings {
	return Settings{Format: settingsFormat, Theme: "system", VersionLists: true}
}

// LoadSettings reads the settings SaveSettings kept in dir, or returns
// DefaultSettings when there are none. A file this build cannot read,
// broken or written by a newer Hearth, is moved aside to settings.json.bad,
// so that saving does not lose it, and the error says so beside the
// defaults.
func LoadSettings(dir string) (Settings, error) {
	s := DefaultSettings()
	path := filepath.Join(dir, settingsFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read settings: %w", err)
	}
	if err = json.Unmarshal(data, &s); err == nil && s.Format > settingsFormat {
		err = fmt.Errorf("format %d is newer than %d", s.Format, settingsFormat)
	}
	if err == nil {
		s.Format = settingsFormat
		return s, nil
	}
	if rerr := os.Rename(path, path+".bad"); rerr != nil {
		return DefaultSettings(), fmt.Errorf("read settings: %w (keep it aside: %w)", err, rerr)
	}
	return DefaultSettings(), fmt.Errorf("read settings, kept as %s.bad: %w", settingsFile, err)
}

// SaveSettings keeps s in dir, replacing the file whole.
func SaveSettings(dir string, s Settings) error {
	s.Format = settingsFormat
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	if err := writeFile(dir, settingsFile, data); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}
