package pkgmgr

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestChocoLocal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(pkg, file, content string) {
		t.Helper()
		dir := filepath.Join(root, "lib", pkg)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	nuspecXML := func(id, version string) string {
		return `<?xml version="1.0" encoding="utf-8"?><package xmlns="http://schemas.microsoft.com/packaging/2015/06/nuspec.xsd"><metadata><id>` + id + `</id><version>` + version + `</version></metadata></package>`
	}
	// The install script leaves only the archive for chocolatey itself.
	var nupkg strings.Builder
	zw := zip.NewWriter(&nupkg)
	w, err := zw.Create("chocolatey.nuspec")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(nuspecXML("chocolatey", "2.7.5"))); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	write("chocolatey", "chocolatey.nupkg", nupkg.String())
	write("jq", "jq.nuspec", nuspecXML("jq", "1.08.0.0"))
	write("pending", "pending.nuspec", nuspecXML("pending", "1.0"))
	write("pending", ".chocolateyPending", "")
	write("broken", "readme.txt", "")

	m := inventory.PackageManager{ID: "choco", Path: filepath.Join(root, "bin", "choco.exe")}
	chocoLocal(&m)
	if m.Version != "2.7.5" {
		t.Errorf("version %q", m.Version)
	}
	var got []string
	for _, p := range m.Packages {
		got = append(got, p.Name+" "+p.Version)
	}
	if len(got) != 2 || got[0] != "chocolatey 2.7.5" || got[1] != "jq 1.8.0" {
		t.Errorf("packages %q", got)
	}
	if len(m.Warnings) != 1 {
		t.Errorf("warnings %q, want the folder without a nuspec", m.Warnings)
	}
}

func TestNugetVersion(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"2.7.5":          "2.7.5",
		"1.08.0.0":       "1.8.0",
		"1.2":            "1.2.0",
		"1.2.3.4":        "1.2.3.4",
		"1.0.0-beta.01":  "1.0.0-beta.01",
		"1.0.0+build.5":  "1.0.0",
		"2026.11.05.120": "2026.11.5.120",
		"not-a-version":  "not-a-version",
	} {
		if got := nugetVersion(in); got != want {
			t.Errorf("nugetVersion(%q) = %q, want %q", in, got, want)
		}
	}
}
