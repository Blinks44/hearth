package pkgmgr

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
)

func TestSearchValidation(t *testing.T) {
	m := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
	for _, bad := range []string{"", "   ", "-rf", "--version", "jq;rm -rf ~", "foo\nbar"} {
		f := &run.Fake{}
		_, err := Search(t.Context(), f, nil, m, bad, "")
		if !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("Search(%q) err = %v, want ErrInvalidQuery", bad, err)
		}
		if len(f.Calls()) != 0 {
			t.Errorf("Search(%q) ran commands: %+v", bad, f.Calls())
		}
	}
}

func TestSearchBrew(t *testing.T) {
	m := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
	f := &run.Fake{}
	f.Stdout("grafana-alloy\ngrafana\nmcp-grafana\n", "/opt/homebrew/bin/brew", "search", "--formula", "grafana")
	f.Set(run.Result{ExitCode: 1, Stderr: []byte(`Error: No formulae or casks found for "grafana".`)}, nil,
		"/opt/homebrew/bin/brew", "search", "--cask", "grafana")
	f.Stdout("==> Formulae\ngcx ✔: CLI for managing Grafana Cloud resources\ngrafanactl (deprecated): CLI to interact with Grafana\nmcp-grafana: MCP server for Grafana\n\n==> Casks\n",
		"/opt/homebrew/bin/brew", "search", "--desc", "grafana")
	f.Stdout("grafana: Gorgeous metric visualizations and dashboards\ngrafana-alloy: OpenTelemetry Collector distribution\n",
		"/opt/homebrew/bin/brew", "desc", "--formula", "grafana", "grafana-alloy")

	got, err := Search(t.Context(), f, nil, m, "grafana", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []SearchResult{
		{Name: "grafana", Kind: "formula", Description: "Gorgeous metric visualizations and dashboards"},
		{Name: "grafana-alloy", Kind: "formula", Description: "OpenTelemetry Collector distribution"},
		{Name: "mcp-grafana", Kind: "formula", Description: "MCP server for Grafana"},
		{Name: "gcx", Kind: "formula", Description: "CLI for managing Grafana Cloud resources"},
		{Name: "grafanactl", Kind: "formula", Description: "CLI to interact with Grafana"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestSearchBrewKindFilterAndDescOnly(t *testing.T) {
	m := inventory.PackageManager{ID: "brew", Name: "Homebrew", Path: "/opt/homebrew/bin/brew"}
	f := &run.Fake{}
	f.Stdout("orbstack\n", "/opt/homebrew/bin/brew", "search", "--cask", "orb")
	f.Stdout("==> Casks\norbstack: Replacement for Docker Desktop\n", "/opt/homebrew/bin/brew", "search", "--cask", "--desc", "orb")

	got, err := Search(t.Context(), f, nil, m, "orb", "cask")
	if err != nil {
		t.Fatal(err)
	}
	want := []SearchResult{{Name: "orbstack", Kind: "cask", Description: "Replacement for Docker Desktop"}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	// When name search finds nothing, --desc results are still returned.
	f2 := &run.Fake{}
	f2.Set(run.Result{ExitCode: 1, Stderr: []byte(`Error: No formulae or casks found for "silver".`)}, nil,
		"/opt/homebrew/bin/brew", "search", "--formula", "silver")
	f2.Set(run.Result{ExitCode: 1, Stderr: []byte(`Error: No formulae or casks found for "silver".`)}, nil,
		"/opt/homebrew/bin/brew", "search", "--cask", "silver")
	f2.Stdout("==> Formulae\nripgrep: Search tool like grep and The Silver Searcher\n\n==> Casks\n",
		"/opt/homebrew/bin/brew", "search", "--desc", "silver")

	got, err = Search(t.Context(), f2, nil, m, "silver", "")
	if err != nil {
		t.Fatal(err)
	}
	wantDesc := []SearchResult{{Name: "ripgrep", Kind: "formula", Description: "Search tool like grep and The Silver Searcher"}}
	if !slices.Equal(got, wantDesc) {
		t.Fatalf("desc-only got %+v\nwant %+v", got, wantDesc)
	}
}

func TestSearchAPT(t *testing.T) {
	m := inventory.PackageManager{ID: "apt", Name: "APT", Path: "/usr/bin/apt"}
	f := &run.Fake{}
	f.Stdout("jq - lightweight and flexible command-line JSON processor\nlibjq1 - lightweight JSON library\n", "/usr/bin/apt-cache", "search", "--", "jq")

	got, err := Search(t.Context(), f, nil, m, "jq", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []SearchResult{
		{Name: "jq", Kind: "package", Description: "lightweight and flexible command-line JSON processor"},
		{Name: "libjq1", Kind: "package", Description: "lightweight JSON library"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	// Exact match must not be dropped even when >30 noisy packages precede it.
	var noisy strings.Builder
	for i := range 35 {
		noisy.WriteString("libfoo-jq-" + strconv.Itoa(i) + " - helper mentioning jq\n")
	}
	noisy.WriteString("jq - lightweight and flexible command-line JSON processor\n")
	f2 := &run.Fake{}
	f2.Stdout(noisy.String(), "/usr/bin/apt-cache", "search", "--", "jq")
	got, err = Search(t.Context(), f2, nil, m, "jq", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != maxSearchResults || got[0].Name != "jq" {
		t.Fatalf("exact match not prioritized: len=%d first=%+v", len(got), got[0])
	}
}

func TestSearchFlatpakANSIEmpty(t *testing.T) {
	m := inventory.PackageManager{ID: "flatpak", Name: "Flatpak", Path: "/usr/bin/flatpak"}
	f := &run.Fake{}
	f.Stdout("\x1b[?25h\n", "/usr/bin/flatpak", "search", "--columns=application,version,description,remotes", "--", "gimp")

	got, err := Search(t.Context(), f, nil, m, "gimp", "")
	if err != nil || len(got) != 0 {
		t.Fatalf("flatpak ANSI empty = (%+v, %v), want (nil, nil)", got, err)
	}
}

func TestSearchPacmanAndEmpty(t *testing.T) {
	m := inventory.PackageManager{ID: "pacman", Name: "Pacman", Path: "/usr/bin/pacman"}
	f := &run.Fake{}
	f.Stdout("extra/ripgrep 14.1.1-1 [installed]\n    A search tool that combines the usability of ag with the raw speed of grep\n", "/usr/bin/pacman", "-Ss", "--", "ripgrep")

	got, err := Search(t.Context(), f, nil, m, "ripgrep", "")
	if err != nil {
		t.Fatal(err)
	}
	want := []SearchResult{{
		Name: "ripgrep", Version: "14.1.1-1", Kind: "package", Origin: "extra",
		Description: "A search tool that combines the usability of ag with the raw speed of grep",
	}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}

	// Empty pacman search exits 1 with empty stderr -> no error, nil slice.
	f2 := &run.Fake{}
	f2.Set(run.Result{ExitCode: 1}, nil, "/usr/bin/pacman", "-Ss", "--", "nonexistent")
	got, err = Search(t.Context(), f2, nil, m, "nonexistent", "")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty pacman search = (%+v, %v), want (nil, nil)", got, err)
	}
}

func TestSearchWinGetAndScoop(t *testing.T) {
	wingetOut := `Name       Id             Version Match
----------------------------------------
jq         jqlang.jq      1.7.1   Tag: json
ripgrep    BurntSushi.rg  14.1.1
`
	wg := inventory.PackageManager{ID: "winget", Name: "WinGet"}
	f := &run.Fake{}
	f.Stdout(wingetOut, "winget", "search", "--query", "jq", "--source", "winget", "--disable-interactivity", "--count", "30")
	got, err := Search(t.Context(), f, nil, wg, "jq", "")
	if err != nil {
		t.Fatal(err)
	}
	wantWG := []SearchResult{
		{Name: "jqlang.jq", Version: "1.7.1", Kind: "package", Description: "jq", Origin: "winget"},
		{Name: "BurntSushi.rg", Version: "14.1.1", Kind: "package", Description: "ripgrep", Origin: "winget"},
	}
	if !slices.Equal(got, wantWG) {
		t.Fatalf("winget got %+v\nwant %+v", got, wantWG)
	}

	scoopOut := `Results from local buckets...

Name    Version Source
----    ------- ------
jq      1.7.1   main
`
	sc := inventory.PackageManager{ID: "scoop", Name: "Scoop", Path: `C:\scoop\shims\scoop.ps1`}
	f2 := &run.Fake{}
	f2.Stdout(scoopOut, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", sc.Path, "search", "jq")
	got, err = Search(t.Context(), f2, nil, sc, "jq", "")
	if err != nil {
		t.Fatal(err)
	}
	wantSc := []SearchResult{{Name: "jq", Version: "1.7.1", Kind: "app", Origin: "main"}}
	if !slices.Equal(got, wantSc) {
		t.Fatalf("scoop got %+v\nwant %+v", got, wantSc)
	}

	// Localized (Russian) WinGet output with carriage-return spinner, ellipsis
	// in Name, a truncated Id row, and a <...> truncation footer that must be skipped.
	ruOut := "   - \r   \\ \rИмя                 ИД              Версия   Совпадение\r\n" +
		"-------------------------------------------------------------\r\n" +
		"Очень длинное имя…  jqlang.jq       1.7.1    Tag: json\r\n" +
		"Broken              Truncated.Id…   1.0.0\r\n" +
		"<дополнительные записи усечены из-за ограничения количества результатов>\r\n"
	f3 := &run.Fake{}
	f3.Stdout(ruOut, "winget", "search", "--query", "jq", "--source", "winget", "--disable-interactivity", "--count", "30")
	got, err = Search(t.Context(), f3, nil, wg, "jq", "")
	if err != nil {
		t.Fatal(err)
	}
	wantRU := []SearchResult{
		{Name: "jqlang.jq", Version: "1.7.1", Kind: "package", Description: "Очень длинное имя…", Origin: "winget"},
	}
	if !slices.Equal(got, wantRU) {
		t.Fatalf("localized winget got %+v\nwant %+v", got, wantRU)
	}
}
