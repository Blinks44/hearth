// Package sources tells where an installation came from by its path: which
// version manager, package manager or installer put it there. Rules follow
// docs/research/detection.md, section 3.
package sources

import (
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Classify tells where the executable at path came from. realPath is path
// with symlinks resolved. A manager's shim is told by path, since it may
// resolve to the manager itself (mise's shims link to mise); anything else
// by realPath. env gives the home and the managers' directories.
//
// Classify reads no files, so it works on paths of any OS: one with a
// drive letter is taken as a Windows path, compared ignoring case.
func Classify(path, realPath string, env sysenv.Env) inventory.Source {
	rules := rulesFor(env)
	for _, r := range rules {
		if r.shim {
			if s, ok := r.match(path); ok {
				return s
			}
		}
	}
	for _, r := range rules {
		if !r.shim {
			if s, ok := r.match(realPath); ok {
				return s
			}
		}
	}
	return inventory.Source{}
}

// rule recognizes paths under dir laid out as pattern: segments that must
// match, where "*" is any segment and "@" is the version. The installation's
// root is dir plus the matched segments. A shim rule matches files right in
// dir/pattern.
type rule struct {
	dir     string
	pattern string
	kind    inventory.Kind
	manager string
	shim    bool
}

func rulesFor(env sysenv.Env) []rule {
	d := ManagerDirs(env)
	or := d.or
	const (
		vm   = inventory.KindVersionManager
		pm   = inventory.KindPackageManager
		inst = inventory.KindInstaller
		sys  = inventory.KindSystem
	)
	localAppData := env.Get("LOCALAPPDATA")
	programData := or(env.Get("ProgramData"), `C:\ProgramData`)

	var rules []rule
	add := func(dir, pattern string, kind inventory.Kind, manager string) {
		if dir != "" {
			rules = append(rules, rule{dir: dir, pattern: pattern, kind: kind, manager: manager})
		}
	}
	shim := func(dir, pattern, manager string) {
		if dir != "" {
			rules = append(rules, rule{dir: dir, pattern: pattern, kind: vm, manager: manager, shim: true})
		}
	}

	// Shims, by the path found.
	shim(d.Volta, "bin", "volta")
	shim(d.Mise, "shims", "mise")
	shim(d.Asdf, "shims", "asdf")
	shim(d.Nodenv, "shims", "nodenv")
	shim(d.Rbenv, "shims", "rbenv")
	shim(d.Pyenv, "shims", "pyenv")
	shim(sub(localAppData, "Python"), "bin", "pymanager")
	scoop := or(env.Get("SCOOP"), "~/scoop")
	scoopGlobal := or(env.Get("SCOOP_GLOBAL"), sub(programData, "scoop"))
	for _, d := range []string{scoop, scoopGlobal} {
		rules = append(rules, rule{dir: d, pattern: "shims", kind: pm, manager: "scoop", shim: true})
	}
	choco := or(env.Get("ChocolateyInstall"), sub(programData, "chocolatey"))
	rules = append(rules, rule{dir: choco, pattern: "bin", kind: pm, manager: "choco", shim: true})
	// WinGet's Links and fnm's multishells are symlinks: the real path
	// tells the installation.

	// Installations, by the real path. More specific rules go first.
	for _, p := range []string{env.Get("HOMEBREW_PREFIX"), "/opt/homebrew", "/usr/local", "/home/linuxbrew/.linuxbrew", "~/.linuxbrew"} {
		if p = or(p); p != "" {
			add(p, "Cellar/*/@", pm, "brew")
			add(p, "Caskroom/*/@", pm, "brew")
		}
	}
	for _, fnm := range d.Fnm {
		add(fnm, "node-versions/@", vm, "fnm")
	}
	add(d.Nvm, "versions/node/@", vm, "nvm")
	add(d.Volta, "tools/image/*/@", vm, "volta")
	add(d.Mise, "installs/*/@", vm, "mise")
	// mise's .NET keeps every SDK in one shared root, as dotnet does.
	add(d.Mise, "dotnet-root", vm, "mise")
	add(d.Asdf, "installs/*/@", vm, "asdf")
	add(d.Nodenv, "versions/@", vm, "nodenv")
	add(d.Rbenv, "versions/@", vm, "rbenv")
	add(d.Rvm, "rubies/@", vm, "rvm")
	add(or("~/.rubies"), "@", vm, "chruby")
	add("/opt/rubies", "@", vm, "chruby")
	add(d.Pyenv, "versions/@", vm, "pyenv")
	add(d.Uv, "@", vm, "uv")
	add(sub(localAppData, "Python"), "@", vm, "pymanager")
	for _, conda := range d.Conda {
		add(conda, "", pm, "conda")
	}
	add(d.N, "n/versions/*/@", vm, "n")
	add(d.NvmWindows, "@", vm, "nvm-windows")
	add(sub(env.Get("APPDATA"), "npm"), "", pm, "npm")
	add(scoop, "apps/*/@", pm, "scoop")
	add(scoopGlobal, "apps/*/@", pm, "scoop")
	add(choco, "lib/*", pm, "choco")
	add(sub(localAppData, `Microsoft\WinGet`), "Packages/*", pm, "winget")
	add(or(env.Get("ProgramFiles"), `C:\Program Files`), `WinGet/Packages/*`, pm, "winget")

	add("/usr/local", "", inst, "")
	add("/opt", "", inst, "")
	add("/Library/Frameworks", "", inst, "")
	add(or(env.Get("ProgramFiles"), `C:\Program Files`), "", inst, "")
	add(or(env.Get("ProgramFiles(x86)"), `C:\Program Files (x86)`), "", inst, "")
	add(sub(localAppData, "Programs"), "", inst, "")

	for _, d := range []string{"/usr", "/bin", "/sbin", "/System", or(env.Get("SystemRoot"), `C:\Windows`)} {
		add(d, "", sys, "")
	}
	return rules
}

// match reports whether p is under the rule's directory in its layout and
// what source it has then.
func (r rule) match(p string) (inventory.Source, bool) {
	segs, win := split(p)
	dir, dirWin := split(r.dir)
	if win != dirWin || len(segs) <= len(dir) {
		return inventory.Source{}, false
	}
	eq := func(a, b string) bool { return a == b || win && strings.EqualFold(a, b) }
	for i, d := range dir {
		if !eq(segs[i], d) {
			return inventory.Source{}, false
		}
	}
	rest := segs[len(dir):]
	var pattern []string
	if r.pattern != "" {
		pattern = strings.Split(r.pattern, "/")
	}
	// Something must be left past the pattern: the file itself, or more of
	// the installation. A shim is the file right after it.
	if len(rest) <= len(pattern) || r.shim && len(rest) != len(pattern)+1 {
		return inventory.Source{}, false
	}
	s := inventory.Source{Kind: r.kind, Manager: r.manager, Shim: r.shim}
	for i, want := range pattern {
		switch want {
		case "@":
			s.Version = version(rest[i])
		case "*":
		default:
			if !eq(rest[i], want) {
				return inventory.Source{}, false
			}
		}
	}
	root := len(dir) + len(pattern)
	if r.shim {
		root = len(dir)
	}
	sep := "/"
	if win {
		sep = `\`
	}
	s.Root = strings.Join(segs[:root], sep)
	if s.Root == "" {
		s.Root = sep
	}
	return s, true
}

// split cuts a path into its segments, dropping empty ones but the leading
// one of an absolute Unix path, and tells whether it is a Windows path.
func split(p string) (segs []string, win bool) {
	win = len(p) >= 2 && p[1] == ':' || strings.HasPrefix(p, `\\`)
	if win {
		p = strings.ReplaceAll(p, `\`, "/")
	}
	for i, s := range strings.Split(p, "/") {
		if s != "" || i == 0 {
			segs = append(segs, s)
		}
	}
	return segs, win
}

// join appends rest to dir with dir's separator.
func join(dir, rest string) string {
	if _, win := split(dir); win {
		return strings.TrimRight(dir, `\/`) + `\` + rest
	}
	return strings.TrimRight(dir, "/") + "/" + rest
}

// version turns a version directory's name into a version: "v22.11.0" is
// "22.11.0", and uv's "cpython-3.12.8-macos-aarch64-none" is "3.12.8".
func version(name string) string {
	if len(name) > 1 && name[0] == 'v' && name[1] >= '0' && name[1] <= '9' {
		return name[1:]
	}
	if rest, ok := strings.CutPrefix(name, "cpython-"); ok {
		v, _, _ := strings.Cut(rest, "-")
		return v
	}
	return name
}
