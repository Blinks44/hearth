package ops

import (
	"context"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/locales"
)

// UpdatePlan joins plans that update what different managers keep into
// one, which runs them in turn and checks each; one plan stays as it is.
// ok is false when there are none.
func UpdatePlan(plans []Plan) (p Plan, ok bool) {
	switch len(plans) {
	case 0:
		return Plan{}, false
	case 1:
		return plans[0], true
	}
	p.Title, p.Why = locales.UpdatesPlanTitle(len(plans)), locales.UpdatesPlanWhy()
	var checks []func(context.Context) (string, error)
	for _, one := range plans {
		p.Steps = append(p.Steps, one.Steps...)
		p.Changes = append(p.Changes, one.Changes...)
		for _, n := range one.Notes {
			if !slices.Contains(p.Notes, n) {
				p.Notes = append(p.Notes, n)
			}
		}
		if one.Verify != nil {
			checks = append(checks, one.Verify)
		}
	}
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
