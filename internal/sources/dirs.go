package sources

import (
	"strings"

	"github.com/Blinks44/hearth/internal/sysenv"
)

// Dirs are where version managers keep their data, after their environment
// variables and defaults. A field is empty when the home it depends on is
// unknown; the directory may not exist.
type Dirs struct {
	// Home is the user's home: HOME, or USERPROFILE on Windows.
	Home string
	// Fnm lists every directory fnm has used by default: any may hold
	// versions.
	Fnm                                           []string
	Nvm, Volta, Mise, Asdf, Nodenv, N, NvmWindows string
	// Pyenv is pyenv's directory, or pyenv-win's; Uv is where uv keeps the
	// Pythons it installs.
	Pyenv, Uv string
	// Rbenv is rbenv's root; Rvm is RVM's, with its rubies.
	Rbenv, Rvm string
	// Conda lists the directories conda's installers put it in by default.
	Conda []string
}

// ManagerDirs returns the managers' directories in env.
func ManagerDirs(env sysenv.Env) Dirs {
	d := Dirs{Home: env.Get("HOME")}
	if d.Home == "" {
		d.Home = env.Get("USERPROFILE")
	}
	or := d.or
	appData, localAppData := env.Get("APPDATA"), env.Get("LOCALAPPDATA")
	xdgData := or(env.Get("XDG_DATA_HOME"), "~/.local/share")
	for _, fnm := range []string{env.Get("FNM_DIR"), sub(appData, "fnm"), sub(xdgData, "fnm"), or("~/Library/Application Support/fnm"), or("~/.fnm")} {
		if fnm != "" {
			d.Fnm = append(d.Fnm, fnm)
		}
	}
	d.Nvm = or(env.Get("NVM_DIR"), "~/.nvm")
	d.Volta = or(env.Get("VOLTA_HOME"), sub(localAppData, "Volta"), "~/.volta")
	d.Mise = or(env.Get("MISE_DATA_DIR"), sub(localAppData, "mise"), sub(xdgData, "mise"))
	d.Asdf = or(env.Get("ASDF_DATA_DIR"), "~/.asdf")
	d.Nodenv = or(env.Get("NODENV_ROOT"), "~/.nodenv")
	d.N = or(env.Get("N_PREFIX"), "/usr/local")
	d.NvmWindows = or(env.Get("NVM_HOME"), sub(appData, "nvm"))
	d.Rbenv = or(env.Get("RBENV_ROOT"), "~/.rbenv")
	d.Rvm = or(env.Get("rvm_path"), "~/.rvm")
	d.Pyenv = or(env.Get("PYENV_ROOT"), env.Get("PYENV"), "~/.pyenv")
	d.Uv = or(env.Get("UV_PYTHON_INSTALL_DIR"), sub(appData, `uv\python`), sub(xdgData, "uv/python"))
	for _, name := range []string{"miniconda3", "anaconda3", "miniforge3", "mambaforge"} {
		if dir := or("~/" + name); dir != "" {
			d.Conda = append(d.Conda, dir)
		}
	}
	return d
}

// or returns the first non-empty dir; a "~/" one only when the home is
// known.
func (d Dirs) or(dirs ...string) string {
	for _, dir := range dirs {
		if rest, ok := strings.CutPrefix(dir, "~/"); ok {
			if d.Home == "" {
				continue
			}
			dir = join(d.Home, rest)
		}
		if dir != "" {
			return dir
		}
	}
	return ""
}

// sub is dir/rest, or "" when dir is.
func sub(dir, rest string) string {
	if dir == "" {
		return ""
	}
	return join(dir, rest)
}
