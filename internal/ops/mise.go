package ops

import (
	"context"
	"errors"
	"path/filepath"
	"slices"

	"github.com/Blinks44/hearth/internal/sources"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// ErrMiseReady reports that mise is installed and hooked in already:
// there is nothing to set up.
var ErrMiseReady = errors.New("mise is installed and hooked in")

// MiseSetup is what puts mise on the computer and hooks it into the
// user's shell, so that new terminals run the versions it installs.
type MiseSetup struct {
	// Steps install mise, when it is not there, and hook it in, when it
	// is not; none when both are done.
	Steps []Step
	Notes []Note
	// Base are how many of the first Steps install what mise installs
	// with: Homebrew on macOS.
	Base int
	// Mise is the program, where it is or will be once Steps ran.
	Mise string
}

// MisePlan returns the plan that sets up mise, checked in a new terminal;
// ErrMiseReady when there is nothing to do. env is the terminal's.
func MisePlan(env sysenv.Env, terminal Terminal) (Plan, error) {
	s, err := SetUpMise(env)
	if err != nil {
		return Plan{}, err
	}
	if len(s.Steps) == 0 {
		return Plan{}, ErrMiseReady
	}
	p := Plan{
		Title: locales.MiseSetupTitle(), Why: locales.MiseSetupWhy(), Steps: s.Steps, Notes: s.Notes,
		Changes: []string{locales.MiseSetupChange()},
	}
	if terminal != nil {
		p.Verify = func(ctx context.Context) (string, error) {
			env, err := terminal(ctx)
			if err != nil {
				return "", err
			}
			return checkMise(env)
		}
	}
	return p, nil
}

// WithMise returns p, a plan that runs mise, after the steps that set mise
// up: p's steps run the mise s installs, wherever the terminal's PATH
// does not find it yet.
func WithMise(p Plan, s MiseSetup) Plan {
	steps := make([]Step, 0, len(s.Steps)+len(p.Steps))
	steps = append(steps, s.Steps...)
	for _, step := range p.Steps {
		if len(step.Argv) > 0 && step.Argv[0] == "mise" {
			step.Argv = append([]string{s.Mise}, step.Argv[1:]...)
		}
		steps = append(steps, step)
	}
	p.Steps = steps
	p.Notes = append(append([]Note(nil), s.Notes...), p.Notes...)
	if len(s.Steps) > 0 {
		p.Why = locales.MiseFirst(p.Why)
	}
	return p
}

// checkMise says whether env, a new terminal's, runs mise's versions: the
// shell activates it, or its shims are on PATH.
func checkMise(env sysenv.Env) (string, error) {
	if env.Get("MISE_SHELL") != "" {
		return locales.MiseCheckActivated(), nil
	}
	shims := filepath.Join(sources.ManagerDirs(env).Mise, "shims")
	if slices.ContainsFunc(filepath.SplitList(env.Get("PATH")), func(d string) bool { return sysenv.SamePath(d, shims) }) {
		return locales.MiseCheckShims(), nil
	}
	return "", errors.New(locales.MiseCheckNot())
}
