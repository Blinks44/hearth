package detect

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// ccCommand is gcc on Windows: MinGW's, as MSYS2's, is the compiler a
// terminal runs; MSVC's cl only runs in a Developer prompt.
const ccCommand = "gcc"

func cppExe(name string) string { return name + ".exe" }

// cppStub reports a stand-in compiler; Windows has none.
func cppStub(string) bool { return false }

// cppBase is the Visual Studio installation with MSVC's compiler, Build
// Tools or a full edition, as vswhere lists it.
func cppBase(ctx context.Context, r run.Runner, env sysenv.Env) *inventory.CppBase {
	vswhere := filepath.Join(env.Get("ProgramFiles(x86)"), "Microsoft Visual Studio", "Installer", "vswhere.exe")
	if !exists(vswhere) {
		return nil
	}
	res, err := r.Run(ctx, run.Cmd{Args: []string{vswhere, "-products", "*", "-requiresAny",
		"-requires", "Microsoft.VisualStudio.Component.VC.Tools.x86.x64", "Microsoft.VisualStudio.Component.VC.Tools.ARM64",
		"-format", "json", "-utf8"}, Env: env})
	if err != nil || res.ExitCode != 0 {
		return nil
	}
	return msvcBase(res.Stdout)
}

// msvcBase is the first installation in vswhere's JSON; nil for none.
func msvcBase(data []byte) *inventory.CppBase {
	var found []struct {
		DisplayName      string `json:"displayName"`
		InstallationPath string `json:"installationPath"`
		Catalog          struct {
			ProductDisplayVersion string `json:"productDisplayVersion"`
		} `json:"catalog"`
	}
	if json.Unmarshal(data, &found) != nil || len(found) == 0 {
		return nil
	}
	v := found[0]
	return &inventory.CppBase{ID: "msvc", Name: v.DisplayName, Version: v.Catalog.ProductDisplayVersion, Path: v.InstallationPath}
}
