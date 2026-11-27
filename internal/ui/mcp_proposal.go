package ui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/cleanup"
	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/proposals"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// RequestFix runs on the UI thread. It only selects an existing desktop plan
// action; execution remains exclusively in startOperation, after a human click.
func (a *App) RequestFix(q proposals.Request) error {
	if !q.Valid() {
		if q.EffectiveKind() == proposals.KindFix {
			return errors.New("invalid issue ID; call get_issues again")
		}
		return errors.New("invalid proposal request; check parameters against the latest read-only query")
	}
	if a.mcpPending || a.preparing != nil || a.op != nil && (a.op.open || a.op.running()) || a.setup.op.running() {
		return errors.New("hearth is showing or preparing another operation; finish or dismiss it first")
	}
	if a.Scanning {
		return errors.New("hearth is scanning; wait and retry")
	}
	if a.Snapshot == nil {
		return errors.New("hearth has no snapshot yet; wait for the first scan to finish")
	}

	switch q.EffectiveKind() {
	case proposals.KindFix:
		return a.requestIssueFix(q.IssueID)
	case proposals.KindProjectPrepare:
		return a.requestProjectPrepare(q.Project)
	case proposals.KindStopProcess:
		return a.requestStopProcess(q.Port, q.PID)
	case proposals.KindServiceAction:
		return a.requestServiceAction(q.Service, q.Action)
	case proposals.KindSkillAction:
		return a.requestSkillAction(q.Action, q.Skill, q.Project)
	case proposals.KindToolInstall:
		return a.requestToolInstall(q.Tool, q.Version)
	case proposals.KindCleanup:
		return a.requestCleanup(q.Kinds)
	default:
		return errors.New("unsupported proposal kind")
	}
}

func prependMCPOrigin(p *ops.Plan) {
	origin := locales.McpProposalOrigin()
	if p.Why != "" {
		p.Why = origin + "\n\n" + p.Why
	} else {
		p.Why = origin
	}
}

func (a *App) proposeFromMCP(p ops.Plan) {
	prependMCPOrigin(&p)
	a.Propose(p)
}

func (a *App) buildPlanFromMCP(build func(sysenv.Env, ops.Terminal) (ops.Plan, error)) error {
	if a.BuildPlan == nil {
		return errors.New("hearth cannot build plans in this session")
	}
	previous := a.op
	a.mcpPending = true
	a.BuildPlan(build, func(p ops.Plan, err error) {
		a.mcpPending = false
		if a.op != previous {
			return
		} // Never replace a plan the human opened meanwhile.
		if err == nil {
			prependMCPOrigin(&p)
		}
		a.proposeOrSay(p, err)
	})
	return nil
}

func (a *App) requestIssueFix(issueID string) error {
	found := issues.FindWithLists(a.Snapshot, a.Projects, a.issueLists(), time.Now())
	for _, is := range found {
		if issues.ID(is) != issueID {
			continue
		}
		previous := a.op
		fix := a.issueFixPlan(is, found, func(p ops.Plan, err error) {
			a.mcpPending = false
			if a.op != previous {
				return
			} // Never replace a plan the human opened meanwhile.
			if err == nil {
				prependMCPOrigin(&p)
			}
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

func (a *App) requestProjectPrepare(dir string) error {
	if a.BuildPlan == nil {
		return errors.New("hearth cannot build plans in this session")
	}
	if slices.IndexFunc(a.Projects, func(p projects.Project) bool { return p.Dir == dir }) < 0 {
		return errors.New("project is not registered in Hearth; call get_projects again")
	}
	a.prepareProjectWithOrigin(dir, locales.McpProposalOrigin())
	return nil
}

func (a *App) requestStopProcess(port, pid string) error {
	st, ok := a.Snapshot.Tool("databases")
	if !ok || st.Databases == nil {
		return errors.New("no listening processes in the desktop snapshot; call get_ports_and_databases again")
	}
	for _, l := range st.Databases.Listeners {
		if l.Port != port || l.PID != pid {
			continue
		}
		if !l.Own {
			return errors.New("process is not owned by the current user and cannot be stopped from Hearth")
		}
		plan, ok := ops.ProcessPlan(l, a.detect("databases"))
		if !ok {
			return errors.New("cannot build a stop plan for this process")
		}
		a.proposeFromMCP(plan)
		return nil
	}
	return errors.New("listener no longer matches the desktop snapshot; call get_ports_and_databases again")
}

func (a *App) requestServiceAction(service, actionName string) error {
	var action ops.ServiceAction
	switch actionName {
	case "start":
		action = ops.ServiceStart
	case "stop":
		action = ops.ServiceStop
	case "restart":
		action = ops.ServiceRestart
	default:
		return errors.New("invalid service action; use start, stop, or restart")
	}
	servers := inventory.DatabaseServers(a.Snapshot)
	matched := false
	tryMatch := func(match func(inventory.Database) bool) (bool, error) {
		for _, db := range servers {
			if !match(db) {
				continue
			}
			matched = true
			plan, ok := ops.ServicePlan(action, db, a.detect("databases"))
			if !ok {
				continue
			}
			a.proposeFromMCP(plan)
			return true, nil
		}
		return false, nil
	}
	if done, err := tryMatch(func(db inventory.Database) bool { return strings.EqualFold(db.Service, service) }); done || err != nil {
		return err
	}
	if done, err := tryMatch(func(db inventory.Database) bool {
		return strings.EqualFold(db.Name, service) || strings.EqualFold(db.Kind, service)
	}); done || err != nil {
		return err
	}
	if matched {
		return errors.New("this database service cannot be controlled automatically by Hearth")
	}
	return errors.New("database service no longer matches the desktop snapshot; call get_ports_and_databases again")
}

func (a *App) requestSkillAction(action, skillName, project string) error {
	if action == "merge_duplicates" {
		if a.SkillBackups == "" {
			return errors.New("skill backup directory is unavailable")
		}
		found := issues.FindWithLists(a.Snapshot, a.Projects, a.issueLists(), time.Now())
		var groups [][]inventory.Skill
		for _, is := range found {
			if is.Kind != issues.SkillCopies {
				continue
			}
			if project != "" && is.Project != project {
				continue
			}
			if skillName != "" && (len(is.Skills) == 0 || !strings.EqualFold(is.Skills[0].Name, skillName)) {
				continue
			}
			groups = append(groups, is.Skills)
		}
		if len(groups) == 0 {
			return errors.New("no duplicate skill copies found in the desktop snapshot; call get_skills again")
		}
		backups := a.SkillBackups
		return a.buildPlanFromMCP(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			return ops.SkillMergePlan(groups, backups, env, time.Now())
		})
	}

	wantBroken := action == "remove_broken"
	findSkill := func(matchBroken bool) *inventory.Skill {
		for _, sk := range a.allSkills() {
			if !strings.EqualFold(sk.Name, skillName) {
				continue
			}
			if matchBroken && sk.Broken != wantBroken {
				continue
			}
			if project != "" && sk.Project != project {
				continue
			}
			if project == "" && sk.Project != "" {
				continue
			}
			copySk := sk
			return &copySk
		}
		if project == "" {
			// Fall back to matching any project skill with that name if no global skill matched.
			for _, sk := range a.allSkills() {
				if !strings.EqualFold(sk.Name, skillName) {
					continue
				}
				if matchBroken && sk.Broken != wantBroken {
					continue
				}
				copySk := sk
				return &copySk
			}
		}
		return nil
	}
	matched := findSkill(true)
	if matched == nil {
		matched = findSkill(false)
	}
	if matched == nil {
		return errors.New("skill no longer matches the desktop snapshot; call get_skills again")
	}
	sk := *matched

	switch action {
	case "link_all":
		if sk.Broken || sk.Synced {
			return errors.New("this skill cannot be linked across agents")
		}
		var unconnected []string
		for _, id := range a.foundIDs() {
			if id == "aider" {
				continue
			}
			connected := false
			for _, p := range sk.Paths {
				if slices.Contains(p.Agents, id) {
					connected = true
					break
				}
			}
			if !connected {
				unconnected = append(unconnected, id)
			}
		}
		if len(unconnected) == 0 {
			return errors.New("skill is already connected to all detected agents")
		}
		targets := slices.Clone(unconnected)
		return a.buildPlanFromMCP(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			return ops.SkillLinkAllPlan(sk, targets, env, agentTitle)
		})
	case "remove":
		if sk.Broken || sk.Synced {
			return errors.New("this skill cannot be removed with action=remove; use remove_broken for broken symlinks")
		}
		return a.buildPlanFromMCP(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			return ops.SkillRemovePlan(sk, env, agentTitle)
		})
	case "remove_broken":
		if !sk.Broken {
			return errors.New("this skill is not a broken symlink")
		}
		return a.buildPlanFromMCP(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
			return ops.SkillRemoveBrokenPlan(sk, env)
		})
	default:
		return errors.New("invalid skill action")
	}
}

func isVersionManagedTool(toolID string) bool {
	switch toolID {
	case "node", "python", "go", "java", "ruby", "rust", "php", "dotnet", "bun":
		return true
	}
	return false
}

func (a *App) requestToolInstall(toolID, version string) error {
	toolID = strings.ToLower(strings.TrimSpace(toolID))
	if version != "" {
		if !isVersionManagedTool(toolID) {
			return errors.New("versioned installs via MCP are only supported for Hearth-managed languages")
		}
		if !ops.ValidVersionSpec(version) {
			return errors.New("invalid version specification")
		}
		st, _ := a.toolState(toolID)
		st.Tool = toolID
		manager := a.installManager(st)
		if manager == "" {
			return errors.New("no version manager is installed for this tool; omit version to propose installing the tool via Hearth Setup")
		}
		plan, ok := a.versionPlan(ops.VersionInstall, toolID, manager, version, version, "")
		if !ok {
			return errors.New("cannot build a version install plan for this tool and manager")
		}
		a.proposeFromMCP(plan)
		return nil
	}
	if !a.canInstallTool(toolID) {
		return errors.New("tool is already installed or cannot be installed automatically by Hearth")
	}
	previous := a.op
	a.mcpPending = true
	a.installToolPlan(toolID, func(p ops.Plan, err error) {
		a.mcpPending = false
		if a.op != previous {
			return
		}
		if err == nil {
			prependMCPOrigin(&p)
		}
		a.proposeOrSay(p, err)
	})
	return nil
}

func (a *App) requestCleanup(kinds []string) error {
	if a.Sizes == nil {
		return errors.New("disk sizes have not been scanned yet; open Cleanup in Hearth first")
	}
	var opts cleanup.Options
	if a.CleanupStaleDays > 0 {
		opts.StaleAfter = time.Duration(a.CleanupStaleDays) * 24 * time.Hour
	}
	if a.CleanupMinMB > 0 {
		opts.MinFrees = int64(a.CleanupMinMB) * 1_000_000
	}
	all := cleanup.FindWithOptions(a.Snapshot, a.Sizes, a.Projects, a.nodeLines(), time.Now(), opts)
	kindSet := map[string]bool{}
	for _, k := range kinds {
		kindSet[strings.ToLower(strings.TrimSpace(k))] = true
	}
	var cands []cleanup.Candidate
	var titles []string
	for _, cand := range all {
		if !ops.Cleans(cand, a.Snapshot) {
			continue
		}
		if len(kindSet) > 0 {
			if !kindSet[strings.ToLower(string(cand.Kind))] {
				continue
			}
		} else if !cand.Selected {
			continue
		}
		cands = append(cands, cand)
		titles = append(titles, a.candidateTitle(cand))
	}
	if len(cands) == 0 {
		return errors.New("no matching cleanup candidates available in the desktop snapshot; call get_cleanup again")
	}
	target, snap, count := a.dockerTarget(), a.Snapshot, a.CountDocker
	return a.buildPlanFromMCP(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
		var docker ops.CountDocker
		if count != nil {
			docker = func(ctx context.Context) ([]inventory.DockerUsage, error) { return count(ctx, env, snap) }
		}
		return ops.CleanupPlan(cands, titles, snap, env, target, docker)
	})
}
