package ops

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// VersionAction is what to do with a version of a language.
type VersionAction int

const (
	VersionInstall VersionAction = iota
	VersionUninstall
	VersionDefault // make it the version new terminals start
)

// Detect finds a tool again, as a scan would: a plan's check uses it.
type Detect func(ctx context.Context) (inventory.ToolState, error)

// The scripts below run a manager that is a shell function with the
// arguments after them. They are fixed: the version goes in as an
// argument, never into them.
const (
	// nvmScript: --no-use keeps nvm.sh from using the default version,
	// which nvm would then refuse to uninstall.
	nvmScript = `. "${NVM_DIR:-$HOME/.nvm}/nvm.sh" --no-use && nvm "$@"`
	rvmScript = `. "${rvm_path:-$HOME/.rvm}/scripts/rvm" && rvm "$@"`
	sdkScript = `. "${SDKMAN_DIR:-$HOME/.sdkman}/bin/sdkman-init.sh" && sdk "$@"`
	// sdkTemurinScript, run as sdk install java <line>, installs the
	// newest Temurin of the line SDKMAN lists, as 25.0.5.0+8-tem: SDKMAN
	// takes a version by its whole name alone. It answers no when sdk
	// install asks to make the new version the default.
	sdkTemurinScript = `. "${SDKMAN_DIR:-$HOME/.sdkman}/bin/sdkman-init.sh" &&
v=$(sdk list java | awk -v line="$3." 'index($NF, line) == 1 && $NF ~ /-tem$/ { print $NF; exit }') &&
if [ -z "$v" ]; then echo "SDKMAN lists no Temurin $3" >&2; exit 1; fi &&
sdk "$1" "$2" "$v" <<< n`
)

// VersionTarget is the version an operation is for and the manager it
// goes through: the copy's, or the one in use.
type VersionTarget struct {
	// Tool is the tool's ID, as "python"; Title its name, as "Python".
	Tool, Title string
	// Version is as the manager names it: the release to install, or the
	// copy's own name, as the folder it keeps it in. Label is the version
	// as the user sees it, when it differs.
	Manager, Version, Label string
	// Root is the copy's Source.Root, which names Homebrew's formula and
	// the name mise and asdf keep the tool by.
	Root string
	// AsdfSet reports asdf 0.16 or later, which sets versions with asdf
	// set; Admin a manager that needs the administrator, as Manager.Admin.
	AsdfSet, Admin bool
}

// VersionPlan returns the plan that does action to t, with the commands
// the user would type; ok is false when Hearth cannot do it through that
// manager.
func VersionPlan(action VersionAction, t VersionTarget, detect Detect) (p Plan, ok bool) {
	if action == VersionInstall && t.Tool == "java" {
		t.Version = javaSpec(t.Manager, t.Version)
	}
	argv := versionCommand(action, t)
	if argv == nil {
		return Plan{}, false
	}
	manager, label := t.Manager, cmp.Or(t.Label, t.Version)
	p.Steps = []Step{{Argv: argv, Admin: t.Admin}}
	if argv[0] == "bash" {
		// A shell function: the plan shows what the script runs.
		p.Steps[0].Runs = argv[3:]
	}
	switch action {
	case VersionInstall:
		p.Title, p.Why = locales.VersionInstallTitle(t.Title, label), locales.VersionInstallWhy(manager, t.Title)
		p.Changes = []string{"+ " + t.Title + " " + label + " · " + manager}
	case VersionUninstall:
		p.Title, p.Why = locales.VersionUninstallTitle(t.Title, label), uninstallWhy(t.Tool, manager)
		p.Changes = []string{"− " + t.Title + " " + label + " · " + manager}
	case VersionDefault:
		p.Title, p.Why = locales.VersionDefaultTitle(t.Title, label), locales.VersionDefaultWhy(manager)
		p.Changes = []string{locales.VersionDefaultChange(t.Title, label, manager)}
		p.RestartTerminal = true
		if manager == "nvm-windows" {
			p.Notes = []Note{{Title: locales.NoteAdmin(), Text: locales.NoteNvmWindowsUAC(), Tag: "UAC"}}
		}
	}
	if t.Admin {
		p.Notes = append(p.Notes, AdminNote())
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			return checkVersion(action, t, label, st)
		}
	}
	return p, true
}

// uninstallWhy says what goes with a version of tool removed through
// manager.
func uninstallWhy(tool, manager string) string {
	switch tool {
	case "node":
		return locales.NodeUninstallWhy(manager)
	case "python":
		return locales.PythonUninstallWhy(manager)
	case "ruby":
		return locales.RubyUninstallWhy(manager)
	}
	return locales.VersionUninstallWhy(manager)
}

// managerTools are the tools a manager keeps, but for mise and asdf,
// which keep any.
var managerTools = map[string][]string{
	"fnm": {"node"}, "nvm": {"node"}, "nvm-windows": {"node"}, "nodenv": {"node"}, "n": {"node"}, "volta": {"node"},
	"pyenv": {"python"}, "uv": {"python"},
	"rbenv": {"ruby"}, "rvm": {"ruby"},
	"sdkman":   {"java"},
	"rustup":   {"rust"},
	"corepack": {"pnpm", "yarn"},
	// Homebrew's other copies are packages, which the package operation
	// changes.
	"brew": {"node"},
}

// knownDefaults are the managers whose default version of a tool a scan
// tells: only that default can be made and checked.
var knownDefaults = map[string][]string{
	"node":   {"fnm", "nvm", "nvm-windows", "mise", "asdf", "nodenv", "n", "volta"},
	"bun":    {"mise"},
	"python": {"pyenv", "mise"},
	"ruby":   {"rbenv", "rvm", "asdf", "mise"},
	"java":   {"sdkman", "mise"},
	"go":     {"mise"},
	"rust":   {"rustup"},
	"php":    {"mise"},
	"dotnet": {"mise"},
	"pnpm":   {"corepack"},
	"yarn":   {"corepack"},
}

// asdfPlugins are the names of asdf's plugins that differ from the
// tool's ID.
var asdfPlugins = map[string]string{"node": "nodejs", "go": "golang", "dotnet": "dotnet-core"}

// versionCommand is the argv that does action to t; nil when Hearth
// cannot.
func versionCommand(action VersionAction, t VersionTarget) []string {
	manager, tool, v := t.Manager, t.Tool, t.Version
	if tools, ok := managerTools[manager]; ok && !slices.Contains(tools, tool) || action == VersionDefault && !slices.Contains(knownDefaults[tool], manager) {
		return nil
	}
	// mise and asdf build PHP from source, which takes libraries a
	// computer seldom has: Hearth installs it with the system's manager.
	if action == VersionInstall && tool == "php" {
		return nil
	}
	// Yarn's current releases install through Corepack alone.
	if action == VersionInstall && tool == "yarn" && manager != "corepack" {
		return nil
	}
	// The name mise and asdf keep a tool by is the folder its versions
	// are in.
	name := ""
	if t.Root != "" {
		name = filepath.Base(filepath.Dir(t.Root))
	}
	var argvs [3][]string // by action: install, uninstall, default
	switch manager {
	case "mise":
		at := cmp.Or(name, tool) + "@" + v
		argvs = [3][]string{{"mise", "install", at}, {"mise", "uninstall", at}, {"mise", "use", "-g", at}}
	case "asdf":
		plugin := cmp.Or(name, asdfPlugins[tool], tool)
		set := []string{"asdf", "global", plugin, v}
		if t.AsdfSet {
			set = []string{"asdf", "set", "-u", plugin, v}
		}
		argvs = [3][]string{{"asdf", "install", plugin, v}, {"asdf", "uninstall", plugin, v}, set}
		// asdf takes a Java by its vendor's whole name, which Hearth
		// does not know before it is installed.
		if tool == "java" {
			argvs[VersionInstall] = nil
		}
	case "fnm":
		argvs = [3][]string{{"fnm", "install", v}, {"fnm", "uninstall", v}, {"fnm", "default", v}}
	case "nvm":
		argvs = shellFunction(nvmScript, "nvm", []string{"install", v}, []string{"uninstall", v}, []string{"alias", "default", v})
	case "nvm-windows":
		argvs = [3][]string{{"nvm", "install", v}, {"nvm", "uninstall", v}, {"nvm", "use", v}}
	case "nodenv", "pyenv", "rbenv":
		argvs = [3][]string{{manager, "install", v}, {manager, "uninstall", "-f", v}, {manager, "global", v}}
	case "n":
		// n download keeps a version without putting it in use; n
		// <version> copies it into the prefix.
		argvs = [3][]string{{"n", "download", v}, {"n", "rm", v}, {"n", v}}
	case "volta":
		// Volta keeps the versions it fetched and has no command that
		// removes one.
		argvs = [3][]string{{"volta", "fetch", "node@" + v}, nil, {"volta", "install", "node@" + v}}
	case "uv":
		// uv keeps no default: python3 is not one of its.
		argvs = [3][]string{{"uv", "python", "install", v}, {"uv", "python", "uninstall", v}, nil}
	case "rvm":
		argvs = shellFunction(rvmScript, "rvm", []string{"install", v}, []string{"remove", v}, []string{"alias", "create", "default", v})
	case "sdkman":
		argvs = shellFunction(sdkScript, "sdk", nil, []string{"uninstall", "java", v}, []string{"default", "java", v})
		argvs[VersionInstall] = []string{"bash", "-c", sdkTemurinScript, "sdk", "install", "java", v}
	case "rustup":
		argvs = [3][]string{{"rustup", "toolchain", "install", v}, {"rustup", "toolchain", "uninstall", v}, {"rustup", "default", v}}
	case "corepack":
		// corepack install -g makes the version the default; Corepack has
		// no command that removes one from its cache.
		at := tool + "@" + v
		argvs = [3][]string{{"corepack", "install", "-g", at}, nil, {"corepack", "install", "-g", at}}
	case "brew":
		// A formula holds one version of a line: Hearth only removes it.
		if t.Root != "" {
			argvs[VersionUninstall] = []string{"brew", "uninstall", name}
		}
	}
	return argvs[action]
}

// shellFunction is the argvs that run the function name through script
// with the arguments of each action.
func shellFunction(script, name string, install, uninstall, def []string) [3][]string {
	argv := func(args []string) []string { return append([]string{"bash", "-c", script, name}, args...) }
	return [3][]string{argv(install), argv(uninstall), argv(def)}
}

// javaSpec is how manager names the newest Temurin of a line, as "25":
// mise by the prefix of its own names, which it installs the newest of.
// SDKMAN's script finds it in SDKMAN's list.
func javaSpec(manager, line string) string {
	if manager == "mise" {
		return "temurin-" + line
	}
	return line
}

// matches reports that in is version, as the manager names it: its own
// name for it, or one it starts, as mise's temurin-21.0.5+11.0.LTS of
// temurin-21.0.5+11, or rustup's stable-aarch64-apple-darwin of stable.
func matches(in inventory.Installation, version string) bool {
	if in.Rust != nil && (in.Rust.Name == version || strings.HasPrefix(in.Rust.Name, version+"-")) {
		return true
	}
	name := cmp.Or(in.Source.Version, in.Version)
	return name == version || in.Version == version || strings.HasPrefix(name, version+".")
}

// checkVersion says whether st, found after the plan ran, shows action
// done to t; label is its version as the user sees it.
func checkVersion(action VersionAction, t VersionTarget, label string, st inventory.ToolState) (string, error) {
	var found, isDefault bool
	for _, in := range st.Installations {
		if in.Source.Manager == t.Manager && !in.Source.Shim && matches(in, t.Version) {
			found = true
			isDefault = isDefault || in.Default
		}
	}
	tool, manager := t.Title, t.Manager
	switch {
	case action == VersionInstall && found:
		return locales.VersionCheckInstalled(tool, label, manager), nil
	case action == VersionInstall:
		return "", errors.New(locales.VersionCheckNotInstalled(tool, label, manager))
	case action == VersionUninstall && !found:
		return locales.VersionCheckRemoved(tool, label), nil
	case action == VersionUninstall:
		return "", errors.New(locales.VersionCheckNotRemoved(tool, label, manager))
	case isDefault:
		return locales.VersionCheckDefault(tool, label, manager), nil
	}
	return "", errors.New(locales.VersionCheckNotDefault(tool, label, manager))
}

// AsdfSet reports that asdf's version, as the detector found it, is 0.16
// or later.
func AsdfSet(version string) bool {
	parts := strings.SplitN(strings.TrimPrefix(version, "v"), ".", 3)
	if len(parts) < 2 {
		return true // unknown: the current asdf
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	return err1 != nil || err2 != nil || major > 0 || minor >= 16
}
