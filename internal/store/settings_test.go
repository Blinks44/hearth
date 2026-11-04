package store

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "userData") // not made yet
	got, err := LoadSettings(dir)
	if err != nil || !reflect.DeepEqual(got, DefaultSettings()) {
		t.Fatalf("no file: %+v, %v; want the defaults", got, err)
	}
	want := Settings{
		Format: settingsFormat, Theme: "dark", VersionLists: false,
		Projects: []Project{{Path: "/Users/me/code/shop"}, {Path: "/Users/me/code/admin"}},
	}
	for range 2 { // the second save replaces the first
		if err := SaveSettings(dir, want); err != nil {
			t.Fatal(err)
		}
	}
	if got, err = LoadSettings(dir); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, %v; want %+v", got, err, want)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("dir holds %d files, want the settings alone", len(entries))
	}
}

func TestLoadSettings(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		want    Settings
		wantErr string // the file is then moved aside
	}{
		{
			name: "missing fields keep the defaults",
			file: `{"format": 1, "projects": [{"path": "/p"}]}`,
			want: Settings{Format: 1, Theme: "system", VersionLists: true, Projects: []Project{{Path: "/p"}}},
		},
		{
			name: "older format",
			file: `{"theme": "light"}`,
			want: Settings{Format: 1, Theme: "light", VersionLists: true},
		},
		{
			name:    "broken",
			file:    `{"theme": "light",`,
			want:    DefaultSettings(),
			wantErr: "unexpected end of JSON input",
		},
		{
			name:    "newer format",
			file:    `{"format": 2, "theme": "light"}`,
			want:    DefaultSettings(),
			wantErr: "format 2 is newer than 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, settingsFile)
			if err := os.WriteFile(path, []byte(tt.file), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := LoadSettings(dir)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err %v, want one with %q", err, tt.wantErr)
			}
			if kept, _ := os.ReadFile(path + ".bad"); string(kept) != tt.file {
				t.Errorf("settings.json.bad holds %q, want the old file", kept)
			}
			// Saving now does not lose the old file.
			if err := SaveSettings(dir, got); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path + ".bad"); err != nil {
				t.Error(err)
			}
		})
	}
}
