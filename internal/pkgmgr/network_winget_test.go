package pkgmgr

import (
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/versions"
)

func TestWingetTable(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, header, row string
		invalid           bool
	}{
		{"English", "Name               Id                 Version     Available", "Example CLI        Example.CLI        1.0         2.0", false},
		{"Russian", "Имя                ИД                 Версия      Доступно", "Example CLI        Example.CLI        1.0         2.0", false},
		{"CJK", "名称               ID                 版本        可用", "Example CLI        Example.CLI        1.0         2.0", false},
		{"no updates", "Name               Id                 Version", "Example CLI        Example.CLI        1.0", false},
		{"truncated id", "Name               Id                 Version", "Example CLI        Example…           1.0", true},
		{"truncated version", "Name               Id                 Version", "Example CLI        Example.CLI        1…", true},
		{"bad row", "Name               Id                 Version", "bad row", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := wingetTable([]byte(tt.header + "\n" + strings.Repeat("-", 70) + "\n" + tt.row + "\n"))
			if (err != nil) != tt.invalid {
				t.Fatalf("%+v, %v", got, err)
			}
			if !tt.invalid && (len(got) != 1 || got[0].Name != "Example.CLI" || got[0].Version != "1.0") {
				t.Fatal(got)
			}
		})
	}
	if _, err := wingetTable([]byte("Source failed\nName               Id                 Version\n----------------------------------------------\nExample CLI        Example.CLI        1.0\n")); err == nil {
		t.Fatal("source warning became a complete result")
	}
}

func TestWingetPartialSources(t *testing.T) {
	t.Parallel()
	f := &run.Fake{}
	f.Stdout("{\"Name\":\"winget\"}\n{\"Name\":\"private\"}", "winget", "source", "export", "--disable-interactivity")
	f.Stdout("Name               Id                 Version     Available\n------------------------------------------------------------------\nExample CLI        Example.CLI        1.0         2.0\n", "winget", "list", "--source", "winget", "--disable-interactivity", "--authentication-mode", "silent")
	// The private source is unavailable. Keep the other source, but do not
	// report its package or update counts as complete.
	got := Network(t.Context(), &versions.Cache{Dir: t.TempDir()}, f, nil, inventory.PackageManager{ID: "winget", Path: "winget", Unlisted: true})
	if got.UpdatesKnown || !got.Unlisted || got.UpdateError == "" || len(got.Packages) != 1 || got.Packages[0].Available != "2.0" {
		t.Fatalf("%+v", got)
	}
}
