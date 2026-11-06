package sources

import (
	"context"
	"testing"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

var (
	macEnv   = sysenv.Env{"HOME=/Users/ann"}
	linuxEnv = sysenv.Env{"HOME=/home/ann"}
	winEnv   = sysenv.Env{
		`USERPROFILE=C:\Users\Ann`,
		`APPDATA=C:\Users\Ann\AppData\Roaming`,
		`LOCALAPPDATA=C:\Users\Ann\AppData\Local`,
		`ProgramData=C:\ProgramData`,
		`ProgramFiles=C:\Program Files`,
		`SystemRoot=C:\WINDOWS`,
	}
)

func TestClassify(t *testing.T) {
	const (
		vm   = inventory.KindVersionManager
		pm   = inventory.KindPackageManager
		inst = inventory.KindInstaller
		sys  = inventory.KindSystem
	)
	tests := []struct {
		name           string
		env            sysenv.Env
		path, realPath string // realPath "" is path
		want           inventory.Source
	}{
		// macOS
		{"brew arm", macEnv, "/opt/homebrew/bin/node", "/opt/homebrew/Cellar/node/23.3.0/bin/node",
			inventory.Source{Kind: pm, Manager: "brew", Root: "/opt/homebrew/Cellar/node/23.3.0", Version: "23.3.0"}},
		{"brew intel", macEnv, "/usr/local/bin/node", "/usr/local/Cellar/node@22/22.11.0/bin/node",
			inventory.Source{Kind: pm, Manager: "brew", Root: "/usr/local/Cellar/node@22/22.11.0", Version: "22.11.0"}},
		{"brew cask", macEnv, "/opt/homebrew/bin/codex", "/opt/homebrew/Caskroom/codex/0.50.0/codex-aarch64-apple-darwin",
			inventory.Source{Kind: pm, Manager: "brew", Root: "/opt/homebrew/Caskroom/codex/0.50.0", Version: "0.50.0"}},
		{"fnm multishell", macEnv, "/Users/ann/.local/state/fnm_multishells/123_456/bin/node", "/Users/ann/.local/share/fnm/node-versions/v22.11.0/installation/bin/node",
			inventory.Source{Kind: vm, Manager: "fnm", Root: "/Users/ann/.local/share/fnm/node-versions/v22.11.0", Version: "22.11.0"}},
		{"fnm app support", macEnv, "/Users/ann/Library/Application Support/fnm/node-versions/v20.18.0/installation/bin/node", "",
			inventory.Source{Kind: vm, Manager: "fnm", Root: "/Users/ann/Library/Application Support/fnm/node-versions/v20.18.0", Version: "20.18.0"}},
		{"fnm FNM_DIR", sysenv.Env{"HOME=/Users/ann", "FNM_DIR=/opt/fnm"}, "/opt/fnm/node-versions/v18.20.4/installation/bin/node", "",
			inventory.Source{Kind: vm, Manager: "fnm", Root: "/opt/fnm/node-versions/v18.20.4", Version: "18.20.4"}},
		{"nvm", macEnv, "/Users/ann/.nvm/versions/node/v22.11.0/bin/node", "",
			inventory.Source{Kind: vm, Manager: "nvm", Root: "/Users/ann/.nvm/versions/node/v22.11.0", Version: "22.11.0"}},
		{"nvm NVM_DIR", sysenv.Env{"HOME=/Users/ann", "NVM_DIR=/Users/ann/.config/nvm/"}, "/Users/ann/.config/nvm/versions/node/v22.11.0/bin/node", "",
			inventory.Source{Kind: vm, Manager: "nvm", Root: "/Users/ann/.config/nvm/versions/node/v22.11.0", Version: "22.11.0"}},
		{"volta shim", macEnv, "/Users/ann/.volta/bin/node", "",
			inventory.Source{Kind: vm, Manager: "volta", Root: "/Users/ann/.volta", Shim: true}},
		{"volta image", macEnv, "/Users/ann/.volta/tools/image/node/22.11.0/bin/node", "",
			inventory.Source{Kind: vm, Manager: "volta", Root: "/Users/ann/.volta/tools/image/node/22.11.0", Version: "22.11.0"}},
		{"mise shim links to mise", macEnv, "/Users/ann/.local/share/mise/shims/node", "/opt/homebrew/Cellar/mise/2026.10.1/bin/mise",
			inventory.Source{Kind: vm, Manager: "mise", Root: "/Users/ann/.local/share/mise", Shim: true}},
		{"mise install", macEnv, "/Users/ann/.local/share/mise/installs/node/22.11.0/bin/node", "",
			inventory.Source{Kind: vm, Manager: "mise", Root: "/Users/ann/.local/share/mise/installs/node/22.11.0", Version: "22.11.0"}},
		{"asdf shim", macEnv, "/Users/ann/.asdf/shims/node", "",
			inventory.Source{Kind: vm, Manager: "asdf", Root: "/Users/ann/.asdf", Shim: true}},
		{"asdf install", macEnv, "/Users/ann/.asdf/installs/nodejs/22.11.0/bin/node", "",
			inventory.Source{Kind: vm, Manager: "asdf", Root: "/Users/ann/.asdf/installs/nodejs/22.11.0", Version: "22.11.0"}},
		{"n", macEnv, "/usr/local/bin/node", "/usr/local/n/versions/node/22.11.0/bin/node",
			inventory.Source{Kind: vm, Manager: "n", Root: "/usr/local/n/versions/node/22.11.0", Version: "22.11.0"}},
		{"nodenv shim", macEnv, "/Users/ann/.nodenv/shims/node", "",
			inventory.Source{Kind: vm, Manager: "nodenv", Root: "/Users/ann/.nodenv", Shim: true}},
		{"nodenv version", macEnv, "/Users/ann/.nodenv/versions/22.11.0/bin/node", "",
			inventory.Source{Kind: vm, Manager: "nodenv", Root: "/Users/ann/.nodenv/versions/22.11.0", Version: "22.11.0"}},
		{"pyenv shim", macEnv, "/Users/ann/.pyenv/shims/python3", "",
			inventory.Source{Kind: vm, Manager: "pyenv", Root: "/Users/ann/.pyenv", Shim: true}},
		{"pyenv version", macEnv, "/Users/ann/.pyenv/versions/3.12.8/bin/python3", "",
			inventory.Source{Kind: vm, Manager: "pyenv", Root: "/Users/ann/.pyenv/versions/3.12.8", Version: "3.12.8"}},
		{"uv python", macEnv, "/Users/ann/.local/bin/python3.12", "/Users/ann/.local/share/uv/python/cpython-3.12.8-macos-aarch64-none/bin/python3.12",
			inventory.Source{Kind: vm, Manager: "uv", Root: "/Users/ann/.local/share/uv/python/cpython-3.12.8-macos-aarch64-none", Version: "3.12.8"}},
		{"miniconda", macEnv, "/Users/ann/miniconda3/bin/python3", "",
			inventory.Source{Kind: pm, Manager: "conda", Root: "/Users/ann/miniconda3"}},
		{"official pkg", macEnv, "/usr/local/bin/node", "",
			inventory.Source{Kind: inst, Root: "/usr/local"}},
		{"go installer", macEnv, "/usr/local/go/bin/go", "",
			inventory.Source{Kind: inst, Root: "/usr/local"}},
		{"python.org", macEnv, "/usr/local/bin/python3", "/Library/Frameworks/Python.framework/Versions/3.13/bin/python3.13",
			inventory.Source{Kind: inst, Root: "/Library/Frameworks"}},
		{"apple", macEnv, "/usr/bin/python3", "",
			inventory.Source{Kind: sys, Root: "/usr"}},
		{"unknown", macEnv, "/Users/ann/bin/node", "",
			inventory.Source{}},
		{"version dir needs a file under it", macEnv, "/Users/ann/.nvm/versions/node/v22.11.0", "",
			inventory.Source{}},

		// Linux
		{"apt", linuxEnv, "/usr/bin/node", "",
			inventory.Source{Kind: sys, Root: "/usr"}},
		{"bin symlinked to usr", linuxEnv, "/bin/node", "/usr/bin/node",
			inventory.Source{Kind: sys, Root: "/usr"}},
		{"linuxbrew", linuxEnv, "/home/linuxbrew/.linuxbrew/bin/node", "/home/linuxbrew/.linuxbrew/Cellar/node/23.3.0/bin/node",
			inventory.Source{Kind: pm, Manager: "brew", Root: "/home/linuxbrew/.linuxbrew/Cellar/node/23.3.0", Version: "23.3.0"}},
		{"fnm xdg", linuxEnv, "/home/ann/.local/share/fnm/node-versions/v22.11.0/installation/bin/node", "",
			inventory.Source{Kind: vm, Manager: "fnm", Root: "/home/ann/.local/share/fnm/node-versions/v22.11.0", Version: "22.11.0"}},
		{"mise XDG_DATA_HOME", sysenv.Env{"HOME=/home/ann", "XDG_DATA_HOME=/home/ann/data"}, "/home/ann/data/mise/installs/node/22.11.0/bin/node", "",
			inventory.Source{Kind: vm, Manager: "mise", Root: "/home/ann/data/mise/installs/node/22.11.0", Version: "22.11.0"}},
		{"n N_PREFIX", sysenv.Env{"HOME=/home/ann", "N_PREFIX=/home/ann/.n"}, "/home/ann/.n/bin/node", "/home/ann/.n/n/versions/node/22.11.0/bin/node",
			inventory.Source{Kind: vm, Manager: "n", Root: "/home/ann/.n/n/versions/node/22.11.0", Version: "22.11.0"}},
		{"opt", linuxEnv, "/opt/nodejs/bin/node", "",
			inventory.Source{Kind: inst, Root: "/opt"}},

		// Windows
		{"npm global windows", winEnv, `C:\Users\Ann\AppData\Roaming\npm\claude.cmd`, "",
			inventory.Source{Kind: pm, Manager: "npm", Root: `C:\Users\Ann\AppData\Roaming\npm`}},
		{"uv windows", winEnv, `C:\Users\Ann\AppData\Roaming\uv\python\cpython-3.12.8-windows-x86_64-none\python.exe`, "",
			inventory.Source{Kind: vm, Manager: "uv", Root: `C:\Users\Ann\AppData\Roaming\uv\python\cpython-3.12.8-windows-x86_64-none`, Version: "3.12.8"}},
		{"python install manager", winEnv, `C:\Users\Ann\AppData\Local\Python\pythoncore-3.14-arm64\python.exe`, "",
			inventory.Source{Kind: vm, Manager: "pymanager", Root: `C:\Users\Ann\AppData\Local\Python\pythoncore-3.14-arm64`, Version: "pythoncore-3.14-arm64"}},
		{"python install manager alias", winEnv, `C:\Users\Ann\AppData\Local\Python\bin\python3.14.exe`, "",
			inventory.Source{Kind: vm, Manager: "pymanager", Root: `C:\Users\Ann\AppData\Local\Python`, Shim: true}},
		{"python full installer", winEnv, `C:\Users\Ann\AppData\Local\Programs\Python\Python315\python.exe`, "",
			inventory.Source{Kind: inst, Root: `C:\Users\Ann\AppData\Local\Programs`}},
		{"nvm-windows", winEnv, `C:\nvm4w\nodejs\node.exe`, `C:\Users\Ann\AppData\Roaming\nvm\v22.11.0\node.exe`,
			inventory.Source{Kind: vm, Manager: "nvm-windows", Root: `C:\Users\Ann\AppData\Roaming\nvm\v22.11.0`, Version: "22.11.0"}},
		{"nvm-windows NVM_HOME", append(winEnv, `NVM_HOME=D:\nvm`), `C:\Program Files\nodejs\node.exe`, `D:\nvm\v20.18.0\node.exe`,
			inventory.Source{Kind: vm, Manager: "nvm-windows", Root: `D:\nvm\v20.18.0`, Version: "20.18.0"}},
		{"fnm windows", winEnv, `C:\Users\Ann\AppData\Local\fnm_multishells\1_2\node.exe`, `C:\Users\Ann\AppData\Roaming\fnm\node-versions\v22.11.0\installation\node.exe`,
			inventory.Source{Kind: vm, Manager: "fnm", Root: `C:\Users\Ann\AppData\Roaming\fnm\node-versions\v22.11.0`, Version: "22.11.0"}},
		{"volta windows shim", winEnv, `C:\Users\Ann\AppData\Local\Volta\bin\node.exe`, "",
			inventory.Source{Kind: vm, Manager: "volta", Root: `C:\Users\Ann\AppData\Local\Volta`, Shim: true}},
		{"mise windows", winEnv, `C:\Users\Ann\AppData\Local\mise\installs\node\22.11.0\node.exe`, "",
			inventory.Source{Kind: vm, Manager: "mise", Root: `C:\Users\Ann\AppData\Local\mise\installs\node\22.11.0`, Version: "22.11.0"}},
		{"scoop shim", winEnv, `C:\Users\Ann\scoop\shims\node.exe`, "",
			inventory.Source{Kind: pm, Manager: "scoop", Root: `C:\Users\Ann\scoop`, Shim: true}},
		{"scoop app", winEnv, `C:\Users\Ann\scoop\apps\nodejs\current\bin\node.exe`, `C:\Users\Ann\scoop\apps\nodejs\23.3.0\bin\node.exe`,
			inventory.Source{Kind: pm, Manager: "scoop", Root: `C:\Users\Ann\scoop\apps\nodejs\23.3.0`, Version: "23.3.0"}},
		{"scoop global", winEnv, `C:\ProgramData\scoop\apps\git\2.47.0\cmd\git.exe`, "",
			inventory.Source{Kind: pm, Manager: "scoop", Root: `C:\ProgramData\scoop\apps\git\2.47.0`, Version: "2.47.0"}},
		{"winget portable", winEnv, `C:\Users\Ann\AppData\Local\Microsoft\WinGet\Links\rg.exe`, `C:\Users\Ann\AppData\Local\Microsoft\WinGet\Packages\BurntSushi.ripgrep.MSVC_Microsoft.Winget.Source_8wekyb3d8bbwe\ripgrep-14.1.1\rg.exe`,
			inventory.Source{Kind: pm, Manager: "winget", Root: `C:\Users\Ann\AppData\Local\Microsoft\WinGet\Packages\BurntSushi.ripgrep.MSVC_Microsoft.Winget.Source_8wekyb3d8bbwe`}},
		{"choco shim", winEnv, `C:\ProgramData\chocolatey\bin\node.exe`, "",
			inventory.Source{Kind: pm, Manager: "choco", Root: `C:\ProgramData\chocolatey`, Shim: true}},
		{"choco lib", winEnv, `C:\ProgramData\chocolatey\lib\nodejs\tools\node.exe`, "",
			inventory.Source{Kind: pm, Manager: "choco", Root: `C:\ProgramData\chocolatey\lib\nodejs`}},
		{"msi, case ignored", winEnv, `c:\program files\nodejs\node.exe`, "",
			inventory.Source{Kind: inst, Root: `c:\program files`}},
		{"user installer", winEnv, `C:\Users\Ann\AppData\Local\Programs\Python\Python313\python.exe`, "",
			inventory.Source{Kind: inst, Root: `C:\Users\Ann\AppData\Local\Programs`}},
		{"windows", winEnv, `C:\Windows\System32\curl.exe`, "",
			inventory.Source{Kind: sys, Root: `C:\Windows`}},
		{"windows path, unix env", macEnv, `C:\Users\Ann\scoop\shims\node.exe`, "",
			inventory.Source{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			real := tt.realPath
			if real == "" {
				real = tt.path
			}
			if got := Classify(tt.path, real, tt.env); got != tt.want {
				t.Errorf("Classify(%q, %q)\n got %+v\nwant %+v", tt.path, real, got, tt.want)
			}
		})
	}
}

func TestOwner(t *testing.T) {
	const path = "/usr/bin/node"
	dpkg := []string{"dpkg-query", "-S", path}
	rpm := []string{"rpm", "-qf", "--queryformat", "%{NAME}\n", path}
	pacman := []string{"pacman", "-Qqo", path}
	tests := []struct {
		name        string
		setup       func(f *run.Fake)
		wantManager string
		wantPkg     string
		wantErr     bool
	}{
		{"dpkg", func(f *run.Fake) { f.Stdout("nodejs: /usr/bin/node\n", dpkg...) }, "apt", "nodejs", false},
		{"dpkg arch and diversion", func(f *run.Fake) {
			f.Stdout("diversion by foo from: /usr/bin/node\nnodejs:amd64, libnode: /usr/bin/node\n", dpkg...)
		}, "apt", "nodejs", false},
		{"rpm", func(f *run.Fake) { f.Stdout("nodejs\n", rpm...) }, "rpm", "nodejs", false},
		{"pacman", func(f *run.Fake) { f.Stdout("nodejs\n", pacman...) }, "pacman", "nodejs", false},
		{"not owned", func(f *run.Fake) {
			f.Set(run.Result{ExitCode: 1, Stderr: []byte("dpkg-query: no path found")}, nil, dpkg...)
			f.Stdout("nodejs\n", pacman...) // only the first manager found answers
		}, "", "", false},
		{"no manager", func(*run.Fake) {}, "", "", false},
		{"timeout", func(f *run.Fake) { f.Set(run.Result{ExitCode: -1}, context.DeadlineExceeded, dpkg...) }, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &run.Fake{}
			tt.setup(f)
			manager, pkg, err := Owner(t.Context(), f, linuxEnv, path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, want error %v", err, tt.wantErr)
			}
			if manager != tt.wantManager || pkg != tt.wantPkg {
				t.Errorf("Owner = %q, %q, want %q, %q", manager, pkg, tt.wantManager, tt.wantPkg)
			}
		})
	}
}
