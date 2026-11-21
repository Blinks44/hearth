package ops

import (
	"cmp"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/locales"
)

// UpdatePlan joins plans that update versions and what different managers
// keep into one, which runs them in turn and checks each; one plan stays
// as it is.
// ok is false when there are none.
func UpdatePlan(plans []Plan) (p Plan, ok bool) {
	switch len(plans) {
	case 0:
		return Plan{}, false
	case 1:
		return plans[0], true
	}
	p.Why = locales.UpdatesPlanWhy()
	var checks []func(context.Context) (string, error)
	for _, one := range plans {
		p.Steps = append(p.Steps, one.Steps...)
		p.Changes = append(p.Changes, one.Changes...)
		p.RestartTerminal = p.RestartTerminal || one.RestartTerminal
		for _, n := range one.Notes {
			if !slices.Contains(p.Notes, n) {
				p.Notes = append(p.Notes, n)
			}
		}
		if one.Verify != nil {
			checks = append(checks, one.Verify)
		}
	}
	p.Title = locales.UpdatesPlanTitle(len(p.Changes))
	if len(checks) > 0 {
		p.Verify = func(ctx context.Context) (string, error) {
			var said []string
			for _, check := range checks {
				s, err := check(ctx)
				if err != nil {
					return "", err
				}
				said = append(said, s)
			}
			return strings.Join(said, "; "), nil
		}
	}
	return p, true
}

// VersionUpdate moves a copy to a newer release of its line.
type VersionUpdate struct {
	// From is the copy and To the release, as their manager names them.
	From, To VersionTarget
	// Default reports that From is its manager's default, which To then
	// becomes.
	Default bool
	// Globals reports packages installed globally into From, as with
	// npm -g or gem install: they go with it.
	Globals bool
	// Toolchain is rustup's name of From, which rustup update moves in
	// place; Mise is mise's name of a tool of its own, which mise upgrade
	// moves in place to the newest its config allows.
	Toolchain, Mise string
}

// VersionUpdatePlan returns the plan that does u through the copy's
// manager: in place for rustup and mise's own tools; through any other,
// To is installed, made the default when From was, and From removed. ok
// is false when Hearth cannot: a Java, whose releases its managers name
// by vendor, or a manager that does not install.
func VersionUpdatePlan(u VersionUpdate, detect Detect) (p Plan, ok bool) {
	from, to := u.From, u.To
	fromLabel, toLabel := cmp.Or(from.Label, from.Version), cmp.Or(to.Label, to.Version)
	p.Title = locales.VersionUpdateTitle(from.Title, fromLabel, toLabel)
	p.Changes = []string{from.Title + " " + fromLabel + " → " + toLabel + " · " + from.Manager}
	if u.Toolchain != "" || u.Mise != "" {
		argv := []string{"rustup", "update", u.Toolchain}
		if u.Mise != "" {
			// mise 2026.11 keeps the old version a day unless told.
			argv = []string{"mise", "upgrade", "--prune", u.Mise}
		}
		p.Why = locales.VersionUpdateInPlaceWhy(from.Manager)
		p.Steps = []Step{{Argv: argv}}
		if detect != nil {
			p.Verify = func(ctx context.Context) (string, error) {
				st, err := detect(ctx)
				if err != nil {
					return "", err
				}
				return checkInPlace(from, fromLabel, st)
			}
		}
		return p, true
	}
	if from.Tool == "java" {
		return Plan{}, false
	}
	install, ok := VersionPlan(VersionInstall, to, nil)
	if !ok {
		return Plan{}, false
	}
	parts := []Plan{install}
	if u.Default {
		def, ok := VersionPlan(VersionDefault, to, nil)
		if !ok {
			return Plan{}, false
		}
		p.RestartTerminal = true
		parts = append(parts, def)
	}
	// Volta keeps the versions it fetched: the old one stays.
	remove, removes := VersionPlan(VersionUninstall, from, nil)
	if removes {
		parts = append(parts, remove)
		p.Why = locales.VersionReplaceWhy(from.Manager)
	} else {
		p.Why = locales.VersionInstallWhy(from.Manager, from.Title)
	}
	for _, part := range parts {
		p.Steps = append(p.Steps, part.Steps...)
		for _, n := range part.Notes {
			if !slices.Contains(p.Notes, n) {
				p.Notes = append(p.Notes, n)
			}
		}
	}
	if u.Globals && removes {
		p.Notes = append(p.Notes, Note{Title: locales.NoteGlobals(), Text: locales.NoteGlobalsText(from.Title, fromLabel)})
	}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			said := []string{}
			check := func(action VersionAction, t VersionTarget, label string) error {
				s, err := checkVersion(action, t, label, st)
				said = append(said, s)
				return err
			}
			if err := check(VersionInstall, to, toLabel); err != nil {
				return "", err
			}
			if u.Default {
				if err := check(VersionDefault, to, toLabel); err != nil {
					return "", err
				}
			}
			if removes {
				if err := check(VersionUninstall, from, fromLabel); err != nil {
					return "", err
				}
			}
			return strings.Join(said, "; "), nil
		}
	}
	return p, true
}

// checkInPlace says whether st, found after rustup or mise upgrade ran,
// shows from moved past label: its toolchain, or mise's folder of the
// tool, holds another version and not that one.
func checkInPlace(from VersionTarget, label string, st inventory.ToolState) (string, error) {
	same := func(in inventory.Installation) bool {
		if in.Source.Manager != from.Manager || in.Source.Shim {
			return false
		}
		if from.Manager == "mise" {
			return filepath.Dir(in.Source.Root) == filepath.Dir(from.Root)
		}
		return in.Source.Root == from.Root
	}
	ins := slices.Clone(st.Installations)
	for _, mt := range st.MiseTools {
		ins = append(ins, mt.Installations...)
	}
	now := ""
	for _, in := range ins {
		if !same(in) {
			continue
		}
		if in.Version == from.Version {
			return "", errors.New(locales.VersionCheckNotUpdated(from.Title, label, from.Manager))
		}
		now = cmp.Or(now, in.Version)
	}
	if now == "" {
		return "", errors.New(locales.VersionCheckNotInstalled(from.Title, label, from.Manager))
	}
	return locales.VersionCheckInstalled(from.Title, now, from.Manager), nil
}
