package ops

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/docker"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

const DockerResourceRemove DockerAction = "resource-remove"

func PlanDockerResourceRemoval(ctx context.Context, r run.Runner, target docker.Target, env sysenv.Env, kind docker.ResourceKind, item docker.Resource) (Plan, error) {
	return PlanDockerResourcesRemoval(ctx, r, target, env, kind, []docker.Resource{item})
}

func PlanDockerResourcesRemoval(ctx context.Context, r run.Runner, target docker.Target, env sysenv.Env, kind docker.ResourceKind, items []docker.Resource) (Plan, error) {
	changed := errors.New(locales.DockerResourceChanged())
	if target.Docker == "" || target.Endpoint == "" || len(items) == 0 {
		return Plan{}, changed
	}
	unique := make([]docker.Resource, 0, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if !kind.ValidID(item.ID) {
			return Plan{}, changed
		}
		if slices.Contains(ids, item.ID) {
			continue
		}
		unique = append(unique, item)
		ids = append(ids, item.ID)
	}
	check := func(ctx context.Context) (string, error) {
		freshItems, err := docker.InspectResources(ctx, r, env, target, kind, ids)
		if err != nil {
			return "", err
		}
		byID := make(map[string]docker.Resource, len(freshItems))
		for _, fresh := range freshItems {
			byID[fresh.ID] = fresh
		}
		var inUse []string
		for _, item := range unique {
			fresh, ok := byID[item.ID]
			if !ok || fresh.ID != item.ID || fresh.Name != item.Name || fresh.Driver != item.Driver ||
				fresh.Project != item.Project || !fresh.Created.Equal(item.Created) {
				return "", changed
			}
			if kind == docker.NetworkResource && fresh.SystemNetwork() {
				return locales.DockerNetworkProtected(), nil
			}
			for _, user := range fresh.Users {
				if !slices.Contains(inUse, user.Name) {
					inUse = append(inUse, user.Name)
				}
			}
		}
		if len(inUse) > 0 {
			return locales.DockerResourceInUse(strings.Join(inUse, ", ")), nil
		}
		return "", nil
	}
	blocked, err := check(ctx)
	if err != nil {
		return Plan{}, err
	}
	p := Plan{DockerAction: DockerResourceRemove, Action: locales.DockerContainerRemove()}
	if kind == docker.VolumeResource {
		p.Why = locales.DockerVolumeRemoveWhy(target.Context)
		if len(unique) == 1 {
			p.Title = locales.DockerVolumeRemoveTitle(unique[0].Name)
		} else {
			p.Title = locales.DockerVolumesRemoveTitle(len(unique))
		}
	} else {
		p.Why = locales.DockerNetworkRemoveWhy(target.Context)
		if len(unique) == 1 {
			p.Title = locales.DockerNetworkRemoveTitle(unique[0].Name)
		} else {
			p.Title = locales.DockerNetworksRemoveTitle(len(unique))
		}
	}
	if blocked != "" {
		p.Why = blocked
		return p, nil
	}
	var projects []string
	for _, item := range unique {
		if item.Project != "" && !slices.Contains(projects, item.Project) {
			projects = append(projects, item.Project)
			p.Notes = append(p.Notes, Note{Title: locales.DockerResourceProject(item.Project)})
		}
	}
	p.Before = func(ctx context.Context) error {
		blocked, err := check(ctx)
		if err == nil && blocked != "" {
			err = errors.New(blocked)
		}
		return err
	}
	steps := make([]Step, 0, len(unique))
	for _, item := range unique {
		cmd := target.Command(env, string(kind), "rm", "--", item.ID)
		p.Env = cmd.Env
		steps = append(steps, Step{Argv: cmd.Args, Note: item.Name, Timeout: time.Minute})
	}
	p.Steps = steps
	return p, nil
}
