package ops

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/internal/versions"
	"github.com/Blinks44/hearth/locales"
)

// ProjectItem explains the observed state and the next action. A command here
// is for copying only; execution always goes through the confirmed Plan.
type ProjectItem struct {
	Tool, Title, Text, Version string
	Config                     string
	Files                      []string
	Command                    []string
	Ready                      bool
}

// ProjectPreparation contains the read-only assessment and any safe automatic
// steps. Empty Steps still leaves a usable assessment with manual actions.
type ProjectPreparation struct {
	Project projects.Project
	Items   []ProjectItem
	Plan    Plan
}

// ProjectInput is an immutable copy of what the plan builder needs.
type ProjectInput struct {
	Project  projects.Project
	Snapshot *inventory.Snapshot
	Releases map[string][]string
	Lines    map[int]versions.Line
	Setup    Setup
	Title    func(string) string
	Detect   func(context.Context, string) (inventory.ToolState, error)
}

// PrepareProject builds a project-scoped plan. Existing managers are respected;
// global defaults, project scripts and dependency installations are excluded.
func PrepareProject(in ProjectInput, env sysenv.Env) (ProjectPreparation, error) {
	out := ProjectPreparation{Project: in.Project}
	var plans []Plan
	needsMise := false
	localMise := map[string]string{}
	statuses := projects.Resolve(in.Project, in.Snapshot, in.Lines)
	// Runtime tools precede package managers that may need them.
	slices.SortStableFunc(statuses, func(a, b projects.Status) int { return projectOrder(a.Tool) - projectOrder(b.Tool) })
	for _, st := range statuses {
		item := ProjectItem{Tool: st.Tool, Title: in.Title(st.Tool)}
		for _, ch := range st.Checks {
			if !slices.Contains(item.Files, ch.File) {
				item.Files = append(item.Files, ch.File)
			}
		}
		state, _ := in.Snapshot.Tool(st.Tool)
		switch {
		case !st.Checked:
			item.Text = locales.PrepareUnchecked(item.Title)
		case st.Tool == "docker":
			p, ready := projectDocker(state, in.Detect)
			item.Ready = ready
			if !ready && len(p.Steps) == 0 && state.Err == "" {
				setup := in.Setup
				setup.Have = cloneHave(in.Setup.Have)
				setup.Tools = []string{"docker"}
				if state.Docker == nil || len(state.Docker.Engines) == 0 || !slices.ContainsFunc(state.Docker.Engines, func(e inventory.DockerEngine) bool { return e.Path != "" }) {
					setup.Have["docker"] = false
					if install, _, err := SetupPlan(setup, env); err == nil {
						p = install
					}
				}
			}
			if ready {
				item.Text = locales.PrepareReady(item.Title)
			} else {
				item.Text = locales.PrepareDockerManual()
			}
			if len(p.Steps) > 0 {
				plans = append(plans, p)
				item.Text = locales.PrepareDockerStart()
				if p.Title == locales.SetupPlanTitle() {
					item.Text = locales.PrepareDockerInstall()
				}
			}
		default:
			selection, err := projects.SelectVersion(st, in.Releases[st.Tool], in.Lines)
			switch {
			case err != nil:
				item.Text = locales.PrepareNoVersion(item.Title, err.Error())
			case selection.Ready:
				item.Ready = true
				item.Version = selection.Version
				item.Text = locales.PrepareReady(item.Title + " " + selection.Version)
			default:
				item.Version = selection.Version
				manager := projectManager(state)
				if selection.Installed != nil {
					manager = selection.Installed.Source.Manager
					item.Text = locales.PrepareChoose(item.Title, selection.Version)
					if manager == "uv" && st.Tool == "python" {
						item.Ready = true
						item.Text = locales.PreparePythonAvailable(selection.Version)
					}
				} else {
					if manager == "" {
						manager = "mise"
						needsMise = true
					}
					target := VersionTarget{Tool: st.Tool, Title: item.Title, Manager: manager, Version: selection.Version, AsdfSet: true}
					for _, m := range state.Managers {
						if m.Name == manager {
							target.Admin = m.Admin
							target.AsdfSet = AsdfSet(m.Version)
						}
					}
					var detect Detect
					if in.Detect != nil {
						detect = func(ctx context.Context) (inventory.ToolState, error) { return in.Detect(ctx, st.Tool) }
					}
					p, ok := VersionPlan(VersionInstall, target, detect)
					if ok {
						if manager == "corepack" {
							p.Steps = []Step{{Argv: []string{"corepack", "pack", st.Tool + "@" + selection.Version, "--output", "corepack.tgz"}, Env: []string{"COREPACK_DEFAULT_TO_LATEST=0"}}}
						}
						plans = append(plans, p)
						item.Text = locales.PrepareInstall(item.Title, selection.Version, manager)
					} else {
						item.Text = locales.PrepareManual(item.Title)
					}
				}
				if manager == "mise" {
					if !slices.ContainsFunc(state.Managers, func(m inventory.Manager) bool { return m.Name == "mise" && (m.Activated || m.Shims || m.SwitchOnCD) }) {
						needsMise = true
					}
					localMise[st.Tool] = selection.Version
					if st.Tool == "java" {
						localMise[st.Tool] = javaSpec("mise", selection.Version)
					}
				}
				item.Command = projectCommand(st.Tool, manager, selection.Version)
				if manager == "nvm-windows" || manager == "n" {
					item.Text += " " + locales.PrepareGlobalSelection()
				}
				if len(item.Command) > 0 {
					item.Text += " " + locales.PrepareCommandInFolder()
				}
				// Only create a missing local pin read by the activated manager.
				// Existing requirement files are left for explicit manual review.
				if st.Tool == "node" && selection.Installed != nil {
					if pin, ok := PinPlan(in.Project, PinTarget{Tool: st.Tool, Title: item.Title, Version: selection.Version, Manager: manager}); ok && pin.Steps[0].Edit.New {
						file := filepath.Base(pin.Steps[0].Edit.Path)
						if slices.ContainsFunc(state.Managers, func(m inventory.Manager) bool {
							return m.Name == manager && m.SwitchOnCD && slices.Contains(m.Files, file)
						}) {
							plans = append(plans, pin)
						}
					}
				}
			}
		}
		out.Items = append(out.Items, item)
	}
	// Unversioned tools, such as Git or Composer, use the existing catalog.
	// Only include entries whose dependencies already exist; installing a
	// language's arbitrary global default would contradict project requirements.
	for i := range out.Items {
		item := &out.Items[i]
		st, _ := in.Snapshot.Tool(item.Tool)
		if !slices.ContainsFunc(statuses, func(s projects.Status) bool { return s.Tool == item.Tool && s.Checked }) || len(st.Installations) > 0 || item.Tool == "docker" {
			continue
		}
		reqs := slices.DeleteFunc(slices.Clone(in.Project.Requirements), func(r projects.Requirement) bool { return r.Tool != item.Tool })
		if slices.ContainsFunc(reqs, func(r projects.Requirement) bool { return r.Want != "" }) || item.Version != "" {
			continue
		}
		setup := in.Setup
		setup.Have = cloneHave(in.Setup.Have)
		setup.Tools = []string{item.Tool}
		parts := setup.Parts()
		if len(parts) != 1 || parts[0].Tool != item.Tool || parts[0].Method.Manager == "mise" {
			continue
		}
		p, _, err := SetupPlan(setup, env)
		if err == nil {
			plans = append(plans, p)
			item.Text = locales.PrepareInstall(item.Title, "", parts[0].Method.Manager)
		}
	}
	// A new, literal-only project config can select mise's versions without
	// changing global defaults or rewriting an existing project config.
	if len(localMise) > 0 {
		_, hasMise := in.Project.Texts["mise.toml"]
		_, hasDotMise := in.Project.Texts[".mise.toml"]
		if !hasMise && !hasDotMise {
			keys := make([]string, 0, len(localMise))
			for tool := range localMise {
				keys = append(keys, tool)
			}
			slices.Sort(keys)
			var text strings.Builder
			text.WriteString("[tools]\n")
			for _, tool := range keys {
				fmt.Fprintf(&text, "%s = %q\n", tool, localMise[tool])
			}
			plans = append(plans, Plan{Steps: []Step{{Edit: &FileEdit{Path: filepath.Join(in.Project.Dir, "mise.toml"), After: text.String(), New: true}}}})
		}
	}
	for i := range out.Items {
		item := &out.Items[i]
		if _, ok := localMise[item.Tool]; !ok {
			continue
		}
		file := "mise.toml"
		if _, ok := in.Project.Texts[".mise.toml"]; ok {
			file = ".mise.toml"
		}
		if _, ok := in.Project.Texts[file]; ok {
			item.Config = fmt.Sprintf("%s = %q", item.Tool, localMise[item.Tool])
			item.Text += " " + locales.PrepareMiseSetting(file)
			if !slices.Contains(item.Files, file) {
				item.Files = append(item.Files, file)
			}
		}
	}
	p, _ := UpdatePlan(plans)
	if needsMise && len(p.Steps) > 0 {
		mise, err := SetUpMise(env)
		if err != nil {
			return out, fmt.Errorf("prepare project manager: %w", err)
		}
		p = WithMise(p, mise)
	}
	p.Title = locales.PrepareTitle(filepath.Base(in.Project.Dir))
	p.Why = locales.PrepareWhy()
	p.Action = locales.PrepareRun()
	p.Env = slices.Clone(env)
	p.Isolated = true
	p.Notes = append(p.Notes, Note{Title: locales.PrepareScope(), Text: locales.PrepareScopeText()})
	// The plan may stay open while files change. Refuse execution before any
	// installation if the requirements or parse errors have changed.
	observed := in.Project
	p.Before = func(context.Context) error {
		now := projects.Read(observed.Dir)
		if len(now.Errs) > 0 || !reflect.DeepEqual(now.Requirements, observed.Requirements) || !reflect.DeepEqual(now.Texts, observed.Texts) {
			return errors.New(locales.PrepareFilesChanged())
		}
		return nil
	}
	out.Plan = p
	return out, nil
}

func projectOrder(tool string) int {
	switch tool {
	case "node", "python", "ruby", "php", "java", "go", "rust", "dotnet":
		return 0
	case "docker":
		return 2
	}
	return 1
}

func projectManager(st inventory.ToolState) string {
	can := func(name string) bool {
		_, ok := VersionPlan(VersionInstall, VersionTarget{Tool: st.Tool, Manager: name, Version: "0"}, nil)
		return ok
	}
	if i := inventory.TerminalCopy(st.Installations); i >= 0 {
		s := st.Installations[i].Source
		if s.Kind == inventory.KindVersionManager && can(s.Manager) {
			return s.Manager
		}
	}
	for _, m := range st.Managers {
		if can(m.Name) {
			return m.Name
		}
	}
	return ""
}

func projectCommand(tool, manager, version string) []string {
	switch manager {
	case "fnm", "nvm":
		return []string{manager, "use", version}
	case "pyenv", "rbenv", "nodenv":
		return []string{manager, "local", version}
	case "uv":
		return []string{"uv", "python", "find", version}
	case "mise":
		return nil
	}
	return nil
}

func projectDocker(st inventory.ToolState, detect func(context.Context, string) (inventory.ToolState, error)) (Plan, bool) {
	d := st.Docker
	if d == nil {
		return Plan{}, false
	}
	for _, c := range d.Contexts {
		if c.Name != d.Selected || !c.Local {
			continue
		}
		if c.Status == "available" {
			return Plan{}, true
		}
		if c.Status != "unavailable" || c.Denied {
			return Plan{}, false
		}
		var engines []inventory.DockerEngine
		for _, e := range d.Engines {
			if e.Path != "" {
				engines = append(engines, e)
			}
		}
		name := c.Engine
		if name == "" && len(engines) == 1 {
			name = engines[0].Name
		}
		for _, e := range engines {
			if e.Name != name {
				continue
			}
			var check Detect
			if detect != nil {
				check = func(ctx context.Context) (inventory.ToolState, error) { return detect(ctx, "docker") }
			}
			p, ok := EngineStartPlan(Engine{Name: e.Name, Path: e.Path}, d.Selected, check)
			if ok {
				return p, false
			}
		}
	}
	return Plan{}, false
}

// ProjectSources includes every requirement, without hiding conflicts behind
// the first file's constraint.
func ProjectSources(p projects.Project, tool string) string {
	var rows []string
	for _, r := range p.Requirements {
		if r.Tool == tool {
			where := r.File
			if r.Key != "" {
				where += " · " + r.Key
			}
			rows = append(rows, where+" → "+r.Want)
		}
	}
	return strings.Join(rows, "; ")
}

func cloneHave(have map[string]bool) map[string]bool {
	out := make(map[string]bool, len(have))
	maps.Copy(out, have)
	return out
}
