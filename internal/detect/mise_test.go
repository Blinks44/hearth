package detect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func TestMiseManager(t *testing.T) {
	root := root(t)
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	mise := filepath.Join(bin, "mise")
	writeFile(t, mise, "")
	if err := os.Chmod(mise, 0o755); err != nil {
		t.Fatal(err)
	}
	shims := filepath.Join(home, ".local", "share", "mise", "shims")
	env := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "LOCALAPPDATA=" + filepath.Join(home, ".local", "share"),
		"PATH=" + shims + string(os.PathListSeparator) + bin}
	d := sources.ManagerDirs(env)
	d.Mise = filepath.Join(home, ".local", "share", "mise")

	for _, tt := range []struct {
		name, rc  string
		activated bool
	}{
		{"activated", `eval "$(mise activate zsh)"` + "\n", true},
		{"commented out", `# eval "$(mise activate zsh)"` + "\n", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writeFile(t, startupFile(home), tt.rc)
			r := &run.Fake{}
			r.Stdout("2025.1.6 macos-arm64 (2025-01-20)\n", mise, "--version")
			m := miseManager(t.Context(), r, env, sysenv.Env{"PATH=" + bin}, d)
			if m.Name != "mise" || m.Version != "2025.1.6" || m.Activated != tt.activated ||
				!m.Shims || m.ShimsInGUI || !m.SwitchOnCD || len(m.Files) != 2 {
				t.Errorf("got %+v", m)
			}
		})
	}
}

func TestMiseVersionFromPath(t *testing.T) {
	r := &run.Fake{}
	if got := miseVersion(t.Context(), r, nil, "/opt/homebrew/Cellar/mise/2025.10.2/bin/mise"); got != "2025.10.2" {
		t.Errorf("version %q", got)
	}
	if len(r.Calls()) != 0 {
		t.Error("ran mise for a version its path names")
	}
}

// Installed versions alone do not mean the terminal switches versions.
func TestMiseConnections(t *testing.T) {
	for _, tt := range []struct {
		name                                     string
		terminal, gui, activate, shell, switches bool
	}{
		{name: "not connected"},
		{name: "terminal shims", terminal: true, switches: true},
		{name: "GUI shims", gui: true},
		{name: "both shims", terminal: true, gui: true, switches: true},
		{name: "activate", activate: true, switches: true},
		{name: "activate and GUI shims", gui: true, activate: true, switches: true},
		{name: "activate from sourced file", shell: true, switches: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := root(t)
			d := sources.Dirs{Home: home, Mise: filepath.Join(home, "mise")}
			shims := filepath.Join(d.Mise, "shims")
			term := sysenv.Env{"HOME=" + home, "USERPROFILE=" + home, "PATH="}
			gui := sysenv.Env{"PATH="}
			if tt.terminal {
				term = append(term, "PATH="+shims)
			}
			if tt.gui {
				gui = append(gui, "PATH="+shims)
			}
			if tt.activate {
				writeFile(t, startupFile(home), "mise activate\n")
			}
			if tt.shell {
				term = append(term, "MISE_SHELL=zsh")
			}
			m := miseManager(t.Context(), &run.Fake{}, term, gui, d)
			if m.SwitchOnCD != tt.switches || m.Shims != tt.terminal || m.ShimsInGUI != tt.gui || m.Activated != (tt.activate || tt.shell) {
				t.Fatalf("unexpected connection: %+v", m)
			}
		})
	}
}

func TestMiseGlobalConfig(t *testing.T) {
	for _, tt := range []struct{ name, global, dir, xdg string }{
		{name: "default"},
		{name: "XDG", xdg: "xdg"},
		{name: "mise directory overrides XDG", dir: "config", xdg: "xdg"},
		{name: "explicit file overrides directories", global: "custom.toml", dir: "config", xdg: "xdg"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := root(t)
			env := sysenv.Env{"PATH="}
			want := filepath.Join(home, ".config", "mise", "config.toml")
			if tt.xdg != "" {
				env = append(env, "XDG_CONFIG_HOME="+filepath.Join(home, tt.xdg))
				want = filepath.Join(home, tt.xdg, "mise", "config.toml")
			}
			if tt.dir != "" {
				env = append(env, "MISE_CONFIG_DIR="+filepath.Join(home, tt.dir))
				want = filepath.Join(home, tt.dir, "config.toml")
			}
			if tt.global != "" {
				env = append(env, "MISE_GLOBAL_CONFIG_FILE="+filepath.Join(home, tt.global))
				want = filepath.Join(home, tt.global)
			}
			d := sources.Dirs{Home: home}
			if m := miseManager(t.Context(), &run.Fake{}, env, nil, d); m.ConfigPath != "" {
				t.Fatalf("missing file shown: %+v", m)
			}
			writeFile(t, want, "[tools]\nnode = \"22\"\n")
			m := miseManager(t.Context(), &run.Fake{}, env, nil, d)
			if m.ConfigPath != want {
				t.Fatalf("config %q, want %q", m.ConfigPath, want)
			}
			if got := miseDefault(d, env, "node", []string{"22.12.0"}); got != "22.12.0" {
				t.Fatalf("default %q", got)
			}
		})
	}
}
