package ops

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/locales"
)

var rustupIdent = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidRustupIdent reports whether s is a safe rustup toolchain or target
// identifier (no spaces, control characters, or leading dash).
func ValidRustupIdent(s string) bool {
	return rustupIdent.MatchString(strings.TrimSpace(s))
}

// RustTargetPlan returns the plan that adds or removes target in toolchain
// through rustup.
func RustTargetPlan(add bool, toolchain, target string, detect Detect) (Plan, error) {
	toolchain = strings.TrimSpace(toolchain)
	target = strings.TrimSpace(target)
	if !rustupIdent.MatchString(toolchain) || !rustupIdent.MatchString(target) {
		return Plan{}, ErrNoFix
	}
	sub := "remove"
	p := Plan{
		Title:   locales.RustTargetRemoveTitle(target),
		Why:     locales.RustTargetRemoveWhy(toolchain),
		Changes: []string{"− " + target + " · " + toolchain},
	}
	if add {
		sub = "add"
		p.Title = locales.RustTargetAddTitle(target)
		p.Why = locales.RustTargetAddWhy(toolchain)
		p.Changes = []string{"+ " + target + " · " + toolchain}
	}
	p.Steps = []Step{{Argv: []string{"rustup", "target", sub, "--toolchain", toolchain, target}}}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			exact := false
			for _, in := range st.Installations {
				if in.Rust != nil && in.Rust.Name == toolchain {
					exact = true
					break
				}
			}
			has := false
			for _, in := range st.Installations {
				if in.Rust == nil {
					continue
				}
				matched := in.Rust.Name == toolchain || (!exact && strings.HasPrefix(in.Rust.Name, toolchain+"-"))
				if matched && slices.Contains(in.Rust.Targets, target) {
					has = true
					break
				}
			}
			switch {
			case add && has:
				return locales.RustTargetCheckAdded(target, toolchain), nil
			case add:
				return "", errors.New(locales.RustTargetCheckNotAdded(target, toolchain))
			case !has:
				return locales.RustTargetCheckRemoved(target, toolchain), nil
			default:
				return "", errors.New(locales.RustTargetCheckNotRemoved(target, toolchain))
			}
		}
	}
	return p, nil
}

// RustComponentPlan returns the plan that adds or removes component in
// toolchain through rustup.
func RustComponentPlan(add bool, toolchain, component string, detect Detect) (Plan, error) {
	toolchain = strings.TrimSpace(toolchain)
	component = strings.TrimSpace(component)
	if !rustupIdent.MatchString(toolchain) || !rustupIdent.MatchString(component) {
		return Plan{}, ErrNoFix
	}
	sub := "remove"
	p := Plan{
		Title:   locales.RustComponentRemoveTitle(component),
		Why:     locales.RustComponentRemoveWhy(toolchain),
		Changes: []string{"− " + component + " · " + toolchain},
	}
	if add {
		sub = "add"
		p.Title = locales.RustComponentAddTitle(component)
		p.Why = locales.RustComponentAddWhy(toolchain)
		p.Changes = []string{"+ " + component + " · " + toolchain}
	}
	p.Steps = []Step{{Argv: []string{"rustup", "component", sub, "--toolchain", toolchain, component}}}
	if detect != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			st, err := detect(ctx)
			if err != nil {
				return "", err
			}
			exact := false
			for _, in := range st.Installations {
				if in.Rust != nil && in.Rust.Name == toolchain {
					exact = true
					break
				}
			}
			has := false
			for _, in := range st.Installations {
				if in.Rust == nil {
					continue
				}
				matched := in.Rust.Name == toolchain || (!exact && strings.HasPrefix(in.Rust.Name, toolchain+"-"))
				if matched && slices.Contains(in.Rust.Components, component) {
					has = true
					break
				}
			}
			switch {
			case add && has:
				return locales.RustComponentCheckAdded(component, toolchain), nil
			case add:
				return "", errors.New(locales.RustComponentCheckNotAdded(component, toolchain))
			case !has:
				return locales.RustComponentCheckRemoved(component, toolchain), nil
			default:
				return "", errors.New(locales.RustComponentCheckNotRemoved(component, toolchain))
			}
		}
	}
	return p, nil
}
