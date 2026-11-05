package detect

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestNodeNvmWindows(t *testing.T) {
	root := root(t)
	nvm := filepath.Join(root, "nvm")
	v20 := fakeNode(t, filepath.Join(nvm, "v20.18.0"))
	v22 := fakeNode(t, filepath.Join(nvm, "v22.11.0"))
	// nvm use points NVM_SYMLINK at the version; the terminal has it on PATH.
	link := filepath.Join(root, "nodejs")
	if err := os.Symlink(filepath.Dir(v22), link); errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
		t.Skip("symlinks need an administrator or Developer Mode")
	} else if err != nil {
		t.Fatal(err)
	}
	term := sysenv.Env{
		"USERPROFILE=" + root,
		"APPDATA=" + filepath.Join(root, "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(root, "Local"),
		"NVM_HOME=" + nvm,
		"NVM_SYMLINK=" + link,
		"PATH=" + link,
	}

	got, err := Node(t.Context(), &run.Fake{}, term, sysenv.Env{})
	if err != nil {
		t.Fatal(err)
	}
	src := func(v string) inventory.Source {
		return inventory.Source{Kind: inventory.KindVersionManager, Manager: "nvm-windows", Root: filepath.Join(nvm, "v"+v), Version: v}
	}
	checkInstallations(t, got, []inventory.Installation{
		{Tool: "node", Version: "22.11.0", Path: filepath.Join(link, "node.exe"), RealPath: v22, Source: src("22.11.0"), OnPath: 1, Default: true},
		{Tool: "node", Version: "20.18.0", Path: v20, RealPath: v20, Source: src("20.18.0")},
	})
}

func startupFile(home string) string {
	return filepath.Join(home, "Documents", "PowerShell", "Microsoft.PowerShell_profile.ps1")
}
