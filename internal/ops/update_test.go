package ops

import (
	"context"
	"errors"
	"testing"
)

// One plan stays as it is; several run in turn, with a note each says
// once, and their checks all pass or the first that does not fails.
func TestUpdatePlan(t *testing.T) {
	if _, ok := UpdatePlan(nil); ok {
		t.Error("a plan of none")
	}
	brew := Plan{Title: "Update git", Steps: []Step{{Argv: []string{"brew", "upgrade", "git"}}}, Changes: []string{"git"},
		Verify: func(context.Context) (string, error) { return "git updated", nil }}
	if p, _ := UpdatePlan([]Plan{brew}); p.Title != "Update git" {
		t.Errorf("one plan became %q", p.Title)
	}
	admin := Note{Title: "Administrator"}
	apt := Plan{Title: "Update curl", Steps: []Step{{Argv: []string{"apt-get", "install", "curl"}, Admin: true}}, Changes: []string{"curl"},
		Notes: []Note{admin}, Verify: func(context.Context) (string, error) { return "curl updated", nil }}
	snap := Plan{Title: "Update code", Steps: []Step{{Argv: []string{"snap", "refresh", "code"}, Admin: true}}, Notes: []Note{admin}}
	p, ok := UpdatePlan([]Plan{brew, apt, snap})
	if !ok || p.Title != "Update through 3 managers" || len(p.Steps) != 3 || len(p.Changes) != 2 || len(p.Notes) != 1 {
		t.Fatalf("plan %+v", p)
	}
	if said, err := p.Verify(context.Background()); err != nil || said != "git updated; curl updated" {
		t.Errorf("check %q, %v", said, err)
	}
	apt.Verify = func(context.Context) (string, error) { return "", errors.New("curl not updated") }
	p, _ = UpdatePlan([]Plan{brew, apt})
	if _, err := p.Verify(context.Background()); err == nil || err.Error() != "curl not updated" {
		t.Errorf("check %v", err)
	}
}
