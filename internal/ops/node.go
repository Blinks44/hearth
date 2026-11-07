package ops

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// NodeAction is what to do with a version of Node.
type NodeAction int

const (
	NodeInstall NodeAction = iota
	NodeUninstall
	NodeDefault // make it the version new terminals start
)

// Detect finds a tool again, as a scan would: a plan's check uses it.
type Detect func(ctx context.Context) (inventory.ToolState, error)

// nvmScript runs nvm, a shell function, with the arguments after it. The
// script is fixed: the version goes in as an argument, never into it.
// --no-use keeps nvm.sh from using the default version, which nvm would
// then refuse to uninstall.
const nvmScript = `. "${NVM_DIR:-$HOME/.nvm}/nvm.sh" --no-use && nvm "$@"`

// NodePlan returns the plan that does action to version through manager,
// the copy's or the one in use, with the commands the user would type;
// ok is false when Hearth cannot do it through that manager. root is the
// copy's Source.Root, which names Homebrew's formula; asdfSet that asdf
// is 0.16 or later, which sets versions with asdf set.
func NodePlan(action NodeAction, manager, version, root string, asdfSet bool, detect Detect) (p Plan, ok bool) {
	argv := nodeCommand(action, manager, version, root, asdfSet)
	if argv == nil {
		return Plan{}, false
	}
	p.Steps = []Step{{Argv: argv}}
	if manager == "nvm" {
		// nvm is a shell function: the step says what the script runs.
		p.Steps[0].Note = strings.Join(argv[3:], " ")
	}
	switch action {
	case NodeInstall:
		p.Title, p.Why = locales.NodeInstallTitle(version), locales.NodeInstallWhy(manager)
		p.Changes = []string{"+ Node v" + version + " · " + manager}
	case NodeUninstall:
		p.Title, p.Why = locales.NodeUninstallTitle(version), locales.NodeUninstallWhy(manager)
		p.Changes = []string{"− Node v" + version + " · " + manager}
	case NodeDefault:
		p.Title, p.Why = locales.NodeDefaultTitle(version), locales.NodeDefaultWhy(manager)
		p.Changes = []string{locales.NodeDefaultChange(version, manager)}
		if manager == "nvm-windows" {
			p.Notes = []Note{{Title: locales.NoteAdmin(), Text: locales.NoteNvmWindowsUAC(), Tag: "UAC"}}
		}
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			return checkNode(action, manager, version, st)
		}
	}
	return p, true
}

// nodeCommand is the argv that does action through manager; nil when
// Hearth cannot.
func nodeCommand(action NodeAction, manager, version, root string, asdfSet bool) []string {
	var argvs [3][]string // by action: install, uninstall, default
	switch manager {
	case "fnm":
		argvs = [3][]string{{"fnm", "install", version}, {"fnm", "uninstall", version}, {"fnm", "default", version}}
	case "nvm":
		nvm := []string{"bash", "-c", nvmScript, "nvm"}
		argvs = [3][]string{
			append(nvm[:4:4], "install", version),
			append(nvm[:4:4], "uninstall", version),
			append(nvm[:4:4], "alias", "default", version),
		}
	case "nvm-windows":
		argvs = [3][]string{{"nvm", "install", version}, {"nvm", "uninstall", version}, {"nvm", "use", version}}
	case "mise":
		argvs = [3][]string{{"mise", "install", "node@" + version}, {"mise", "uninstall", "node@" + version}, {"mise", "use", "-g", "node@" + version}}
	case "asdf":
		set := []string{"asdf", "global", "nodejs", version}
		if asdfSet {
			set = []string{"asdf", "set", "-u", "nodejs", version}
		}
		argvs = [3][]string{{"asdf", "install", "nodejs", version}, {"asdf", "uninstall", "nodejs", version}, set}
	case "nodenv":
		argvs = [3][]string{{"nodenv", "install", version}, {"nodenv", "uninstall", "-f", version}, {"nodenv", "global", version}}
	case "volta":
		// Volta keeps the versions it fetched and has no command that
		// removes one.
		argvs = [3][]string{{"volta", "fetch", "node@" + version}, nil, {"volta", "install", "node@" + version}}
	case "brew":
		// A formula holds one version of a line: Hearth only removes it.
		if root != "" {
			argvs[NodeUninstall] = []string{"brew", "uninstall", filepath.Base(filepath.Dir(root))}
		}
	}
	return argvs[action]
}

// checkNode says whether st, found after the plan ran, shows action done.
func checkNode(action NodeAction, manager, version string, st inventory.ToolState) (string, error) {
	var found, isDefault bool
	for _, in := range st.Installations {
		if in.Version == version && in.Source.Manager == manager && !in.Source.Shim {
			found = true
			isDefault = isDefault || in.Default
		}
	}
	switch {
	case action == NodeInstall && found:
		return locales.NodeCheckInstalled(version, manager), nil
	case action == NodeInstall:
		return "", errors.New(locales.NodeCheckNotInstalled(version, manager))
	case action == NodeUninstall && !found:
		return locales.NodeCheckRemoved(version), nil
	case action == NodeUninstall:
		return "", errors.New(locales.NodeCheckNotRemoved(version, manager))
	case isDefault:
		return locales.NodeCheckDefault(version, manager), nil
	}
	return "", errors.New(locales.NodeCheckNotDefault(version, manager))
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
