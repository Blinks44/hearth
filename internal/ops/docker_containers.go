package ops

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

type DockerAction string

const (
	DockerStart   DockerAction = "start"
	DockerStop    DockerAction = "stop"
	DockerRestart DockerAction = "restart"
	DockerRemove  DockerAction = "rm"
	DockerUp      DockerAction = "up"
	DockerDown    DockerAction = "down"
)

func (action DockerAction) Label() string {
	switch action {
	case DockerStart:
		return locales.DockerContainerStart()
	case DockerStop:
		return locales.DockerContainerStop()
	case DockerRestart:
		return locales.DockerContainerRestart()
	case DockerRemove:
		return locales.DockerContainerRemove()
	case DockerUp:
		return locales.DockerComposeUp()
	case DockerDown:
		return locales.DockerComposeDown()
	default:
		return ""
	}
}

func (action DockerAction) Applies(c docker.Container) bool {
	switch action {
	case DockerStart:
		return c.State.Status == "created" || c.State.Status == "exited"
	case DockerStop, DockerRestart:
		return c.Active() && c.State.Status != "paused"
	case DockerRemove:
		return c.State.Status != "removing" && c.State.Status != "paused"
	default:
		return false
	}
}

type dockerContainerChange struct {
	action DockerAction
	target docker.Target
	item   docker.Container
}

func PlanDockerContainers(target docker.Target, env sysenv.Env, items []docker.Container, action DockerAction) (Plan, error) {
	if target.Docker == "" || target.Endpoint == "" || len(items) == 0 {
		return Plan{}, errors.New(locales.DockerContainerChanged())
	}
	label := action.Label()
	p := Plan{
		Title: locales.DockerContainerPlan(label, len(items), target.Context),
		Why:   locales.DockerContainerScope(), Action: label, Env: slices.Clone(env), DockerAction: action,
	}
	if action == DockerRemove {
		p.Notes = append(p.Notes, Note{Title: locales.DockerContainerRemoveNote(), Text: locales.DockerVolumesStay()})
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if !action.Applies(item) || !docker.ContainerID(item.ID) || seen[item.ID] {
			return Plan{}, errors.New(locales.DockerContainerChanged())
		}
		seen[item.ID] = true
		change := &dockerContainerChange{action: action, target: target, item: item}
		p.Steps = append(p.Steps, Step{
			Argv: change.command(env, action).Args, Note: item.Name, dockerContainer: change,
		})
	}
	return p, nil
}

func (m *dockerContainerChange) command(env sysenv.Env, action DockerAction) run.Cmd {
	cmd := m.target.Command(env, "container", string(action), "--", m.item.ID)
	cmd.Timeout = time.Minute
	return cmd
}

func (m *dockerContainerChange) apply(ctx context.Context, r run.Runner, env sysenv.Env) error {
	ids := []string{m.item.ID}
	got, err := docker.InspectContainers(ctx, r, env, m.target, ids)
	if err != nil {
		return err
	}
	if len(got) != 1 || got[0].ID != m.item.ID || got[0].Name != m.item.Name || !m.action.Applies(got[0]) ||
		m.action == DockerRemove && got[0].Active() != m.item.Active() {
		return errors.New(locales.DockerContainerChanged())
	}
	if m.action == DockerRemove && got[0].Active() {
		if err := runDockerAction(ctx, r, m.command(env, DockerStop)); err != nil {
			return err
		}
	}
	if err := runDockerAction(ctx, r, m.command(env, m.action)); err != nil {
		return err
	}
	next, err := docker.InspectContainers(ctx, r, env, m.target, ids)
	if err != nil {
		return err
	}
	if m.action == DockerRemove && len(next) == 0 {
		return nil
	}
	if len(next) == 1 && next[0].ID == m.item.ID {
		switch m.action {
		case DockerStop:
			if !next[0].Active() {
				return nil
			}
		case DockerStart, DockerRestart:
			if next[0].State.Status == "running" && (m.action == DockerStart || next[0].State.StartedAt.After(got[0].State.StartedAt)) {
				return nil
			}
		}
	}
	return errors.New(locales.DockerContainerCheck())
}

func runDockerAction(ctx context.Context, r run.Runner, cmd run.Cmd) error {
	res, err := r.Run(ctx, cmd)
	if err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("docker: %s (exit %d)", strings.TrimSpace(string(res.Stderr)), res.ExitCode)
	}
	return nil
}

func (e *execution) executeDockerContainers(ctx context.Context, r run.Runner, p Plan) State {
	e.state.StepDetails = make([]string, len(p.Steps))
	failed := false
	for i, step := range p.Steps {
		if ctx.Err() != nil {
			e.end(i, Skipped, Stopped, -1, "")
			e.emit()
			return e.snapshot()
		}
		e.state.Current, e.state.Steps[i] = i, Busy
		e.printf("$ %s\n", Command(step.Shown()))
		e.emit()
		err := step.dockerContainer.apply(ctx, r, p.Env)
		e.state.Steps[i] = Done
		if err != nil {
			failed = true
			e.state.Steps[i], e.state.StepDetails[i] = StepFailed, err.Error()
			if ctx.Err() != nil {
				e.state.Steps[i] = StepStopped
			}
			e.printf("%s: %s\n", step.Note, err)
		}
		e.emit()
	}
	switch {
	case ctx.Err() != nil:
		e.state.Phase = Stopped
	case failed:
		e.state.Phase, e.state.Err = NotVerified, locales.DockerContainerPartial()
	default:
		e.state.Phase = Succeeded
	}
	e.emit()
	return e.snapshot()
}

// PlanDockerCompose pins the engine, files and project directory. A known
// Compose name comes from the engine; when bringing up a Hearth project,
// Compose resolves the name from its file and environment.
func PlanDockerCompose(ctx context.Context, r run.Runner, env sysenv.Env, target docker.Target,
	project docker.ComposeProject, action DockerAction) (Plan, error) {
	if target.Docker == "" || target.Endpoint == "" || !filepath.IsAbs(project.Dir) || project.Files == "" ||
		(project.Name == "" && action != DockerUp && action != DockerDown) || action.Label() == "" {
		return Plan{}, errors.New(locales.DockerComposeMissing())
	}
	base := []string{"compose", "--project-directory", project.Dir}
	if project.Name != "" {
		base = append(base, "-p", project.Name)
	}
	files := strings.Split(project.Files, ",")
	if project.Name == "" {
		files = []string{project.Files}
		for _, name := range []string{"compose.override.yaml", "compose.override.yml", "docker-compose.override.yml", "docker-compose.override.yaml"} {
			path := filepath.Join(project.Dir, name)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				files = append(files, path)
				break
			}
		}
	}
	for _, file := range files {
		if !filepath.IsAbs(file) {
			return Plan{}, errors.New(locales.DockerComposeMissing())
		}
		base = append(base, "-f", file)
	}
	configCmd := target.Command(env, append(slices.Clone(base), "config", "--format", "json")...)
	configCmd.Dir = project.Dir
	readConfig := func(ctx context.Context) (string, [sha256.Size]byte, error) {
		result, err := r.Run(ctx, configCmd)
		defer clear(result.Stdout)
		if err != nil {
			return "", [sha256.Size]byte{}, fmt.Errorf("compose config: %w", err)
		}
		var config struct{ Name string }
		if result.ExitCode != 0 || result.Truncated || json.Unmarshal(result.Stdout, &config) != nil || config.Name == "" {
			return "", [sha256.Size]byte{}, errors.New(locales.DockerComposeConfig())
		}
		// Only the fingerprint survives planning. docker compose config can
		// include secrets from the environment and service env files.
		return config.Name, sha256.Sum256(result.Stdout), nil
	}
	name, configHash, err := readConfig(ctx)
	if err != nil {
		return Plan{}, err
	}
	if project.Name == "" {
		project.Name, project.Files = name, strings.Join(files, ",")
		base = append(base, "-p", name)
	} else if project.Name != name {
		return Plan{}, errors.New(locales.DockerComposeChanged())
	}

	readProject := func(ctx context.Context) ([]docker.Container, error) {
		for _, file := range files {
			info, err := os.Stat(file)
			if err != nil {
				return nil, fmt.Errorf("compose file: %w", err)
			}
			if !info.Mode().IsRegular() {
				return nil, errors.New(locales.DockerComposeMissing())
			}
		}
		items, err := docker.ReadContainers(ctx, r, env, target)
		if err != nil {
			return nil, err
		}
		var scoped []docker.Container
		for _, c := range items {
			if c.Compose.Name != project.Name {
				continue
			}
			if c.Compose != project {
				return nil, errors.New(locales.DockerComposeChanged())
			}
			scoped = append(scoped, c)
		}
		return scoped, nil
	}
	items, err := readProject(ctx)
	if err != nil {
		return Plan{}, err
	}
	check := func(ctx context.Context) error {
		_, currentHash, err := readConfig(ctx)
		if err != nil {
			return err
		}
		if currentHash != configHash {
			return errors.New(locales.DockerComposeConfigChanged())
		}
		next, err := readProject(ctx)
		if err != nil {
			return err
		}
		if !slices.EqualFunc(items, next, func(a, b docker.Container) bool { return a.ID == b.ID && a.Name == b.Name }) {
			return errors.New(locales.DockerContainerChanged())
		}
		return nil
	}

	label := action.Label()
	why := locales.DockerContainerScope()
	notes := []Note{{Title: target.Context, Text: project.Dir}}
	if action == DockerDown || action == DockerRemove {
		why = locales.DockerComposeDownWhy()
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.Name)
		}
		notes = append(notes, Note{Title: locales.DockerVolumesStay()},
			Note{Title: locales.DockerComposeDownRemoved(), Text: strings.Join(names, ", ")})
		action = DockerDown
	}
	args := append(base, string(action))
	if action == DockerUp {
		args = append(args, "-d")
	}
	cmd := target.Command(env, args...)
	return Plan{
		Title: label + " " + project.Name, Why: why, Action: label, Notes: notes, Before: check, DockerAction: action,
		Env: cmd.Env, Steps: []Step{{Argv: cmd.Args, Dir: project.Dir, Timeout: 5 * time.Minute}},
	}, nil
}
