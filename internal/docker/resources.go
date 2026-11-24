package docker

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

type ResourceKind string

const (
	VolumeResource  ResourceKind = "volume"
	NetworkResource ResourceKind = "network"
)

type Resource struct {
	ID, Name, Driver, Project string
	Created                   time.Time
	Subnets                   []string
	Users                     []ResourceUser
	Size                      int64 // -1 when the engine did not report a size
}

type ResourceUser struct {
	ID, Name, Project string
	Active            bool
}

func (r Resource) SystemNetwork() bool {
	return r.Name == "bridge" || r.Name == "host" || r.Name == "none"
}

var volumeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func (k ResourceKind) ValidID(id string) bool {
	switch k {
	case VolumeResource:
		return volumeName.MatchString(id)
	case NetworkResource:
		return ContainerID(id)
	default:
		return false
	}
}

const inspectVolume = `{"ID":{{json .Name}},"Name":{{json .Name}},"Driver":{{json .Driver}},"Project":{{with index .Labels "com.docker.compose.project"}}{{json .}}{{else}}""{{end}},"Created":{{with .CreatedAt}}{{json .}}{{else}}null{{end}}}`
const inspectNetwork = `{"ID":{{json .Id}},"Name":{{json .Name}},"Driver":{{json .Driver}},"Project":{{with index .Labels "com.docker.compose.project"}}{{json .}}{{else}}""{{end}},"Created":{{json .Created}},"IPAM":{{json .IPAM.Config}},"Endpoints":{{json .Containers}}}`

type resourceRow struct {
	Resource
	IPAM      []struct{ Subnet string }
	Endpoints map[string]struct{ Name string }
}

func isMissingDockerObject(stderr, kind string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "no such "+kind) || strings.Contains(lower, "not found")
}

// InspectResources reads only the selected IDs and includes stopped containers.
func InspectResources(ctx context.Context, r run.Runner, env sysenv.Env, target Target, kind ResourceKind, ids []string) ([]Resource, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	format := inspectNetwork
	if kind == VolumeResource {
		format = inspectVolume
	}
	var rows []resourceRow
	for batch := range slices.Chunk(ids, 64) {
		for _, id := range batch {
			if !kind.ValidID(id) {
				return nil, fmt.Errorf("docker %s inspect: invalid ID %q", kind, id)
			}
		}
		cmd := target.Command(env, string(kind), "inspect", "--format", format, "--")
		cmd.Args = append(cmd.Args, batch...)
		res, err := r.Run(ctx, cmd)
		if err != nil {
			return nil, fmt.Errorf("docker %s inspect: %w", kind, err)
		}
		if res.Truncated {
			return nil, fmt.Errorf("docker %s inspect: output too large", kind)
		}
		if res.ExitCode != 0 && !isMissingDockerObject(string(res.Stderr), string(kind)) {
			return nil, fmt.Errorf("docker %s inspect: %s", kind, strings.TrimSpace(string(res.Stderr)))
		}
		for line := range strings.Lines(string(res.Stdout)) {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var item resourceRow
			if err := json.Unmarshal([]byte(line), &item); err != nil {
				return nil, fmt.Errorf("docker %s inspect: %w", kind, err)
			}
			rows = append(rows, item)
		}
	}
	containers, err := ReadContainers(ctx, r, env, target)
	if err != nil {
		return nil, err
	}
	users := make(map[string]map[string]ResourceUser, len(rows))
	for _, row := range rows {
		users[row.ID] = make(map[string]ResourceUser, len(row.Endpoints))
		for id, endpoint := range row.Endpoints {
			users[row.ID][id] = ResourceUser{ID: id, Name: endpoint.Name, Active: true}
		}
	}
	for _, c := range containers {
		user := ResourceUser{ID: c.ID, Name: c.Name, Project: c.Compose.Name, Active: c.Active()}
		if kind == VolumeResource {
			for _, mount := range c.Mounts {
				if byID := users[mount.Name]; mount.Type == "volume" && byID != nil {
					byID[c.ID] = user
				}
			}
		} else {
			for _, network := range c.Networks {
				if byID := users[network.NetworkID]; byID != nil {
					byID[c.ID] = user
				}
			}
		}
	}
	items := make([]Resource, 0, len(rows))
	for _, row := range rows {
		item := row.Resource
		item.Size = -1
		for _, config := range row.IPAM {
			if config.Subnet != "" {
				item.Subnets = append(item.Subnets, config.Subnet)
			}
		}
		for _, user := range users[row.ID] {
			item.Users = append(item.Users, user)
		}
		slices.SortFunc(item.Users, func(a, b ResourceUser) int {
			return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
		})
		items = append(items, item)
	}
	return items, nil
}

func readResources(ctx context.Context, r run.Runner, env sysenv.Env, target Target, kind ResourceKind) ([]Resource, error) {
	args := []string{string(kind), "ls", "--format", "{{json .}}"}
	if kind == NetworkResource {
		args = append(args, "--no-trunc")
	}
	rows, err := readRows[struct{ ID, Name string }](ctx, r, target.Command(env, args...))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if kind == VolumeResource {
			ids = append(ids, row.Name)
		} else {
			ids = append(ids, row.ID)
		}
	}
	items, err := InspectResources(ctx, r, env, target, kind, ids)
	slices.SortFunc(items, func(a, b Resource) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return items, err
}

func readVolumeSizes(ctx context.Context, r run.Runner, env sysenv.Env, target Target, items []Resource) error {
	cmd := target.Command(env, "system", "df", "--verbose", "--format", "{{json .Volumes}}")
	cmd.Timeout = time.Minute
	rows, err := readRows[[]struct{ Name, Size string }](ctx, r, cmd)
	if err != nil {
		return err
	}
	sizes := make(map[string]int64, len(items))
	for _, batch := range rows {
		for _, row := range batch {
			if size, err := ParseSize(strings.ReplaceAll(row.Size, " ", "")); err == nil && size >= 0 {
				sizes[row.Name] = size
			}
		}
	}
	for i := range items {
		if size, ok := sizes[items[i].Name]; ok {
			items[i].Size = size
		}
	}
	return nil
}

func WatchResources(ctx context.Context, r run.Runner, env sysenv.Env, target Target, kind ResourceKind, got func([]Resource, error, error)) {
	watchChanges(ctx, env, target, []string{"type=" + string(kind), "type=container"}, func(ctx context.Context) {
		items, err := readResources(ctx, r, env, target, kind)
		var sizeErr error
		if err == nil && kind == VolumeResource && len(items) > 0 {
			sizeErr = readVolumeSizes(ctx, r, env, target, items)
		}
		if ctx.Err() == nil {
			got(items, err, sizeErr)
		}
	})
}
