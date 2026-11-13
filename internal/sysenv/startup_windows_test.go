package sysenv

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
)

func TestCulpritLines(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "Microsoft.PowerShell_profile.ps1")
	text := "Set-Alias ll Get-ChildItem\r\noh-my-posh init pwsh | Invoke-Expression\r\n# Import-Module posh-git\r\nImport-Module posh-git\r\n"
	if err := os.WriteFile(profile, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	want := []inventory.StartupLine{
		{File: profile, Line: 2, Text: "oh-my-posh init pwsh | Invoke-Expression"},
		{File: profile, Line: 4, Text: "Import-Module posh-git"},
	}
	if got := culpritLines([]string{profile}); !slices.Equal(got, want) {
		t.Errorf("culpritLines = %+v, want %+v", got, want)
	}
}
