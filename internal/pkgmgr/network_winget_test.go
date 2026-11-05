package pkgmgr

import (
	"os"
	"path/filepath"
	"slices"
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

// A fresh Windows: the spinner shares the header's line, and the source does
// not know the MSIX and ARP entries, whose ids WinGet truncates.
func TestWingetTableFresh(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "winget-list-fresh.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := wingetTable(data)
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	for _, p := range got {
		rows = append(rows, p.Name+" "+p.Version+" "+p.Available)
	}
	want := []string{
		"Microsoft.OneDrive 22.176.0821.0002 26.183.0920.0005",
		"Microsoft.DotNet.Native.Runtime 2.2.28604.0 ",
		"Microsoft.DotNet.Native.Runtime 2.2.28604.0 ",
		"Microsoft.WindowsTerminal 1.18.10301.0 1.25.2733.0",
	}
	if !slices.Equal(rows, want) {
		t.Fatalf("rows %q", rows)
	}
}

func TestWingetPartialSources(t *testing.T) {
	t.Parallel()
	f := &run.Fake{}
	// winget-font is explicit: WinGet does not list from it unless named.
	f.Stdout("{\"Name\":\"winget\"}\n{\"Name\":\"private\"}\n{\"Name\":\"winget-font\",\"Explicit\":true}\n{\"Name\":\"empty\"}", "winget", "source", "export", "--disable-interactivity")
	// Nothing installed from it, said in the language of Windows.
	f.Stdout("Не найдены установленные пакеты, соответствующие введенным условиям.\r\n", "winget", "list", "--source", "empty", "--disable-interactivity", "--authentication-mode", "silent")
	f.Stdout("Name               Id                 Version     Available\n------------------------------------------------------------------\nExample CLI        Example.CLI        1.0         2.0\n", "winget", "list", "--source", "winget", "--disable-interactivity", "--authentication-mode", "silent")
	// The private source is unavailable, as msstore of an old WinGet whose
	// pinned certificates no longer match. Keep the other source, but do not
	// report its package or update counts as complete.
	f.Set(run.Result{ExitCode: 2316632158, Stdout: []byte("Failed when opening source(s); try the 'source reset' command if the problem persists.\nAn unexpected error occurred while executing the command:\n0x8a15005e : The server certificate did not match any of the expected values.\n")}, nil,
		"winget", "list", "--source", "private", "--disable-interactivity", "--authentication-mode", "silent")
	got := Network(t.Context(), &versions.Cache{Dir: t.TempDir()}, f, nil, inventory.PackageManager{ID: "winget", Path: "winget", Unlisted: true})
	if got.UpdatesKnown || !got.Unlisted || len(got.Packages) != 1 || got.Packages[0].Available != "2.0" {
		t.Fatalf("%+v", got)
	}
	for _, c := range f.Calls() {
		if slices.Contains(c.Args, "winget-font") {
			t.Fatal("listed from an explicit source")
		}
	}
	if want := "some package sources could not be queried: package query exited 0x8a15005e: 0x8a15005e : The server certificate did not match any of the expected values."; got.UpdateError != want {
		t.Fatalf("error %q, want %q", got.UpdateError, want)
	}
}
