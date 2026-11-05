package pkgmgr

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseWingetRegistrations(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/winget-registrations.json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []map[string]string
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatal(err)
	}
	got := parseWingetRegistrations(entries)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].Name != "Example.CLI" || got[0].Version != "2.0" || got[0].Kind != "portable" || got[1].Version != "1.0" {
		t.Fatalf("%+v", got)
	}
}
