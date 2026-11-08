package ops

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/locales"
)

// MovePlan returns the plan that moves a copy of t.Tool, from source, to
// mise: mise installs the same version, Java's newest Temurin of its line,
// and makes it the default. The copy stays: removing it is its own
// operation. ok is false for a tool mise does not set the default of.
func MovePlan(t VersionTarget, source string, detect Detect) (p Plan, ok bool) {
	if !slices.Contains(knownDefaults[t.Tool], "mise") || t.Tool == "php" || t.Version == "" {
		return Plan{}, false
	}
	t.Manager = "mise"
	label := cmp.Or(t.Label, t.Version)
	if t.Tool == "java" {
		t.Version = javaSpec("mise", t.Version)
	}
	// mise use installs the version when it is not there yet.
	p.Steps = []Step{{Argv: versionCommand(VersionDefault, t)}}
	p.Title, p.Why = locales.MoveTitle(t.Title, label), locales.MoveWhy(source)
	p.Changes = []string{"+ " + t.Title + " " + label + " · mise", locales.VersionDefaultChange(t.Title, label, "mise")}
	p.Notes = []Note{{Title: locales.MovePackagesTitle(), Text: locales.MovePackagesText(source)}}
	if t.Tool == "python" {
		p.Notes = append(p.Notes, Note{Title: locales.MoveVenvTitle(), Text: locales.MoveVenvText()})
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			return checkVersion(VersionDefault, t, label, st)
		}
	}
	return p, true
}

// The folders python.org's installer for macOS puts a version in.
var (
	pythonOrgVersions = "/Library/Frameworks/Python.framework/Versions"
	pythonOrgApps     = "/Applications"
	pythonOrgBin      = "/usr/local/bin"
)

// PythonOrgVersion is the version folder of the python.org copy at path,
// as 3.13 of /Library/Frameworks/Python.framework/Versions/3.13/bin/python3;
// "" when it is not one.
func PythonOrgVersion(path string) string {
	rest, ok := strings.CutPrefix(filepath.ToSlash(path), pythonOrgVersions+"/")
	if !ok {
		return ""
	}
	version, _, _ := strings.Cut(rest, "/")
	if version == "Current" {
		return ""
	}
	return version
}

// PythonOrgRemove returns the plan that removes the python.org copy of
// version, as 3.13: python.org has no uninstaller, so the plan deletes
// what its installer put, each path shown: the version's folder, or the
// framework with the last version, its folder in Applications and the links to it in /usr/local/bin. label is
// the version as the page shows it. It reads the folders, so it runs off
// the UI thread.
func PythonOrgRemove(version, label string, detect Detect) (Plan, error) {
	dir := filepath.Join(pythonOrgVersions, version)
	if _, err := os.Stat(dir); err != nil {
		return Plan{}, err
	}
	paths := []string{dir}
	// The last version takes the framework with it.
	versions, _ := os.ReadDir(pythonOrgVersions)
	if !slices.ContainsFunc(versions, func(v os.DirEntry) bool { return v.Name() != version && v.Name() != "Current" }) {
		paths[0] = filepath.Dir(pythonOrgVersions)
	}
	if app := filepath.Join(pythonOrgApps, "Python "+version); exists(app) {
		paths = append(paths, app)
	}
	links, _ := os.ReadDir(pythonOrgBin)
	for _, l := range links {
		link := filepath.Join(pythonOrgBin, l.Name())
		if target, err := os.Readlink(link); err == nil && strings.HasPrefix(absLink(link, target), dir+string(filepath.Separator)) {
			paths = append(paths, link)
		}
	}
	p := Plan{
		Title: locales.VersionUninstallTitle("Python", label),
		Why:   locales.PythonOrgRemoveWhy(),
		Steps: []Step{{Argv: append([]string{"rm", "-rf"}, paths...), Admin: true}},
		Notes: []Note{{Title: locales.PythonOrgPaths(), Text: strings.Join(paths, "\n")}, {Title: locales.PythonOrgNoUninstaller(), Text: locales.PythonOrgNoUninstallerText(), URL: "https://docs.python.org/3/using/mac.html"}, AdminNote()},
	}
	for _, path := range paths {
		p.Changes = append(p.Changes, "− "+path)
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			for _, in := range st.Installations {
				if PythonOrgVersion(in.RealPath) == version {
					return "", errors.New(locales.VersionCheckNotRemoved("Python", label, "python.org"))
				}
			}
			return locales.VersionCheckRemoved("Python", label), nil
		}
	}
	return p, nil
}

// absLink is where a link's target lies: relative targets go from the
// link's folder.
func absLink(link, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Join(filepath.Dir(link), target)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
