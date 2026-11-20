package docker

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Command addresses the socket directly, independently of the CLI's context.
func (t Target) Command(env sysenv.Env, args ...string) run.Cmd {
	return run.Cmd{
		Args:    append([]string{t.Docker, "--host", t.Endpoint}, args...),
		Env:     append(slices.Clone(env), "DOCKER_CONTEXT="),
		Timeout: 10 * time.Second,
	}
}

type ComposeProject struct {
	Name, Dir, Files string
}

type Container struct {
	ID, Name, Image, ImageID, Kind string
	Compose                        ComposeProject
	State                          struct {
		Status, Error string
		ExitCode      int
		OOMKilled     bool
		StartedAt     time.Time
		Health        *struct{ Status string }
	}
	RestartCount int
	Ports        string
	CPU, Memory  string
}

func (c Container) Active() bool {
	switch c.State.Status {
	case "running", "restarting", "paused":
		return true
	default:
		return false
	}
}

func (c Container) Problem() bool {
	return c.State.Error != "" || c.State.Status == "dead" || c.State.Status == "restarting" ||
		(c.State.Status == "exited" && c.State.ExitCode != 0) ||
		(c.State.Health != nil && c.State.Health.Status == "unhealthy")
}

var boundPort = regexp.MustCompile(`(?i)(?:address already in use|port is already allocated|only one usage)`)
var portAddress = regexp.MustCompile(`:(\d{1,5})(?:\s|:|/|$)`)

func (c Container) ConflictPort() int {
	if !boundPort.MatchString(c.State.Error) {
		return 0
	}
	m := portAddress.FindStringSubmatch(c.State.Error)
	if len(m) == 0 {
		return 0
	}
	port, _ := strconv.Atoi(m[1])
	if port > 65535 {
		return 0
	}
	return port
}

var containerID = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ContainerID accepts only full, immutable IDs so names cannot retarget a plan.
func ContainerID(id string) bool { return containerID.MatchString(id) }

const inspectContainer = `{"ID":{{json .Id}},"Name":{{json .Name}},"Image":{{json .Config.Image}},"ImageID":{{json (index . "Image")}},"Kind":{{json (index .Config.Labels "io.x-k8s.kind.cluster")}},"Compose":{"Name":{{json (index .Config.Labels "com.docker.compose.project")}},"Dir":{{json (index .Config.Labels "com.docker.compose.project.working_dir")}},"Files":{{json (index .Config.Labels "com.docker.compose.project.config_files")}}},"State":{"Status":{{json .State.Status}},"Error":{{json .State.Error}},"ExitCode":{{json .State.ExitCode}},"OOMKilled":{{json (index .State "OOMKilled")}},"StartedAt":{{json .State.StartedAt}},"Health":{{if $health := index .State "Health"}}{"Status":{{json (index $health "Status")}}}{{else}}null{{end}}},"RestartCount":{{json (index . "RestartCount")}}}`

// InspectContainers reads only the fields needed by the list and plans.
func InspectContainers(ctx context.Context, r run.Runner, env sysenv.Env, t Target, ids []string) ([]Container, error) {
	var containers []Container
	for batch := range slices.Chunk(ids, 64) {
		for _, id := range batch {
			if !ContainerID(id) {
				return nil, fmt.Errorf("docker inspect: invalid container ID %q", id)
			}
		}
		cmd := t.Command(env, "container", "inspect", "--format", inspectContainer, "--")
		cmd.Args = append(cmd.Args, batch...)
		res, err := r.Run(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("docker inspect: %w", err)
		}
		if res.Truncated || (res.ExitCode != 0 && !strings.Contains(string(res.Stderr), "No such container")) {
			return nil, fmt.Errorf("docker inspect: %s", strings.TrimSpace(string(res.Stderr)))
		}
		// Docker emits rows for the containers that still exist even if
		// another one disappeared between ps and inspect.
		for line := range strings.Lines(string(res.Stdout)) {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var c Container
			if err := json.Unmarshal([]byte(line), &c); err != nil {
				return nil, fmt.Errorf("docker inspect: %w", err)
			}
			c.Name = strings.TrimPrefix(c.Name, "/")
			containers = append(containers, c)
		}
	}
	return containers, nil
}

type containerRow struct{ ID, Ports string }

func ReadContainers(ctx context.Context, r run.Runner, env sysenv.Env, t Target) ([]Container, error) {
	rows, err := readRows[containerRow](ctx, r, t.Command(env, "ps", "--all", "--no-trunc", "--format", "{{json .}}"))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	ports := make(map[string]string, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
		ports[row.ID] = row.Ports
	}
	containers, err := InspectContainers(ctx, r, env, t, ids)
	if err != nil {
		return nil, err
	}
	for i := range containers {
		containers[i].Ports = ports[containers[i].ID]
	}
	slices.SortFunc(containers, func(a, b Container) int { return cmp.Compare(a.Name, b.Name) })
	return containers, nil
}

type containerStat struct{ ID, CPUPerc, MemUsage string }

func sampleContainers(ctx context.Context, r run.Runner, env sysenv.Env, t Target, containers []Container) error {
	stats, err := readRows[containerStat](ctx, r, t.Command(env, "stats", "--no-stream", "--no-trunc", "--format", "{{json .}}"))
	if err != nil {
		return err
	}
	byID := make(map[string]containerStat, len(stats))
	for _, stat := range stats {
		byID[stat.ID] = stat
	}
	for i := range containers {
		if stat, ok := byID[containers[i].ID]; ok && containers[i].Active() {
			containers[i].CPU = stat.CPUPerc
			mem, _, _ := strings.Cut(stat.MemUsage, " / ")
			containers[i].Memory = mem
		}
	}
	return nil
}

func readRows[T any](ctx context.Context, r run.Runner, cmd run.Cmd) ([]T, error) {
	res, err := r.Run(ctx, cmd)
	name := "docker " + cmd.Args[3]
	switch {
	case err != nil:
		return nil, fmt.Errorf("%s: %w", name, err)
	case res.ExitCode != 0:
		return nil, fmt.Errorf("%s: %s", name, strings.TrimSpace(string(res.Stderr)))
	case res.Truncated:
		return nil, fmt.Errorf("%s: output too large", name)
	}
	var rows []T
	for line := range strings.Lines(string(res.Stdout)) {
		var row T
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// WatchContainers combines event-driven list refreshes with periodic samples
// of CPU and memory. Only the visible Containers view keeps it running.
func WatchContainers(ctx context.Context, r run.Runner, env sysenv.Env, t Target, got func([]Container, error, error)) {
	watchChanges(ctx, env, t, []string{"type=container"}, func(ctx context.Context) {
		items, err := ReadContainers(ctx, r, env, t)
		var statsErr error
		if err == nil && slices.ContainsFunc(items, Container.Active) {
			statsErr = sampleContainers(ctx, r, env, t, items)
		}
		if ctx.Err() == nil {
			got(items, err, statsErr)
		}
	})
}

func watchChanges(ctx context.Context, env sysenv.Env, t Target, filters []string, read func(context.Context)) {
	if ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	changed := make(chan struct{}, 1)
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		cmd := t.Command(env, "events", "--format", "{{.Action}}")
		for _, filter := range filters {
			cmd.Args = append(cmd.Args, "--filter", filter)
		}
		for ctx.Err() == nil {
			_ = run.Lines(ctx, cmd, func(string) {
				select {
				case changed <- struct{}{}:
				default:
				}
			})
			select {
			case <-ctx.Done():
				return
			case <-time.After(Every):
			}
		}
	}()
	defer func() {
		cancel()
		<-streamDone
	}()
	for ctx.Err() == nil {
		read(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(Every):
		case <-changed:
			select {
			case <-ctx.Done():
				return
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
}
