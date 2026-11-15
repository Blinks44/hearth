package ui

import (
	"errors"
	"time"

	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/proposals"
	"github.com/Blinks44/hearth/locales"
)

// RequestFix runs on the UI thread. It only selects the existing plan action;
// execution remains exclusively in startOperation, after a human click.
func (a *App) RequestFix(q proposals.Request) error {
	if !q.Valid() {
		return errors.New("invalid issue ID; call get_issues again")
	}
	if a.mcpPending || a.op != nil && (a.op.open || a.op.running()) || a.setup.op.running() {
		return errors.New("hearth is showing or preparing another operation; finish or dismiss it first")
	}
	if a.Scanning {
		return errors.New("hearth is scanning; wait and call get_issues again")
	}
	found := issues.Find(a.Snapshot, a.Projects, a.NodeVersions, time.Now())
	for _, is := range found {
		if issues.ID(is) != q.IssueID {
			continue
		}
		previous := a.op
		fix := a.issueFixPlan(is, found, func(p ops.Plan, err error) {
			a.mcpPending = false
			if a.op != previous {
				return
			} // Never replace a plan the human opened meanwhile.
			p.Why = locales.McpProposalOrigin() + "\n\n" + p.Why
			a.proposeOrSay(p, err)
		})
		if fix == nil {
			return errors.New("this issue has no automatic fix; review it in Hearth")
		}
		a.mcpPending = true
		fix()
		return nil
	}
	return errors.New("issue no longer matches the desktop snapshot; call get_issues again")
}
