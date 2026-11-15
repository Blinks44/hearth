package ui

import (
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/issues"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

// issueText says what an issue is and why it matters, in plain words;
// paths show home as ~.
func issueText(is issues.Issue, home string) (title, explain string) {
	tool := toolTitle(is.Tool)
	switch is.Kind {
	case issues.InstructionDiffers:
		return locales.InstructionsIssueTitle(filepath.Base(is.Project)), locales.InstructionsIssueExplain()
	case issues.DatabasePortConflict:
		return locales.DatabaseConflictTitle(is.Dirs[0]), locales.DatabaseConflictExplain(filepath.Base(is.Project))
	case issues.DockerUnavailable:
		return locales.DockerUnavailableTitle(), locales.DockerUnavailableExplain()
	case issues.DockerDenied:
		return locales.DockerDeniedTitle(), locales.DockerDeniedExplain()
	case issues.DockerSignIn:
		return locales.DockerDeniedTitle(), locales.DockerSignInExplain()
	case issues.Duplicate:
		first := is.Copies[0]
		version := copyVersion(first)
		if first.Version == "" {
			version = locales.CopyOfUnknownVersion()
		}
		var others []string
		for _, in := range is.Copies[1:] {
			others = append(others, copyVersion(in)+" ("+copySource(in)+")")
		}
		return locales.IssueDuplicateTitle(tool, len(is.Copies)),
			locales.IssueDuplicateExplain(version, copySource(first), strings.Join(others, ", "))
	case issues.GUIDiffers:
		if len(is.Copies) < 2 {
			return locales.IssueGuiMissingTitle(tool), locales.GuiNotFound(is.Tool)
		}
		term, gui := is.Copies[0], is.Copies[1]
		return locales.IssueGuiTitle(tool), locales.GuiRunsOther(sourceName(gui.Source), copyVersion(gui), copyVersion(term))
	case issues.ProjectMissing:
		want := is.Check.Want
		if want == "" {
			want = locales.AnyVersion()
		}
		return locales.IssueProjectMissingTitle(filepath.Base(is.Project), tool, want),
			locales.IssueProjectMissingExplain(is.Check.File, tool, want)
	case issues.ProjectTerminal:
		run := "—"
		if len(is.Copies) > 0 {
			run = copyVersion(is.Copies[0])
		}
		return locales.IssueProjectTerminalTitle(filepath.Base(is.Project), tool),
			locales.IssueProjectTerminalExplain(is.Check.File, tool, is.Check.Want, run)
	case issues.Unsupported:
		var vs []string
		for _, in := range is.Copies {
			vs = append(vs, copyVersion(in))
		}
		return locales.IssueUnsupportedTitle(tool, strings.Join(vs, ", ")), locales.IssueUnsupportedExplain()
	case issues.ExtraVersion:
		var vs []string
		for _, in := range is.Copies {
			vs = append(vs, copyVersion(in)+" ("+copySource(in)+")")
		}
		return locales.IssueExtraTitle(tool, len(is.Copies)), locales.IssueExtraExplain(strings.Join(vs, ", "))
	case issues.StubOnPath:
		in := is.Copies[0]
		if in.OnPath != 1 {
			return locales.IssueStubBehindTitle(tool), stubWarning([]inventory.Installation{in}, []int{0}, home)
		}
		return locales.IssueStubTitle(tool), stubWarning([]inventory.Installation{in}, []int{0}, home)
	case issues.PathCurrentDir:
		var entries []string
		for _, d := range is.Dirs {
			if d == "" {
				entries = append(entries, locales.EmptyPathEntry())
			} else {
				entries = append(entries, strconv.Quote(d))
			}
		}
		return locales.IssuePathCurrentTitle(), locales.IssuePathCurrentExplain(strings.Join(entries, ", "))
	case issues.PathMissing:
		dirs := make([]string, len(is.Dirs))
		for i, d := range is.Dirs {
			dirs[i] = tilde(d, home)
		}
		return locales.IssuePathMissingTitle(), locales.IssuePathMissingExplain(strings.Join(dirs, ", "))
	case issues.SlowStartup:
		st := is.Startup
		title = locales.IssueSlowStartupTitle(seconds(st.Took))
		if len(st.Lines) > 0 && st.Lines[0].Took > 0 {
			l := st.Lines[0]
			return title, locales.IssueSlowStartupLine(st.Shell, tilde(l.File, home)+":"+strconv.Itoa(l.Line), seconds(l.Took))
		}
		return title, locales.IssueSlowStartupExplain(st.Shell)
	case issues.SkillCopies, issues.SkillDiffers:
		dirs := make([]string, len(is.Skills))
		for i, sk := range is.Skills {
			dirs[i] = tilde(sk.Dir, home)
		}
		name := is.Skills[0].Name
		if is.Kind == issues.SkillCopies {
			return locales.IssueSkillCopiesTitle(name, len(is.Skills)), locales.IssueSkillCopiesExplain(strings.Join(dirs, ", "))
		}
		return locales.IssueSkillDiffersTitle(name), locales.IssueSkillDiffersExplain(strings.Join(dirs, ", "))
	case issues.PlainSecret:
		sv := is.MCP[0]
		return locales.IssueSecretTitle(sv.Name), secretExplain(sv, home)
	case issues.SettingsSecret:
		ag := is.Agent
		return locales.IssueSettingsSecretTitle(agentTitle(ag.ID)),
			locales.IssueSettingsSecretExplain(tilde(ag.Settings, home), strings.Join(ag.PlainEnv, ", "))
	case issues.AgentUnasked:
		r := is.Rule
		if r.Kind == inventory.RuleMode {
			return locales.IssueUnaskedModeTitle(agentTitle(r.Agent)), locales.IssueUnaskedModeExplain(tilde(r.File, home), r.Rule)
		}
		return locales.IssueUnaskedRuleTitle(agentTitle(r.Agent)), locales.IssueUnaskedRuleExplain(tilde(r.File, home), r.Rule)
	case issues.MCPMissing:
		// Servers by name, each with the agents declaring it.
		var names []string
		agents := map[string][]string{}
		for _, sv := range is.MCP {
			if _, ok := agents[sv.Name]; !ok {
				names = append(names, sv.Name)
			}
			if a := agentTitle(sv.Agent); !slices.Contains(agents[sv.Name], a) {
				agents[sv.Name] = append(agents[sv.Name], a)
			}
		}
		servers := make([]string, len(names))
		for i, n := range names {
			servers[i] = n + " (" + strings.Join(agents[n], ", ") + ")"
		}
		// The title names the command; the explanation says where it is.
		command := is.MCP[0].Command
		return locales.IssueMcpMissingTitle(filepath.Base(command)),
			locales.IssueMcpMissingExplain(strings.Join(servers, ", "), tilde(command, home))
	}
	return "", ""
}

// secretExplain says which secrets of sv its file writes out, and that
// Hearth does not move them when it cannot.
func secretExplain(sv inventory.MCPServer, home string) string {
	text := locales.IssueSecretExplain(secretWhat(sv), tilde(sv.File, home))
	if !ops.SecretMovable(sv) || sv.PlainArgs {
		text += " " + locales.SecretByHand()
	}
	return text
}

// secretWhat lists the secrets sv's file writes out: variables, headers,
// and those in its arguments or URL.
func secretWhat(sv inventory.MCPServer) string {
	what := slices.Concat(sv.PlainEnv, sv.PlainHeaders)
	if sv.PlainArgs {
		what = append(what, locales.SecretInArgs())
	}
	return strings.Join(what, ", ")
}

// skillsPage and mcpPage are where skills' and MCP servers' issues live.
const (
	skillsPage = "/ai/skills"
	mcpPage    = "/ai/mcp"
)

// copySource is sourceName, a manager's shim named as one.
func copySource(in inventory.Installation) string {
	if in.Source.Shim && in.Source.Manager != "" {
		return locales.ShimOf(in.Source.Manager)
	}
	return sourceName(in.Source)
}

// issuePage is the path of the page an issue lives on: the skills', its
// project's, its tool's, or the PATH's.
func issuePage(is issues.Issue) string {
	switch {
	case is.Kind == issues.InstructionDiffers:
		return instructionsPage
	case len(is.Skills) > 0:
		return skillsPage
	case len(is.MCP) > 0:
		return mcpPage
	case is.Agent != nil:
		return agentPath(is.Agent.ID)
	case is.Rule != nil:
		return agentPath(is.Rule.Agent)
	case is.Tool == "databases":
		return "/tools/databases"
	case is.Project != "":
		return projectPath(is.Project)
	case is.Tool == "docker":
		return "/tools/docker"
	case is.Tool == "git":
		return gitPage
	case jsTool(is.Tool):
		return "/tools/jspm"
	case is.Tool != "":
		for _, l := range languages {
			if l.id == is.Tool {
				return l.path
			}
		}
		return overview.path
	}
	return pathPage
}

// issueFix is what fixes is with an operation that exists, nil for an
// issue without one: the page it lives on tells what to do then. found
// are the scan's issues, for the skills' copies, which merge at once.
func (a *App) issueFix(is issues.Issue, found []issues.Issue) func() {
	switch is.Kind {
	case issues.PathMissing:
		term := a.Snapshot.TerminalPath
		var dirs []string
		for i, e := range term {
			if slices.Contains(is.Dirs, e.Dir) && !slices.Contains(dirs, e.Dir) && a.fixable(term, i) {
				dirs = append(dirs, e.Dir)
			}
		}
		if len(dirs) == 0 {
			return nil
		}
		return func() {
			a.PlanPath(term, dirs, func(p ops.Plan, err error) {
				if errors.Is(err, ops.ErrNoFix) {
					err = errors.New(locales.PathNoFix())
				}
				a.proposeOrSay(p, err)
			})
		}
	case issues.SlowStartup:
		if a.BuildPlan == nil {
			return nil
		}
		for _, l := range is.Startup.Lines {
			if sysenv.StartupCulprit(l.Text) == sysenv.CulpritNvm {
				return func() { a.nvmMove(l) }
			}
		}
	case issues.ProjectMissing:
		for _, p := range a.Projects {
			if p.Dir != is.Project {
				continue
			}
			for _, st := range projects.Resolve(p, a.Snapshot, a.nodeLines()) {
				if st.Tool != is.Tool {
					continue
				}
				if plan, ok := a.projectInstall(st); ok {
					return func() { a.Propose(plan) }
				}
			}
		}
	case issues.DockerUnavailable:
		if plan, ok := a.engineStartPlan(); ok {
			return func() { a.Propose(plan) }
		}
	case issues.DatabasePortConflict:
		if plan, ok := a.conflictPlan(is, inventory.DatabaseServers(a.Snapshot)); ok {
			return func() { a.Propose(plan) }
		}
	case issues.PlainSecret:
		if sv := is.MCP[0]; a.BuildPlan != nil && ops.SecretMovable(sv) {
			return func() { a.moveSecrets(sv) }
		}
	case issues.SettingsSecret:
		if a.BuildPlan != nil {
			ag := *is.Agent
			return func() {
				a.BuildPlan(func(env sysenv.Env, terminal ops.Terminal) (ops.Plan, error) {
					return ops.SettingsSecretPlan(ag, env, terminal, agentTitle)
				}, a.proposeOrSay)
			}
		}
	case issues.SkillCopies:
		if a.BuildPlan == nil || a.SkillBackups == "" {
			return nil
		}
		var groups [][]inventory.Skill
		for _, o := range found {
			if o.Kind == issues.SkillCopies {
				groups = append(groups, o.Skills)
			}
		}
		backups := a.SkillBackups
		return func() {
			a.BuildPlan(func(env sysenv.Env, _ ops.Terminal) (ops.Plan, error) {
				return ops.SkillMergePlan(groups, backups, env, time.Now())
			}, a.proposeOrSay)
		}
	}
	return nil
}

// moveSecrets proposes the plan that moves the secrets sv's file writes
// out into environment variables.
func (a *App) moveSecrets(sv inventory.MCPServer) {
	a.BuildPlan(func(env sysenv.Env, terminal ops.Terminal) (ops.Plan, error) {
		return ops.MCPSecretPlan(sv, env, terminal, agentTitle)
	}, a.proposeOrSay)
}
